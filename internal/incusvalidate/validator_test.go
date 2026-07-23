package incusvalidate

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotReader is an in-memory read port used by validator behavior tests.
type snapshotReader struct {
	snapshot Snapshot
	err      error
	names    Names
}

// Read returns the configured snapshot and records the requested names.
func (r *snapshotReader) Read(_ context.Context, names Names) (Snapshot, error) {
	r.names = names
	return r.snapshot, r.err
}

// TestParseBaselineUsesPolicyAndExactJSON proves both policy and strict decoding boundaries.
func TestParseBaselineUsesPolicyAndExactJSON(t *testing.T) {
	t.Parallel()

	data := readBaselineFixture(t)
	policyCalled := false
	baseline, err := ParseBaseline("fixture.json", data, func(filename string, candidate []byte) error {
		policyCalled = true
		assert.Equal(t, "fixture.json", filename)
		assert.Equal(t, data, candidate)
		return nil
	})

	require.NoError(t, err)
	assert.True(t, policyCalled)
	assert.Equal(t, "github-runners", baseline.Names.Project)
}

// TestParseBaselineRejectsInvalidBoundaries proves policy failures and trailing data fail closed.
func TestParseBaselineRejectsInvalidBoundaries(t *testing.T) {
	t.Parallel()

	data := readBaselineFixture(t)
	tests := []struct {
		name   string
		data   []byte
		policy PolicyValidator
		match  string
	}{
		{
			name:  "missing policy",
			data:  data,
			match: "policy validator is required",
		},
		{
			name: "policy rejection",
			data: data,
			policy: func(string, []byte) error {
				return errors.New("policy rejected baseline")
			},
			match: "policy rejected baseline",
		},
		{
			name:   "trailing JSON value",
			data:   append(append([]byte(nil), data...), []byte("\n{}\n")...),
			policy: func(string, []byte) error { return nil },
			match:  "more than one JSON value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseBaseline("fixture.json", tt.data, tt.policy)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.match)
		})
	}
}

// TestValidateComparesReadOnlySnapshot proves the complete drift and server requirement matrix.
func TestValidateComparesReadOnlySnapshot(t *testing.T) {
	t.Parallel()

	baseline := decodeBaselineFixture(t)
	tests := []struct {
		name    string
		mutate  func(snapshot *Snapshot)
		readErr error
		match   string
	}{
		{name: "matching baseline"},
		{
			name: "order-independent ACL rules",
			mutate: func(snapshot *Snapshot) {
				snapshot.NetworkACL.Egress[0], snapshot.NetworkACL.Egress[2] =
					snapshot.NetworkACL.Egress[2], snapshot.NetworkACL.Egress[0]
			},
		},
		{
			name: "missing API extension",
			mutate: func(snapshot *Snapshot) {
				snapshot.Server.APIExtensions = snapshot.Server.APIExtensions[1:]
			},
			match: "required Incus API extension is unavailable",
		},
		{
			name: "future nesting extension",
			mutate: func(snapshot *Snapshot) {
				snapshot.Server.APIExtensions = append(
					snapshot.Server.APIExtensions,
					baseline.ResidualControls.ProjectVMNestingRestriction.FutureAPIExtension,
				)
			},
			match: "baseline must be upgraded",
		},
		{
			name: "exposed API",
			mutate: func(snapshot *Snapshot) {
				snapshot.Server.Config["core.https_address"] = "0.0.0.0:8443"
			},
			match: "core.https_address drift detected",
		},
		{
			name: "old server",
			mutate: func(snapshot *Snapshot) {
				snapshot.Server.Version = "6.0.4"
			},
			match: "incus 7.0 or newer is required",
		},
		{
			name: "untrusted API view",
			mutate: func(snapshot *Snapshot) {
				snapshot.Server.Auth = "untrusted"
			},
			match: "trusted read-only API view",
		},
		{
			name: "clustered server",
			mutate: func(snapshot *Snapshot) {
				snapshot.Server.Clustered = true
			},
			match: "clustered Incus is outside",
		},
		{
			name: "firewall drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.Server.FirewallDriver = "xtables"
			},
			match: "firewall driver drift",
		},
		{
			name: "project drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.Project.Config["restricted"] = "false"
			},
			match: "project drift detected",
		},
		{
			name: "network drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.Network.Config["ipv4.nat"] = "false"
			},
			match: "network drift detected",
		},
		{
			name: "ACL drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.NetworkACL.Egress[0].Source = "10.0.0.0/8"
			},
			match: "network ACL drift detected",
		},
		{
			name: "profile extra device",
			mutate: func(snapshot *Snapshot) {
				snapshot.Profile.Devices["gpu"] = map[string]string{"type": "gpu"}
			},
			match: "profile drift detected",
		},
		{
			name: "storage source drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["source"] = "unexpected-zpool"
			},
			match: "storage pool drift detected",
		},
		{
			name: "unexpected volatile storage key",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["volatile.unexpected"] = "value"
			},
			match: "storage pool drift detected",
		},
		{
			name:    "query failure",
			readErr: errors.New("read server: unavailable"),
			match:   "read Incus state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshot := validSnapshot(baseline)
			if tt.mutate != nil {
				tt.mutate(&snapshot)
			}
			reader := &snapshotReader{snapshot: snapshot, err: tt.readErr}

			result, err := Validate(context.Background(), baseline, reader)
			if tt.match != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.match)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, baseline.Names, reader.names)
			assert.Equal(t, []string{nestingCompatibilityNotice}, result.Notices)
		})
	}
}

