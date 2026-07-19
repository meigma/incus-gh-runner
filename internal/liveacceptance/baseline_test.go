package liveacceptance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadRuntimeBaselineDerivesAcceptanceLimits proves the checked-in policy is the probe's source of truth.
func TestLoadRuntimeBaselineDerivesAcceptanceLimits(t *testing.T) {
	t.Parallel()

	baseline, err := LoadRuntimeBaseline(filepath.Join("..", "..", "deploy", "incus", "baseline.example.json"))
	require.NoError(t, err)

	assert.Equal(t, 10, baseline.MaximumRunners)
	assert.Equal(t, 2, baseline.RunnerCPU)
	assert.Equal(t, uint64(4<<30), baseline.RunnerMemoryBytes)
	assert.Equal(t, uint64(20<<30), baseline.RunnerRootDiskBytes)
	assert.Equal(t, uint64(100_000_000), baseline.RunnerNetworkBitsPerSecond)
	assert.Equal(t, uint64(100<<20), baseline.RunnerDiskBytesPerSecond)
	assert.Equal(t, "none", baseline.Manifest.Profile.Devices["eth0"]["ipv6.address"])
}

// TestLoadRuntimeBaselineRejectsNonPolicyInputs proves the live probe cannot bypass CUE validation.
func TestLoadRuntimeBaselineRejectsNonPolicyInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, directory string) string
		match   string
	}{
		{
			name: "directory",
			prepare: func(_ *testing.T, directory string) string {
				return directory
			},
			match: "regular file",
		},
		{
			name: "unknown manifest",
			prepare: func(t *testing.T, directory string) string {
				return writeBaselineTestFile(t, directory, []byte(`{"unexpected":true}`))
			},
			match: "violates CUE policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.prepare(t, t.TempDir())
			_, err := LoadRuntimeBaseline(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.match)
		})
	}
}

// TestAcceptanceManifestBindsDisposableMarkerWithoutMutatingPolicy proves live safety metadata stays test-local.
func TestAcceptanceManifestBindsDisposableMarkerWithoutMutatingPolicy(t *testing.T) {
	t.Parallel()

	baseline, err := LoadRuntimeBaseline(filepath.Join("..", "..", "deploy", "incus", "baseline.example.json"))
	require.NoError(t, err)

	manifest := acceptanceManifest(baseline.Manifest)
	assert.NotContains(t, baseline.Manifest.Project.Config, disposableKey)
	assert.Equal(t, trueValue, manifest.Project.Config[disposableKey])
}

// writeBaselineTestFile writes one isolated invalid baseline fixture.
func writeBaselineTestFile(t *testing.T, directory string, data []byte) string {
	t.Helper()
	path := filepath.Join(directory, "baseline.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}
