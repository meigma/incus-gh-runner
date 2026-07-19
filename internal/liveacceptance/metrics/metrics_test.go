package metrics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/liveacceptance/metrics"
)

const gibibyte = uint64(1 << 30)

// TestEvaluateEnforcesRuntimeAcceptancePolicy proves every fixed policy boundary.
func TestEvaluateEnforcesRuntimeAcceptancePolicy(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name                 string
		input                metrics.Input
		wantPassed           bool
		wantViolation        string
		wantAPIP95           time.Duration
		wantAPIMax           time.Duration
		wantPeerGap          time.Duration
		wantMinimumAvailable uint64
	}{
		{
			name: "accepts values exactly on every boundary",
			input: metrics.Input{
				HostMemoryBytes: 40 * gibibyte,
				API: append(
					repeatedProbeSamples(startedAt, 19, time.Second),
					metrics.ProbeSample{At: startedAt.Add(19 * time.Second), Latency: 5 * time.Second, Succeeded: true},
				),
				Peer: []metrics.ProbeSample{
					{At: startedAt, Succeeded: true},
					{At: startedAt.Add(5 * time.Second), Succeeded: true},
				},
				Memory: []metrics.MemorySample{
					{At: startedAt, AvailableBytes: 5 * gibibyte},
					{At: startedAt.Add(time.Second), AvailableBytes: 4 * gibibyte},
				},
			},
			wantPassed:           true,
			wantAPIP95:           time.Second,
			wantAPIMax:           5 * time.Second,
			wantPeerGap:          5 * time.Second,
			wantMinimumAvailable: 4 * gibibyte,
		},
		{
			name:                 "rejects one API failure",
			input:                validInput(startedAt),
			wantPassed:           false,
			wantViolation:        "api: failures 1 exceed limit 0",
			wantAPIP95:           500 * time.Millisecond,
			wantAPIMax:           500 * time.Millisecond,
			wantPeerGap:          time.Second,
			wantMinimumAvailable: 2 * gibibyte,
		},
		{
			name: "rejects API p95 over one second",
			input: metrics.Input{
				HostMemoryBytes: 8 * gibibyte,
				API: append(
					repeatedProbeSamples(startedAt, 18, 500*time.Millisecond),
					repeatedProbeSamples(startedAt.Add(18*time.Second), 2, time.Second+time.Millisecond)...,
				),
				Peer:   successfulPeerSamples(startedAt),
				Memory: healthyMemorySamples(startedAt),
			},
			wantPassed:           false,
			wantViolation:        "api: p95 latency 1.001s exceeds limit 1s",
			wantAPIP95:           time.Second + time.Millisecond,
			wantAPIMax:           time.Second + time.Millisecond,
			wantPeerGap:          time.Second,
			wantMinimumAvailable: 2 * gibibyte,
		},
		{
			name: "rejects API maximum over five seconds while p95 remains healthy",
			input: metrics.Input{
				HostMemoryBytes: 8 * gibibyte,
				API: append(
					repeatedProbeSamples(startedAt, 19, 500*time.Millisecond),
					metrics.ProbeSample{
						At:        startedAt.Add(19 * time.Second),
						Latency:   5*time.Second + time.Millisecond,
						Succeeded: true,
					},
				),
				Peer:   successfulPeerSamples(startedAt),
				Memory: healthyMemorySamples(startedAt),
			},
			wantPassed:           false,
			wantViolation:        "api: max latency 5.001s exceeds limit 5s",
			wantAPIP95:           500 * time.Millisecond,
			wantAPIMax:           5*time.Second + time.Millisecond,
			wantPeerGap:          time.Second,
			wantMinimumAvailable: 2 * gibibyte,
		},
		{
			name: "rejects one peer heartbeat failure",
			input: metrics.Input{
				HostMemoryBytes: 8 * gibibyte,
				API:             successfulAPISamples(startedAt),
				Peer: []metrics.ProbeSample{
					{At: startedAt, Succeeded: true},
					{At: startedAt.Add(time.Second), Succeeded: false},
					{At: startedAt.Add(2 * time.Second), Succeeded: true},
				},
				Memory: healthyMemorySamples(startedAt),
			},
			wantPassed:           false,
			wantViolation:        "peer: failures 1 exceed limit 0",
			wantAPIP95:           500 * time.Millisecond,
			wantAPIMax:           500 * time.Millisecond,
			wantPeerGap:          2 * time.Second,
			wantMinimumAvailable: 2 * gibibyte,
		},
		{
			name: "rejects a peer heartbeat gap over five seconds",
			input: metrics.Input{
				HostMemoryBytes: 8 * gibibyte,
				API:             successfulAPISamples(startedAt),
				Peer: []metrics.ProbeSample{
					{At: startedAt.Add(6 * time.Second), Succeeded: true},
					{At: startedAt, Succeeded: true},
				},
				Memory: healthyMemorySamples(startedAt),
			},
			wantPassed:           false,
			wantViolation:        "peer: max gap 6s exceeds limit 5s",
			wantAPIP95:           500 * time.Millisecond,
			wantAPIMax:           500 * time.Millisecond,
			wantPeerGap:          6 * time.Second,
			wantMinimumAvailable: 2 * gibibyte,
		},
		{
			name: "rejects MemAvailable below the two GiB absolute floor",
			input: metrics.Input{
				HostMemoryBytes: 8 * gibibyte,
				API:             successfulAPISamples(startedAt),
				Peer:            successfulPeerSamples(startedAt),
				Memory: []metrics.MemorySample{
					{At: startedAt, AvailableBytes: 2*gibibyte - 1},
				},
			},
			wantPassed:           false,
			wantViolation:        "memory: minimum MemAvailable 2147483647 bytes is below floor 2147483648 bytes",
			wantAPIP95:           500 * time.Millisecond,
			wantAPIMax:           500 * time.Millisecond,
			wantPeerGap:          time.Second,
			wantMinimumAvailable: 2*gibibyte - 1,
		},
		{
			name: "rejects MemAvailable below ten percent of a large host",
			input: metrics.Input{
				HostMemoryBytes: 40 * gibibyte,
				API:             successfulAPISamples(startedAt),
				Peer:            successfulPeerSamples(startedAt),
				Memory: []metrics.MemorySample{
					{At: startedAt, AvailableBytes: 4*gibibyte - 1},
				},
			},
			wantPassed:           false,
			wantViolation:        "memory: minimum MemAvailable 4294967295 bytes is below floor 4294967296 bytes",
			wantAPIP95:           500 * time.Millisecond,
			wantAPIMax:           500 * time.Millisecond,
			wantPeerGap:          time.Second,
			wantMinimumAvailable: 4*gibibyte - 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.name == "rejects one API failure" {
				tt.input.API[0].Succeeded = false
			}

			report := metrics.Evaluate(tt.input)

			assert.Equal(t, tt.wantPassed, report.Passed)
			assert.Equal(t, tt.wantAPIP95, report.API.P95Latency)
			assert.Equal(t, tt.wantAPIMax, report.API.MaxLatency)
			assert.Equal(t, tt.wantPeerGap, report.Peer.MaxGap)
			assert.Equal(t, tt.wantMinimumAvailable, report.Memory.MinAvailableBytes)
			if tt.wantViolation == "" {
				assert.Empty(t, report.Violations)
				return
			}
			assert.Contains(t, report.Violations, tt.wantViolation)
		})
	}
}