// TestValidateLVMStorageSnapshot proves only Incus's generated initial-source key is normalized.
func TestValidateLVMStorageSnapshot(t *testing.T) {
	t.Parallel()

	baseline := decodeBaselineFixtureFile(t, "../../deploy/incus/baseline.lvm.example.json")
	tests := []struct {
		name   string
		mutate func(snapshot *Snapshot)
	}{
		{name: "matching snapshot with initial source"},
		{
			name: "description drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Description = "General-purpose LVM pool"
			},
		},
		{
			name: "driver drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Driver = "zfs"
			},
		},
		{
			name: "source drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["source"] = "other-vg"
			},
		},
		{
			name: "volume group drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["lvm.vg_name"] = "other-vg"
			},
		},
		{
			name: "thin pool drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["lvm.thinpool_name"] = "other-thinpool"
			},
		},
		{
			name: "volume size drift",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["volume.size"] = "64GiB"
			},
		},
		{
			name: "unexpected volatile key",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["volatile.unexpected"] = "value"
			},
		},
		{
			name: "unexpected configuration key",
			mutate: func(snapshot *Snapshot) {
				snapshot.StoragePool.Config["lvm.use_thinpool"] = "true"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshot := validSnapshot(baseline)
			if tt.mutate != nil {
				tt.mutate(&snapshot)
			}

			_, err := Validate(context.Background(), baseline, &snapshotReader{snapshot: snapshot})
			if tt.mutate == nil {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), "storage pool drift detected")
		})
	}
}

// readBaselineFixture reads the checked-in CUE-rendered baseline.
func readBaselineFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../deploy/incus/baseline.example.json")
	require.NoError(t, err)
	return data
}

// decodeBaselineFixture decodes the checked-in baseline for comparison tests.
func decodeBaselineFixture(t *testing.T) Baseline {
	t.Helper()
	return decodeBaselineFixtureFile(t, "../../deploy/incus/baseline.example.json")
}

// decodeBaselineFixtureFile decodes one checked-in baseline for comparison tests.
func decodeBaselineFixtureFile(t *testing.T, filename string) Baseline {
	t.Helper()
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	var baseline Baseline
	require.NoError(t, json.Unmarshal(data, &baseline))
	return baseline
}

// validSnapshot constructs live state matching a parsed baseline.
func validSnapshot(baseline Baseline) Snapshot {
	storageConfig := maps.Clone(baseline.StoragePool.Config)
	storageConfig["volatile.initial_source"] = storageConfig["source"]
	return Snapshot{
		Server: ServerState{
			Auth:           "trusted",
			APIExtensions:  append([]string(nil), baseline.Server.RequiredAPIExtensions...),
			Config:         map[string]string{},
			Version:        "7.0.1",
			Clustered:      false,
			FirewallDriver: baseline.Server.FirewallDriver,
		},
		Project: Project{
			Description: baseline.Project.Description,
			Config:      maps.Clone(baseline.Project.Config),
		},
		Network: Network{
			Description: baseline.Network.Description,
			Type:        baseline.Network.Type,
			Managed:     baseline.Network.Managed,
			Config:      maps.Clone(baseline.Network.Config),
		},
		NetworkACL: NetworkACL{
			Description: baseline.NetworkACL.Description,
			Config:      maps.Clone(baseline.NetworkACL.Config),
			Ingress:     append([]NetworkACLRule(nil), baseline.NetworkACL.Ingress...),
			Egress:      append([]NetworkACLRule(nil), baseline.NetworkACL.Egress...),
		},
		Profile: Profile{
			Description: baseline.Profile.Description,
			Config:      maps.Clone(baseline.Profile.Config),
			Devices:     cloneTestDevices(baseline.Profile.Devices),
		},
		StoragePool: StoragePool{
			Description: baseline.StoragePool.Description,
			Driver:      baseline.StoragePool.Driver,
			Config:      storageConfig,
		},
	}
}

// cloneTestDevices returns a deep device-map copy for mutation tests.
func cloneTestDevices(devices map[string]map[string]string) map[string]map[string]string {
	cloned := make(map[string]map[string]string, len(devices))
	for name, config := range devices {
		cloned[name] = maps.Clone(config)
	}
	return cloned
}
