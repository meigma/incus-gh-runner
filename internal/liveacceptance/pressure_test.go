package liveacceptance

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	guestpressure "github.com/meigma/incus-gh-runner/internal/liveacceptance/pressure"
)

// pressureIncusCall records one project-scoped command received by a pressure test stub.
type pressureIncusCall struct {
	project   string
	arguments []string
}

// pressureIncusStub is a concurrency-safe guest-command boundary for lifecycle tests.
type pressureIncusStub struct {
	mu      sync.Mutex
	calls   []pressureIncusCall
	handler func(context.Context, string, ...string) (commandResult, error)
}

// incus records and handles one project-scoped guest command.
func (s *pressureIncusStub) incus(
	ctx context.Context,
	project string,
	arguments ...string,
) (commandResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, pressureIncusCall{project: project, arguments: append([]string(nil), arguments...)})
	s.mu.Unlock()
	if s.handler == nil {
		return commandResult{}, nil
	}
	return s.handler(ctx, project, arguments...)
}

// snapshot returns an immutable copy of the calls observed by the pressure stub.
func (s *pressureIncusStub) snapshot() []pressureIncusCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]pressureIncusCall, len(s.calls))
	copy(result, s.calls)
	return result
}

// TestGuestPressureArtifactsAreStableAndRunScoped proves retries cannot consume another run's fixed files.
func TestGuestPressureArtifactsAreStableAndRunScoped(t *testing.T) {
	t.Parallel()

	first := newGuestPressureArtifacts("acceptance-one")
	second := newGuestPressureArtifacts("acceptance-two")
	assert.Equal(t, first, newGuestPressureArtifacts("acceptance-one"))
	assert.NotEqual(t, first, second)
	assert.True(t, strings.HasSuffix(first.unit, ".service"))
	assert.NotContains(t, first.executable, "acceptance-one")
	assert.True(t, strings.HasPrefix(first.executable, "/root/"))
	assert.False(t, strings.HasPrefix(first.executable, "/run/"))
}

// TestClassifyGuestPressureStatusRejectsAmbiguousResults proves only the explicit completion envelope is accepted.
func TestClassifyGuestPressureStatusRejectsAmbiguousResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		ready     bool
		output    string
		errorText string
	}{
		{name: "ready", raw: "ready\n{\"cpu_hashes\":1}", ready: true, output: "{\"cpu_hashes\":1}"},
		{name: "pending", raw: "pending\n"},
		{name: "failed", raw: "failed\nallocation failed", errorText: "allocation failed"},
		{name: "unknown", raw: "unknown\ndetail", errorText: "unknown"},
		{name: "missing delimiter", raw: "ready", errorText: "delimiter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			output, ready, err := classifyGuestPressureStatus(tt.raw)
			if tt.errorText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorText)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.ready, ready)
			assert.Equal(t, tt.output, string(output))
		})
	}
}

// TestValidatePressureResultRequiresFullDuration proves a short workload cannot close the resource-survival gate.
func TestValidatePressureResultRequiresFullDuration(t *testing.T) {
	t.Parallel()

	result := guestpressure.Result{
		CPUWorkers:       1,
		CPUHashes:        1,
		MemoryBytes:      1,
		DiskTargetBytes:  1,
		DiskBytesWritten: 1,
		DiskFileBytes:    1,
		Elapsed:          9 * time.Minute,
	}
	workload := guestPressureWorkload{cpuWorkers: 1, memoryBytes: 1, diskBytes: 1}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.Error(t, validatePressureResult(data, 10*time.Minute, workload))

	result.Elapsed = 10 * time.Minute
	data, err = json.Marshal(result)
	require.NoError(t, err)
	assert.NoError(t, validatePressureResult(data, 10*time.Minute, workload))
}

// TestStartGuestPressureBindsRunScopedSystemdArtifacts proves the detached unit cannot reuse stale fixed output.
func TestStartGuestPressureBindsRunScopedSystemdArtifacts(t *testing.T) {
	t.Parallel()

	commands := &pressureIncusStub{}
	options := Options{Project: "runtime-acceptance", VMA: "runner-a", StressDuration: 10 * time.Minute}
	artifacts := newGuestPressureArtifacts("run-one")
	workload := guestPressureWorkload{cpuWorkers: 8, memoryBytes: 2 << 30, diskBytes: 1 << 30}
	require.NoError(t, startGuestPressure(context.Background(), commands, options, workload, artifacts))

	calls := commands.snapshot()
	require.Len(t, calls, 2)
	startArguments := strings.Join(calls[0].arguments, "\x00")
	assert.Contains(t, startArguments, "StandardOutput=truncate:"+artifacts.result)
	assert.Contains(t, startArguments, "StandardError=truncate:"+artifacts.stderr)
	assert.Contains(t, startArguments, "TimeoutStopSec=10s")
	assert.Contains(t, startArguments, artifacts.unit)
	assert.Contains(t, startArguments, artifacts.executable)
	assert.Contains(t, startArguments, artifacts.disk)
	assert.Contains(t, startArguments, "\x008\x00")
	assert.Contains(t, startArguments, "\x002147483648\x00")
	assert.Contains(t, startArguments, "\x001073741824")
}

// TestCleanupGuestPressureUnlinksBeforeConvergence proves a delayed start loses its executable before unit checks.
func TestCleanupGuestPressureUnlinksBeforeConvergence(t *testing.T) {
	t.Parallel()

	commands := &pressureIncusStub{}
	options := Options{Project: "runtime-acceptance", VMA: "runner-a"}
	artifacts := newGuestPressureArtifacts("run-one")
	require.NoError(t, cleanupGuestPressure(context.Background(), commands, options, artifacts))

	calls := commands.snapshot()
	require.Len(t, calls, 1)
	require.Greater(t, len(calls[0].arguments), 5)
	script := calls[0].arguments[5]
	unlinkAt := strings.Index(script, `rm -f -- "$executable"`)
	stateAt := strings.Index(script, "read_unit_state\nif unit_stopped")
	require.NotEqual(t, -1, unlinkAt)
	require.NotEqual(t, -1, stateAt)
	assert.Less(t, unlinkAt, stateAt)
	assert.Contains(t, script, "--property=LoadState --property=ActiveState --property=MainPID")
	assert.NotContains(t, script, "systemctl show \"$unit\" --property=ActiveState --value 2>/dev/null || true")
	assert.Equal(t, artifacts.unit, calls[0].arguments[7])
}

// TestRunEgressWatchdogPublishesInFlightFailure proves scheduling cancellation cannot discard a boundary failure.
func TestRunEgressWatchdogPublishesInFlightFailure(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	commands := &pressureIncusStub{
		handler: func(context.Context, string, ...string) (commandResult, error) {
			close(started)
			<-release
			return commandResult{exitCode: 1, stderr: []byte("egress denied")}, nil
		},
	}
	scheduleContext, cancelSchedule := context.WithCancel(context.Background())
	results := make(chan egressWatchdogSample, 1)
	go runEgressWatchdog(
		scheduleContext,
		context.Background(),
		commands,
		Options{
			Project:      "runtime-acceptance",
			VMB:          "runner-b",
			AllowedURL:   "https://github.com/",
			PollInterval: time.Second,
		},
		results,
	)
	<-started
	cancelSchedule()
	close(release)

	sample, open := <-results
	require.True(t, open)
	assert.False(t, sample.Succeeded)
	_, open = <-results
	assert.False(t, open)
}