// TestEvaluateFailsClosedWithoutRequiredEvidence proves empty or incomplete input cannot pass.
func TestEvaluateFailsClosedWithoutRequiredEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         metrics.Input
		wantFragments []string
	}{
		{
			name: "no evidence",
			wantFragments: []string{
				"memory: host total must be positive",
				"api: no heartbeat samples",
				"peer: no heartbeat samples",
				"memory: no MemAvailable samples",
			},
		},
		{
			name: "host total is unknown",
			input: metrics.Input{
				API:    successfulAPISamples(time.Time{}),
				Peer:   successfulPeerSamples(time.Time{}),
				Memory: healthyMemorySamples(time.Time{}),
			},
			wantFragments: []string{"memory: host total must be positive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			report := metrics.Evaluate(tt.input)

			require.False(t, report.Passed)
			joined := strings.Join(report.Violations, "\n")
			for _, fragment := range tt.wantFragments {
				assert.Contains(t, joined, fragment)
			}
		})
	}
}

// TestDefaultThresholdsRoundsTenPercentUp proves fractional bytes cannot weaken the memory floor.
func TestDefaultThresholdsRoundsTenPercentUp(t *testing.T) {
	t.Parallel()

	thresholds := metrics.DefaultThresholds(20*gibibyte + 1)

	assert.Equal(t, 2*gibibyte+1, thresholds.MinMemAvailableBytes)
}

// validInput returns a complete healthy sample set for one evaluation.
func validInput(startedAt time.Time) metrics.Input {
	return metrics.Input{
		HostMemoryBytes: 8 * gibibyte,
		API:             successfulAPISamples(startedAt),
		Peer:            successfulPeerSamples(startedAt),
		Memory:          healthyMemorySamples(startedAt),
	}
}

// successfulAPISamples returns enough healthy attempts to exercise nearest-rank p95.
func successfulAPISamples(startedAt time.Time) []metrics.ProbeSample {
	return repeatedProbeSamples(startedAt, 20, 500*time.Millisecond)
}

// successfulPeerSamples returns two healthy one-second peer heartbeats.
func successfulPeerSamples(startedAt time.Time) []metrics.ProbeSample {
	return []metrics.ProbeSample{
		{At: startedAt, Latency: 10 * time.Millisecond, Succeeded: true},
		{At: startedAt.Add(time.Second), Latency: 10 * time.Millisecond, Succeeded: true},
	}
}

// healthyMemorySamples returns observations exactly on and above the absolute floor.
func healthyMemorySamples(startedAt time.Time) []metrics.MemorySample {
	return []metrics.MemorySample{
		{At: startedAt, AvailableBytes: 3 * gibibyte},
		{At: startedAt.Add(time.Second), AvailableBytes: 2 * gibibyte},
	}
}

// repeatedProbeSamples returns uniformly spaced successful probe attempts.
func repeatedProbeSamples(startedAt time.Time, count int, latency time.Duration) []metrics.ProbeSample {
	samples := make([]metrics.ProbeSample, 0, count)
	for index := range count {
		samples = append(samples, metrics.ProbeSample{
			At:        startedAt.Add(time.Duration(index) * time.Second),
			Latency:   latency,
			Succeeded: true,
		})
	}
	return samples
}
