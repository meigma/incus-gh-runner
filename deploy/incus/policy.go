// Package incuspolicy validates rendered Incus baselines against the embedded CUE policy.
package incuspolicy

import (
	_ "embed"
	"errors"
	"fmt"

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

	return nil
}
