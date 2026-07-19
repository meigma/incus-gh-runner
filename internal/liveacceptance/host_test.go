package liveacceptance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMemoryStateParsesRequiredKernelValues proves watchdog memory samples use bytes consistently.
func TestReadMemoryStateParsesRequiredKernelValues(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "meminfo")
	require.NoError(t, os.WriteFile(path, []byte("MemTotal: 32768 kB\nMemFree: 1 kB\nMemAvailable: 4096 kB\n"), 0o600))
	observedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	state, err := readMemoryState(path, observedAt)
	require.NoError(t, err)
	assert.Equal(t, uint64(32768*1024), state.TotalBytes)
	assert.Equal(t, uint64(4096*1024), state.AvailableBytes)
	assert.Equal(t, observedAt, state.ObservedAt)
}

// TestParseDaemonStateIgnoresPropertyOrder proves restart detection is stable across systemd output ordering.
func TestParseDaemonStateIgnoresPropertyOrder(t *testing.T) {
	t.Parallel()

	state, err := parseDaemonState([]byte("NRestarts=3\nMainPID=1842\n"))
	require.NoError(t, err)
	assert.Equal(t, daemonState{MainPID: 1842, NRestarts: 3}, state)
}

// TestKernelLogHasResourceFailureRecognizesInvalidatingEvents proves benign logs do not mask host failures.
func TestKernelLogHasResourceFailureRecognizesInvalidatingEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		log  string
		want bool
	}{
		{name: "benign", log: "incus bridge initialized", want: false},
		{name: "OOM", log: "invoked oom-killer: gfp_mask=0x0", want: true},
		{name: "hung task", log: "task blocked for more than 120 seconds", want: true},
		{name: "disk failure", log: "critical I/O error on device", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, kernelLogHasResourceFailure([]byte(tt.log)))
		})
	}
}
