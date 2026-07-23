package incuspolicy

import (
	"encoding/json"
	"os"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateBaselineAcceptsRenderedPolicy proves the embedded schema accepts supported output.
func TestValidateBaselineAcceptsRenderedPolicy(t *testing.T) {
	t.Parallel()

	baseline := readPolicyFixture(t, "baseline.example.json")
	require.NoError(t, ValidateBaseline("baseline.example.json", baseline))
	lvmBaseline := readPolicyFixture(t, "baseline.lvm.example.json")
	require.NoError(t, ValidateBaseline("baseline.lvm.example.json", lvmBaseline))

	custom := decodePolicyFixture(t, "baseline.example.json")
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
			name: "overlong managed bridge name",
			mutate: func(t *testing.T, baseline map[string]any) {
				const network = "runner-network-x"
				policyObject(t, baseline, "names")["network"] = network
				policyObject(t, baseline, "project", "config")["restricted.networks.access"] = network
				policyObject(t, baseline, "profile", "devices", "eth0")["network"] = network
			},
		},
		{
			name: "short managed bridge name",
			mutate: func(t *testing.T, baseline map[string]any) {
				const network = "a"
				policyObject(t, baseline, "names")["network"] = network
				policyObject(t, baseline, "project", "config")["restricted.networks.access"] = network
				policyObject(t, baseline, "profile", "devices", "eth0")["network"] = network
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
			baseline := decodePolicyFixture(t, "baseline.example.json")
			tt.mutate(t, baseline)
			err := ValidateBaseline("invalid.json", encodePolicyFixture(t, baseline))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "baseline violates CUE policy")
		})
	}
}

// TestValidateBaselineRejectsInvalidLVMStorage proves the LVM policy remains an exact closed variant.
func TestValidateBaselineRejectsInvalidLVMStorage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(t *testing.T, baseline map[string]any)
	}{
		{
			name: "unsupported driver",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "storage_pool")["driver"] = "btrfs"
			},
		},
		{
			name: "mixed ZFS key",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "storage_pool", "config")["zfs.pool_name"] = "incus-vg"
			},
		},
		{
			name: "missing source",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "storage_pool", "config"), "source")
			},
		},
		{
			name: "missing volume group",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "storage_pool", "config"), "lvm.vg_name")
			},
		},
		{
			name: "missing thin pool",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "storage_pool", "config"), "lvm.thinpool_name")
			},
		},
		{
			name: "missing volume size",
			mutate: func(t *testing.T, baseline map[string]any) {
				delete(policyObject(t, baseline, "storage_pool", "config"), "volume.size")
			},
		},
		{
			name: "source and volume group mismatch",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "storage_pool", "config")["lvm.vg_name"] = "other-vg"
			},
		},
		{
			name: "zero volume size",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "storage_pool", "config")["volume.size"] = "0GiB"
			},
		},
		{
			name: "invalid volume size unit",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "storage_pool", "config")["volume.size"] = "32GB"
			},
		},
		{
			name: "unexpected storage key",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "storage_pool", "config")["lvm.use_thinpool"] = "true"
			},
		},
		{
			name: "mutated description",
			mutate: func(t *testing.T, baseline map[string]any) {
				policyObject(t, baseline, "storage_pool")["description"] = "General-purpose LVM pool"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseline := decodePolicyFixture(t, "baseline.lvm.example.json")
			tt.mutate(t, baseline)
			err := ValidateBaseline("invalid-lvm.json", encodePolicyFixture(t, baseline))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "baseline violates CUE policy")
		})
	}
}

// TestCUEExamplesMatchJSONFixtures proves both checked-in examples render their fixtures semantically.
func TestCUEExamplesMatchJSONFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		packagePath string
		fixture     string
	}{
		{name: "default ZFS", packagePath: "./examples/default", fixture: "baseline.example.json"},
		{name: "LVM thin pool", packagePath: "./examples/lvm", fixture: "baseline.lvm.example.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			instances := load.Instances([]string{tt.packagePath}, &load.Config{Dir: "cue"})
			require.Len(t, instances, 1)
			value := cuecontext.New().BuildInstance(instances[0])
			require.NoError(t, value.Err())
			rendered, err := value.LookupPath(cue.ParsePath("baseline")).MarshalJSON()
			require.NoError(t, err)
			assert.JSONEq(t, string(readPolicyFixture(t, tt.fixture)), string(rendered))
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

// readPolicyFixture reads one checked-in CUE-rendered baseline.
func readPolicyFixture(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	return data
}

// decodePolicyFixture returns a mutable generic representation of one baseline.
func decodePolicyFixture(t *testing.T, filename string) map[string]any {
	t.Helper()
	var baseline map[string]any
	require.NoError(t, json.Unmarshal(readPolicyFixture(t, filename), &baseline))
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
