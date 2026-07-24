// Package incuspolicy validates rendered Incus baselines against the embedded CUE policy.
package incuspolicy

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cuejson "cuelang.org/go/encoding/json"
)

// MaximumBaselineBytes bounds the rendered manifest accepted by the embedded policy.
const MaximumBaselineBytes = 1 << 20

const validationBridge = "\n#EmbeddedBaselineSchema: _#Baseline\n"

//go:embed cue/deployment.cue
var policySource []byte

// ValidateBaseline checks one rendered JSON baseline against the embedded CUE policy.
func ValidateBaseline(filename string, data []byte) error {
	if len(data) == 0 {
		return errors.New("baseline is empty")
	}
	if len(data) > MaximumBaselineBytes {
		return fmt.Errorf("baseline exceeds the %d-byte limit", MaximumBaselineBytes)
	}

	ctx := cuecontext.New(cuecontext.EvaluatorVersion(cuecontext.EvalStable))
	root := ctx.CompileBytes(
		append(append([]byte(nil), policySource...), validationBridge...),
		cue.Filename("deployment.cue"),
	)
	if err := root.Err(); err != nil {
		return fmt.Errorf("compile embedded CUE policy: %w", err)
	}

	schema := root.LookupPath(cue.MakePath(cue.Def("EmbeddedBaselineSchema")))
	if err := schema.Err(); err != nil {
		return fmt.Errorf("load embedded baseline schema: %w", err)
	}

	expression, err := cuejson.Extract(filename, data)
	if err != nil {
		return fmt.Errorf("parse baseline JSON: %w", err)
	}
	baseline := ctx.BuildExpr(expression)
	if err := baseline.Err(); err != nil {
		return fmt.Errorf("build baseline value: %w", err)
	}

	// Specialize the relational schema before subsumption. Concrete validation
	// rejects conflicts, unknown fields, and unresolved input-derived values;
	// subsuming the original value afterward catches fixed fields that unification
	// would otherwise fill from the policy.
	validated := schema.Unify(baseline)
	if err := validated.Validate(cue.Concrete(true), cue.Final()); err != nil {
		return fmt.Errorf("baseline violates CUE policy: %w", err)
	}
	if err := validated.Subsume(baseline, cue.Final(), cue.Raw()); err != nil {
		return fmt.Errorf("baseline violates CUE policy: %w", err)
	}
	if err := validateAdditionalEgress(data); err != nil {
		return fmt.Errorf("baseline violates CUE policy: %w", err)
	}

	return nil
}

// validateAdditionalEgress enforces relational list constraints after CUE fixes each rule's shape.
func validateAdditionalEgress(data []byte) error {
	const (
		fixedEgressRules       = 3
		maximumAdditionalRules = 16
	)

	var manifest struct {
		NetworkACL struct {
			Egress []struct {
				Destination     string `json:"destination"`
				DestinationPort string `json:"destination_port"`
				Protocol        string `json:"protocol"`
			} `json:"egress"`
		} `json:"network_acl"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode validated egress rules: %w", err)
	}

	rules := manifest.NetworkACL.Egress
	if len(rules) > fixedEgressRules+maximumAdditionalRules {
		return fmt.Errorf("additional egress exceeds %d rules", maximumAdditionalRules)
	}

	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		key := rule.Protocol + "\x00" + rule.Destination + "\x00" + rule.DestinationPort
		if _, exists := seen[key]; exists {
			return fmt.Errorf("egress rule %d duplicates an earlier endpoint", index)
		}
		seen[key] = struct{}{}

		if index < fixedEgressRules {
			continue
		}
		port, err := strconv.Atoi(rule.DestinationPort)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("additional egress rule %d has an invalid port", index)
		}
	}

	return nil
}
