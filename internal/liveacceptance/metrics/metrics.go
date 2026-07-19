// Package metrics evaluates runtime-acceptance health samples against the
// availability and host-headroom policy used by the live Incus harness.
package metrics

import (
	"fmt"
	"slices"
	"time"
)

const (
	// DefaultAPIP95Limit is the greatest accepted 95th-percentile API latency.
	DefaultAPIP95Limit = time.Second
	// DefaultAPIMaxLimit is the greatest accepted individual API latency.
	DefaultAPIMaxLimit = 5 * time.Second
	// DefaultPeerGapLimit is the greatest accepted gap between peer heartbeats.
	DefaultPeerGapLimit = 5 * time.Second
	// DefaultMinimumMemAvailableBytes is the absolute host-memory headroom floor.
	DefaultMinimumMemAvailableBytes uint64 = 2 << 30

	defaultMaximumFailures = 0
	apiLatencyPercentile   = 95
	percentileBase         = 100
	memoryFractionDivisor  = 10
)

// ProbeSample records one API or peer heartbeat attempt.
type ProbeSample struct {
	// At is the wall-clock time at which the attempt completed.
	At time.Time `json:"at"`
	// Latency is the elapsed time from attempt start through completion.
	Latency time.Duration `json:"latency"`
	// Succeeded reports whether the heartbeat completed without error.
	Succeeded bool `json:"succeeded"`
}

// MemorySample records one host MemAvailable observation.
type MemorySample struct {
	// At is the wall-clock time at which MemAvailable was read.
	At time.Time `json:"at"`
	// AvailableBytes is the observed MemAvailable value in bytes.
	AvailableBytes uint64 `json:"available_bytes"`
}

// Input contains the sample series and host capacity needed for evaluation.
type Input struct {
	// HostMemoryBytes is the host's total physical memory in bytes.
	HostMemoryBytes uint64 `json:"host_memory_bytes"`
	// API contains the Incus API heartbeat attempts, normally sampled at 1 Hz.
	API []ProbeSample `json:"api"`
	// Peer contains the unaffected-runner heartbeat attempts, normally sampled at 1 Hz.
	Peer []ProbeSample `json:"peer"`
	// Memory contains host MemAvailable observations, normally sampled at 1 Hz.
	Memory []MemorySample `json:"memory"`
}

// Thresholds records the fixed runtime-acceptance policy used for a host.
type Thresholds struct {
	// APIMaxFailures is the greatest accepted number of failed API heartbeats.
	APIMaxFailures int `json:"api_max_failures"`
	// APIP95Latency is the greatest accepted 95th-percentile API latency.
	APIP95Latency time.Duration `json:"api_p95_latency"`
	// APIMaxLatency is the greatest accepted individual API latency.
	APIMaxLatency time.Duration `json:"api_max_latency"`
	// PeerMaxFailures is the greatest accepted number of failed peer heartbeats.
	PeerMaxFailures int `json:"peer_max_failures"`
	// PeerMaxGap is the greatest accepted gap between successful peer heartbeats.
	PeerMaxGap time.Duration `json:"peer_max_gap"`
	// MinMemAvailableBytes is the required host MemAvailable floor in bytes.
	MinMemAvailableBytes uint64 `json:"min_mem_available_bytes"`
}

// APISummary contains the computed API heartbeat measurements.
type APISummary struct {
	// SampleCount is the number of evaluated API attempts.
	SampleCount int `json:"sample_count"`
	// FailureCount is the number of API attempts that did not succeed.
	FailureCount int `json:"failure_count"`
	// P95Latency is the nearest-rank 95th-percentile latency.
	P95Latency time.Duration `json:"p95_latency"`
	// MaxLatency is the greatest observed latency.
	MaxLatency time.Duration `json:"max_latency"`
}

// PeerSummary contains the computed unaffected-runner heartbeat measurements.
type PeerSummary struct {
	// SampleCount is the number of evaluated peer heartbeat attempts.
	SampleCount int `json:"sample_count"`
	// FailureCount is the number of peer heartbeat attempts that did not succeed.
	FailureCount int `json:"failure_count"`
	// MaxGap is the greatest observed interval without a successful peer heartbeat.
	MaxGap time.Duration `json:"max_gap"`
}

// MemorySummary contains the computed host-memory measurements.
type MemorySummary struct {
	// SampleCount is the number of evaluated MemAvailable observations.
	SampleCount int `json:"sample_count"`
	// MinAvailableBytes is the lowest observed MemAvailable value in bytes.
	MinAvailableBytes uint64 `json:"min_available_bytes"`
}

// Report contains computed measurements and all failed policy checks.
type Report struct {
	// Passed reports whether every acceptance threshold was satisfied.
	Passed bool `json:"passed"`
	// Thresholds records the policy applied during evaluation.
	Thresholds Thresholds `json:"thresholds"`
	// API contains the evaluated Incus API heartbeat measurements.
	API APISummary `json:"api"`
	// Peer contains the evaluated unaffected-runner heartbeat measurements.
	Peer PeerSummary `json:"peer"`
	// Memory contains the evaluated host-memory measurements.
	Memory MemorySummary `json:"memory"`
	// Violations contains deterministic descriptions of failed policy checks.
	Violations []string `json:"violations"`
}

// DefaultThresholds returns the acceptance policy for the supplied host memory.
func DefaultThresholds(hostMemoryBytes uint64) Thresholds {
	return Thresholds{
		APIMaxFailures:       defaultMaximumFailures,
		APIP95Latency:        DefaultAPIP95Limit,
		APIMaxLatency:        DefaultAPIMaxLimit,
		PeerMaxFailures:      defaultMaximumFailures,
		PeerMaxGap:           DefaultPeerGapLimit,
		MinMemAvailableBytes: memoryAvailableFloor(hostMemoryBytes),
	}
}

