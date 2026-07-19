package incusvalidate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const nestingCompatibilityNotice = "Incus 7.0-7.2 cannot enforce VM nesting at project level; exact profile security.nesting=false is the compensating control."

// ParseBaseline validates and strictly decodes one rendered baseline.
func ParseBaseline(filename string, data []byte, policy PolicyValidator) (Baseline, error) {
	if policy == nil {
		return Baseline{}, errors.New("baseline policy validator is required")
	}
	if err := policy(filename, data); err != nil {
		return Baseline{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var baseline Baseline
	if err := decoder.Decode(&baseline); err != nil {
		return Baseline{}, fmt.Errorf("decode baseline: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Baseline{}, err
	}

	return baseline, nil
}

// Validate compares one parsed baseline with a fresh read-only Incus snapshot.
func Validate(ctx context.Context, baseline Baseline, reader Reader) (Result, error) {
	if reader == nil {
		return Result{}, errors.New("incus validation reader is required")
	}

	snapshot, err := reader.Read(ctx, baseline.Names)
	if err != nil {
		return Result{}, fmt.Errorf("read Incus state: %w", err)
	}
	if err := validateServer(baseline, snapshot.Server); err != nil {
		return Result{}, err
	}
	if err := compareResource("project", baseline.Project, snapshot.Project); err != nil {
		return Result{}, err
	}
	if err := compareResource("network", baseline.Network, snapshot.Network); err != nil {
		return Result{}, err
	}
	if err := compareResource(
		"network ACL",
		normalizeNetworkACL(baseline.NetworkACL),
		normalizeNetworkACL(snapshot.NetworkACL),
	); err != nil {
		return Result{}, err
	}
	if err := compareResource("profile", baseline.Profile, snapshot.Profile); err != nil {
		return Result{}, err
	}
	if err := compareResource(
		"storage pool",
		normalizeExpectedStorage(baseline.StoragePool),
		normalizeObservedStorage(snapshot.StoragePool),
	); err != nil {
		return Result{}, err
	}

	return Result{Notices: []string{nestingCompatibilityNotice}}, nil
}

// requireJSONEnd rejects trailing values after the baseline object.
func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode trailing baseline data: %w", err)
	}

	return errors.New("baseline contains more than one JSON value")
}

// validateServer checks the daemon properties and extensions required by the baseline.
func validateServer(baseline Baseline, actual ServerState) error {
	minimum, err := parseVersion(baseline.Server.MinimumVersion)
	if err != nil {
		return fmt.Errorf("invalid minimum Incus version %q: %w", baseline.Server.MinimumVersion, err)
	}
	have, err := parseVersion(actual.Version)
	if err != nil {
		return fmt.Errorf("server returned invalid Incus version %q: %w", actual.Version, err)
	}
	if compareVersion(have, minimum) < 0 {
		return fmt.Errorf("incus %s or newer is required; found %s", baseline.Server.MinimumVersion, actual.Version)
	}
	if actual.Auth != "trusted" {
		return errors.New("validator requires a trusted read-only API view")
	}
	if baseline.Server.Standalone && actual.Clustered {
		return errors.New("clustered Incus is outside this dedicated-host baseline")
	}
	if actual.FirewallDriver != baseline.Server.FirewallDriver {
		return errors.New("server firewall driver drift detected")
	}
	if actual.Config["core.https_address"] != baseline.Server.CoreHTTPSAddress {
		return errors.New("core.https_address drift detected")
	}
	if actual.Config["cluster.https_address"] != baseline.Server.ClusterHTTPSAddress {
		return errors.New("cluster.https_address must remain empty")
	}

	for _, extension := range baseline.Server.RequiredAPIExtensions {
		if !slices.Contains(actual.APIExtensions, extension) {
			return fmt.Errorf("required Incus API extension is unavailable: %s", extension)
		}
	}
	futureExtension := baseline.ResidualControls.ProjectVMNestingRestriction.FutureAPIExtension
	if slices.Contains(actual.APIExtensions, futureExtension) {
		return fmt.Errorf(
			"server supports %s; baseline must be upgraded to enforce the project-level restriction",
			futureExtension,
		)
	}

	return nil
}

// parseVersion parses the numeric Incus version shape accepted by the baseline.
func parseVersion(value string) ([3]int, error) {
	core, _, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return [3]int{}, errors.New("version must contain two or three numeric components")
	}

	var parsed [3]int
	for index, part := range parts {
		component, err := strconv.Atoi(part)
		if err != nil || component < 0 {
			return [3]int{}, errors.New("version components must be non-negative integers")
		}
		parsed[index] = component
	}

	return parsed, nil
}

// compareVersion compares two parsed versions using major, minor, and patch order.
func compareVersion(left [3]int, right [3]int) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}

	return 0
}

// compareResource rejects any difference in a desired Incus resource projection.
func compareResource(label string, expected any, actual any) error {
	if !reflect.DeepEqual(expected, actual) {
		return fmt.Errorf("%s drift detected", label)
	}

	return nil
}

// normalizeNetworkACL makes rule ordering and empty collections deterministic.
func normalizeNetworkACL(acl NetworkACL) NetworkACL {
	acl.Config = normalizeStringMap(acl.Config)
	acl.Ingress = normalizeRules(acl.Ingress)
	acl.Egress = normalizeRules(acl.Egress)
	return acl
}

// normalizeRules copies and sorts ACL rules by their complete writable shape.
func normalizeRules(rules []NetworkACLRule) []NetworkACLRule {
	normalized := append([]NetworkACLRule(nil), rules...)
	if normalized == nil {
		normalized = []NetworkACLRule{}
	}
	sort.Slice(normalized, func(left int, right int) bool {
		return ruleSortKey(normalized[left]) < ruleSortKey(normalized[right])
	})
	return normalized
}

// ruleSortKey returns a deterministic key covering every writable ACL rule field.
func ruleSortKey(rule NetworkACLRule) string {
	data, _ := json.Marshal(rule)
	return string(data)
}

// normalizeExpectedStorage copies the desired storage state for structural comparison.
func normalizeExpectedStorage(pool StoragePool) StoragePool {
	pool.Config = normalizeStringMap(pool.Config)
	return pool
}

// normalizeObservedStorage removes only the known server-generated ZFS source field.
func normalizeObservedStorage(pool StoragePool) StoragePool {
	pool.Config = normalizeStringMap(pool.Config)
	delete(pool.Config, "volatile.initial_source")
	return pool
}

// normalizeStringMap copies a map and represents nil as an empty map.
func normalizeStringMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}

	return maps.Clone(values)
}
