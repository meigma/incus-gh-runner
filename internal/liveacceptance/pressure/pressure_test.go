package pressure_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/liveacceptance/pressure"
)

// TestRunAppliesBoundedPressure proves all workloads run without exceeding the pressure file limit.
func TestRunAppliesBoundedPressure(t *testing.T) {
	pressurePath := filepath.Join(t.TempDir(), "pressure.dat")
	diskLimit := int64(128 << 10)

	result, err := pressure.Run(context.Background(), pressure.Options{
		Duration:       100 * time.Millisecond,
		CPUWorkers:     1,
		MemoryBytes:    64 << 10,
		DiskPath:       pressurePath,
		DiskBytes:      diskLimit,
		DiskBlockBytes: 4 << 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.CPUWorkers, "expected requested CPU worker intent")
	assert.Positive(t, result.CPUHashes, "expected CPU hashing to complete work")
	assert.Equal(t, int64(64<<10), result.MemoryBytes, "expected requested memory to remain held")
	assert.Equal(t, diskLimit, result.DiskTargetBytes, "expected requested disk target intent")
	assert.Positive(t, result.DiskBytesWritten, "expected synchronous disk writes")
	assert.Positive(t, result.DiskFileBytes, "expected a materialized pressure file")
	assert.LessOrEqual(t, result.DiskFileBytes, diskLimit, "expected the pressure file to remain bounded")
	assert.Positive(t, result.Elapsed, "expected elapsed runtime to be reported")

	info, statErr := os.Stat(pressurePath)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "expected a private pressure file")
	assert.Equal(t, result.DiskFileBytes, info.Size(), "expected the reported file size to match the filesystem")
}

// TestRunReturnsPartialWorkOnCancellation proves caller cancellation stops a long pressure run promptly.
func TestRunReturnsPartialWorkOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	time.AfterFunc(25*time.Millisecond, cancel)
	startedAt := time.Now()

	result, err := pressure.Run(ctx, pressure.Options{
		Duration:   5 * time.Second,
		CPUWorkers: 1,
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Positive(t, result.CPUHashes, "expected partial CPU work before cancellation")
	assert.Less(t, time.Since(startedAt), time.Second, "expected cancellation to stop the run promptly")
}

// TestRunRejectsUnsafeOptions proves validation rejects unbounded and incomplete workloads.
func TestRunRejectsUnsafeOptions(t *testing.T) {
	tests := []struct {
		name    string
		options pressure.Options
	}{
		{
			name: "missing duration",
			options: pressure.Options{
				CPUWorkers: 1,
			},
		},
		{
			name: "duration exceeds limit",
			options: pressure.Options{
				Duration:   pressure.MaxDuration + time.Nanosecond,
				CPUWorkers: 1,
			},
		},
		{
			name: "CPU workers exceed limit",
			options: pressure.Options{
				Duration:   time.Second,
				CPUWorkers: pressure.MaxCPUWorkers + 1,
			},
		},
		{
			name: "CPU workers are negative",
			options: pressure.Options{
				Duration:   time.Second,
				CPUWorkers: -1,
			},
		},
		{
			name: "memory exceeds limit",
			options: pressure.Options{
				Duration:    time.Second,
				MemoryBytes: pressure.MaxMemoryBytes + 1,
			},
		},
		{
			name: "memory is negative",
			options: pressure.Options{
				Duration:    time.Second,
				MemoryBytes: -1,
			},
		},
		{
			name: "disk exceeds limit",
			options: pressure.Options{
				Duration:  time.Second,
				DiskPath:  filepath.Join(t.TempDir(), "pressure.dat"),
				DiskBytes: pressure.MaxDiskBytes + 1,
			},
		},
		{
			name: "disk is negative",
			options: pressure.Options{
				Duration:  time.Second,
				DiskBytes: -1,
			},
		},
		{
			name: "disk block exceeds limit",
			options: pressure.Options{
				Duration:       time.Second,
				DiskPath:       filepath.Join(t.TempDir(), "pressure.dat"),
				DiskBytes:      1,
				DiskBlockBytes: pressure.MaxDiskBlockBytes + 1,
			},
		},
		{
			name: "disk path is relative",
			options: pressure.Options{
				Duration:  time.Second,
				DiskPath:  "pressure.dat",
				DiskBytes: 1,
			},
		},
		{
			name: "disk settings without disk pressure",
			options: pressure.Options{
				Duration:   time.Second,
				CPUWorkers: 1,
				DiskPath:   filepath.Join(t.TempDir(), "pressure.dat"),
			},
		},
		{
			name: "no workload",
			options: pressure.Options{
				Duration: time.Second,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := pressure.Run(context.Background(), test.options)

			require.Error(t, err)
			assert.ErrorIs(t, err, pressure.ErrInvalidOptions, "expected an invalid-options error")
		})
	}
}

// TestRunRefusesExistingDiskPath proves disk pressure cannot overwrite caller data.
func TestRunRefusesExistingDiskPath(t *testing.T) {
	pressurePath := filepath.Join(t.TempDir(), "existing.dat")
	require.NoError(t, os.WriteFile(pressurePath, []byte("preserve me"), 0o600))

	_, err := pressure.Run(context.Background(), pressure.Options{
		Duration:       10 * time.Millisecond,
		DiskPath:       pressurePath,
		DiskBytes:      4 << 10,
		DiskBlockBytes: 4 << 10,
	})

	require.Error(t, err)
	contents, readErr := os.ReadFile(pressurePath)
	require.NoError(t, readErr)
	assert.Equal(t, "preserve me", string(contents), "expected existing data to remain unchanged")
}