// Evaluate summarizes samples and applies the default runtime-acceptance policy.
func Evaluate(input Input) Report {
	report := Report{
		Thresholds: DefaultThresholds(input.HostMemoryBytes),
		API:        summarizeAPI(input.API),
		Peer:       summarizePeer(input.Peer),
		Memory:     summarizeMemory(input.Memory),
	}

	if input.HostMemoryBytes == 0 {
		report.Violations = append(report.Violations, "memory: host total must be positive")
	}
	if report.API.SampleCount == 0 {
		report.Violations = append(report.Violations, "api: no heartbeat samples")
	}
	if report.API.FailureCount > report.Thresholds.APIMaxFailures {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"api: failures %d exceed limit %d",
			report.API.FailureCount,
			report.Thresholds.APIMaxFailures,
		))
	}
	if report.API.P95Latency > report.Thresholds.APIP95Latency {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"api: p95 latency %s exceeds limit %s",
			report.API.P95Latency,
			report.Thresholds.APIP95Latency,
		))
	}
	if report.API.MaxLatency > report.Thresholds.APIMaxLatency {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"api: max latency %s exceeds limit %s",
			report.API.MaxLatency,
			report.Thresholds.APIMaxLatency,
		))
	}
	if report.Peer.SampleCount == 0 {
		report.Violations = append(report.Violations, "peer: no heartbeat samples")
	}
	if report.Peer.FailureCount > report.Thresholds.PeerMaxFailures {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"peer: failures %d exceed limit %d",
			report.Peer.FailureCount,
			report.Thresholds.PeerMaxFailures,
		))
	}
	if report.Peer.MaxGap > report.Thresholds.PeerMaxGap {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"peer: max gap %s exceeds limit %s",
			report.Peer.MaxGap,
			report.Thresholds.PeerMaxGap,
		))
	}
	if report.Memory.SampleCount == 0 {
		report.Violations = append(report.Violations, "memory: no MemAvailable samples")
	} else if report.Memory.MinAvailableBytes < report.Thresholds.MinMemAvailableBytes {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"memory: minimum MemAvailable %d bytes is below floor %d bytes",
			report.Memory.MinAvailableBytes,
			report.Thresholds.MinMemAvailableBytes,
		))
	}

	report.Passed = len(report.Violations) == 0
	return report
}

// memoryAvailableFloor returns the greater of 2 GiB and ten percent of host memory.
func memoryAvailableFloor(hostMemoryBytes uint64) uint64 {
	tenPercent := hostMemoryBytes / memoryFractionDivisor
	if hostMemoryBytes%memoryFractionDivisor != 0 {
		tenPercent++
	}
	return max(DefaultMinimumMemAvailableBytes, tenPercent)
}

// summarizeAPI computes failure, percentile, and maximum API measurements.
func summarizeAPI(samples []ProbeSample) APISummary {
	latencies := make([]time.Duration, 0, len(samples))
	summary := APISummary{SampleCount: len(samples)}
	for _, sample := range samples {
		latencies = append(latencies, sample.Latency)
		if !sample.Succeeded {
			summary.FailureCount++
		}
		if sample.Latency > summary.MaxLatency {
			summary.MaxLatency = sample.Latency
		}
	}
	summary.P95Latency = nearestRank(latencies, apiLatencyPercentile)
	return summary
}

// summarizePeer computes failures and the longest interval without a successful heartbeat.
func summarizePeer(samples []ProbeSample) PeerSummary {
	ordered := slices.Clone(samples)
	slices.SortFunc(ordered, func(left, right ProbeSample) int {
		return left.At.Compare(right.At)
	})
	summary := PeerSummary{SampleCount: len(ordered)}
	if len(ordered) == 0 {
		return summary
	}

	windowStart := ordered[0].At
	lastSuccess := windowStart
	hasSuccess := false
	for _, sample := range ordered {
		if !sample.Succeeded {
			summary.FailureCount++
			continue
		}
		if hasSuccess {
			summary.MaxGap = max(summary.MaxGap, sample.At.Sub(lastSuccess))
		} else {
			summary.MaxGap = max(summary.MaxGap, sample.At.Sub(windowStart))
			hasSuccess = true
		}
		lastSuccess = sample.At
	}

	windowEnd := ordered[len(ordered)-1].At
	if hasSuccess {
		summary.MaxGap = max(summary.MaxGap, windowEnd.Sub(lastSuccess))
	} else {
		summary.MaxGap = max(summary.MaxGap, windowEnd.Sub(windowStart))
	}
	return summary
}

// summarizeMemory computes the lowest observed host MemAvailable value.
func summarizeMemory(samples []MemorySample) MemorySummary {
	summary := MemorySummary{SampleCount: len(samples)}
	if len(samples) == 0 {
		return summary
	}

	summary.MinAvailableBytes = samples[0].AvailableBytes
	for _, sample := range samples[1:] {
		summary.MinAvailableBytes = min(summary.MinAvailableBytes, sample.AvailableBytes)
	}
	return summary
}

// nearestRank returns the nearest-rank percentile for a duration series.
func nearestRank(values []time.Duration, percentile int) time.Duration {
	if len(values) == 0 {
		return 0
	}

	ordered := slices.Clone(values)
	slices.Sort(ordered)
	rank := (len(ordered)*percentile + percentileBase - 1) / percentileBase
	return ordered[rank-1]
}
