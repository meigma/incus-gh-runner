package incuspolicy

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateBaselineAcceptsRenderedPolicy proves the embedded schema accepts supported output.
func TestValidateBaselineAcceptsRenderedPolicy(t *testing.T) {
	t.Parallel()

	baseline := readPolicyFixture(t)
	require.NoError(t, ValidateBaseline("baseline.example.json", baseline))

	custom := decodePolicyFixture(t)
	projectConfig := policyObject(t, custom, "project", "config")
	projectConfig["limits.cpu"] = "12"
	projectConfig["limits.disk"] = "120GiB"
	projectConfig["limits.disk.pool.runner-storage"] = "120GiB"
	projectConfig["limits.instances"] = "3"
	projectConfig["limits.memory"] = "24GiB"
	projectConfig["limits.virtual-machines"] = "3"
	profileConfig := policyObject(t, custom, "profile", "config")
	profileConfig["limits.cpu"] = "4"
	profileConfig["limits.memory"] = "8GiB"
	policyObject(t, custom, "profile", "devices", "eth0")["limits.max"] = "250Mbit"
	rootDevice := policyObject(t, custom, "profile", "devices", "root")
	rootDevice["size"] = "30GiB"
	rootDevice["limits.max"] = "150MiB"
	policyRules(t, custom)[2].(map[string]any)["destination_port"] = "8080"
	require.NoError(t, ValidateBaseline("custom.json", encodePolicyFixture(t, custom)))
}

// TestValidateBaselineRejectsWeakening proves the former shell policy matrix in-process.
func TestValidateBaselineRejectsWeakening(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(t *testing.T, baseline map[string]any)
	}{
		{
			name: "unsupported authority",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "authority")["mode"] = "project-restricted-loopback-tls"
			},
		},
		{
			name: "missing fixed authority field",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "authority"), "unix_socket_is_root_equivalent")
			},
		},
		{
			name: "exposed API",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "server")["core_https_address"] = "127.0.0.1:8443"
			},
		},
		{
			name: "indirect ACL only",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "profile", "devices", "eth0"), "security.acls")
			},
		},
		{
			name: "project-local network",
			mutate: func(t *testing.T, baseline map[string]any) {
				config := policyObject(t, baseline, "project", "config")
				config["features.networks"] = "true"
				config["limits.networks"] = "1"
			},
		},
		{
			name: "missing network ACL",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "network", "config"), "security.acls")
			},
		},
		{
			name: "permissive network default",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "network", "config")["security.acls.default.egress.action"] = "allow"
			},
		},
		{
			name: "unlogged NIC default",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "profile", "devices", "eth0")["security.acls.default.egress.logged"] = "false"
			},
		},
		{
			name: "missing explicit IPv6 denial",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "profile", "devices", "eth0"), "ipv6.address")
			},
		},
		{
			name: "weakened explicit IPv6 denial",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "profile", "devices", "eth0")["ipv6.address"] = "auto"
			},
		},
		{
			name: "weakened project restriction",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "project", "config")["restricted.devices.unix-char"] = "allow"
			},
		},
		{
			name: "missing API extension",
			mutate: func(t *testing.T, baseline map[string]any) {
				server := policyObject(t, baseline, "server")
				extensions := server["required_api_extensions"].([]any)
				server["required_api_extensions"] = extensions[1:]
			},
		},
		{
			name: "unexpected project key",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "project", "config")["restricted.devices.nic_typo"] = "managed"
			},
		},
		{
			name: "missing storage source",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "storage_pool", "config"), "source")
			},
		},
		{
			name: "wide ACL destination",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyRules(t, baseline)[0].(map[string]any)["destination"] = "0.0.0.0/0"
			},
		},
		{
			name: "malformed ACL destination",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyRules(t, baseline)[0].(map[string]any)["destination"] = "999.0.0.1/32"
			},
		},
		{
			name: "proxy on DNS port",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyRules(t, baseline)[2].(map[string]any)["destination_port"] = "53"
			},
		},
		{
			name: "invalid proxy port",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyRules(t, baseline)[2].(map[string]any)["destination_port"] = "65536"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseline := decodePolicyFixture(t)
			tt.mutate(t, baseline)
			err := ValidateBaseline("invalid.json", encodePolicyFixture(t, baseline))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "baseline violates CUE policy")
		})
	}
}

// TestValidateBaselineRejectsInvalidInput proves parser and size limits fail before validation.
func TestValidateBaselineRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		data  []byte
		match string
	}{
		{name: "empty", data: nil, match: "baseline is empty"},
		{name: "malformed JSON", data: []byte("{"), match: "parse baseline JSON"},
		{name: "oversized", data: make([]byte, MaximumBaselineBytes+1), match: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateBaseline("invalid.json", tt.data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.match)
		})
	}
}

// readPolicyFixture reads the checked-in CUE-rendered baseline.
func readPolicyFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("baseline.example.json")
	require.NoError(t, err)
	return data
}

// decodePolicyFixture returns a mutable generic representation of the baseline.
func decodePolicyFixture(t *testing.T) map[string]any {
	t.Helper()
	var baseline map[string]any
	require.NoError(t, json.Unmarshal(readPolicyFixture(t), &baseline))
	return baseline
}

// encodePolicyFixture serializes a mutated baseline for policy validation.
func encodePolicyFixture(t *testing.T, baseline map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(baseline)
	require.NoError(t, err)
	return data
}

// policyObject traverses a fixture path and returns its object value.
func policyObject(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	current := root
	for _, name := range path {
		next, ok := current[name].(map[string]any)
		require.True(t, ok, "expected %q to be an object", name)
		current = next
	}
	return current
}

// policyRules returns the fixture's egress ACL rule list.
func policyRules(t *testing.T, baseline map[string]any) []any {
	t.Helper()
	rules, ok := policyObject(t, baseline, "network_acl")["egress"].([]any)
	require.True(t, ok, "expected egress to be an array")
	return rules
}
