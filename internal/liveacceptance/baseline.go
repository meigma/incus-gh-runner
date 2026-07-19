// Package liveacceptance implements the disposable-host runtime security probe.
package liveacceptance

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	incuspolicy "github.com/meigma/incus-gh-runner/deploy/incus"
	"github.com/meigma/incus-gh-runner/internal/incusvalidate"
)

// RuntimeBaseline contains the typed limits and resource names consumed by the live probe.
type RuntimeBaseline struct {
	sourceJSON []byte
	// Manifest is the policy-validated desired Incus state.
	Manifest incusvalidate.Baseline
	// MaximumRunners is the project and controller VM ceiling exercised by the probe.
	MaximumRunners int
	// RunnerCPU is the logical CPU count exposed to every runner VM.
	RunnerCPU int
	// RunnerMemoryBytes is the memory ceiling exposed to every runner VM.
	RunnerMemoryBytes uint64
	// RunnerRootDiskBytes is the configured root volume size for every runner VM.
	RunnerRootDiskBytes uint64
	// RunnerNetworkBitsPerSecond is the requested per-VM network ceiling.
	RunnerNetworkBitsPerSecond uint64
	// RunnerDiskBytesPerSecond is the requested per-VM disk throughput ceiling.
	RunnerDiskBytesPerSecond uint64
}

// LoadRuntimeBaseline validates and derives the runtime acceptance inputs from one rendered baseline.
func LoadRuntimeBaseline(path string) (RuntimeBaseline, error) {
	info, err := os.Stat(path)
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("stat baseline: %w", err)
	}
	if !info.Mode().IsRegular() {
		return RuntimeBaseline{}, errors.New("baseline must be a regular file")
	}
	if info.Size() > incuspolicy.MaximumBaselineBytes {
		return RuntimeBaseline{}, fmt.Errorf("baseline exceeds the %d-byte limit", incuspolicy.MaximumBaselineBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("read baseline: %w", err)
	}
	manifest, err := incusvalidate.ParseBaseline(path, data, incuspolicy.ValidateBaseline)
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("validate baseline: %w", err)
	}

	maximumRunners, err := parsePositiveInteger(manifest.Project.Config["limits.instances"])
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("parse project VM ceiling: %w", err)
	}
	runnerCPU, err := parsePositiveInteger(manifest.Profile.Config["limits.cpu"])
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("parse runner CPU ceiling: %w", err)
	}
	runnerMemoryBytes, err := parseBinaryQuantity(manifest.Profile.Config["limits.memory"], "GiB", gibibyte)
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("parse runner memory ceiling: %w", err)
	}
	runnerRootDiskBytes, err := parseBinaryQuantity(
		manifest.Profile.Devices["root"]["size"],
		"GiB",
		gibibyte,
	)
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("parse runner root disk ceiling: %w", err)
	}
	runnerNetworkBitsPerSecond, err := parseBinaryQuantity(
		manifest.Profile.Devices["eth0"]["limits.max"],
		"Mbit",
		megabitPerSecond,
	)
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("parse runner network ceiling: %w", err)
	}
	runnerDiskBytesPerSecond, err := parseBinaryQuantity(
		manifest.Profile.Devices["root"]["limits.max"],
		"MiB",
		mebibyte,
	)
	if err != nil {
		return RuntimeBaseline{}, fmt.Errorf("parse runner disk throughput ceiling: %w", err)
	}

	return RuntimeBaseline{
		sourceJSON:                 append([]byte(nil), data...),
		Manifest:                   manifest,
		MaximumRunners:             maximumRunners,
		RunnerCPU:                  runnerCPU,
		RunnerMemoryBytes:          runnerMemoryBytes,
		RunnerRootDiskBytes:        runnerRootDiskBytes,
		RunnerNetworkBitsPerSecond: runnerNetworkBitsPerSecond,
		RunnerDiskBytesPerSecond:   runnerDiskBytesPerSecond,
	}, nil
}

// parsePositiveInteger parses one strictly positive base-ten integer.
func parsePositiveInteger(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("expected a positive integer, got %q", value)
	}

	return parsed, nil
}

// parseBinaryQuantity parses the exact positive integer and suffix emitted by the CUE policy.
func parseBinaryQuantity(value string, suffix string, multiplier uint64) (uint64, error) {
	raw, found := strings.CutSuffix(value, suffix)
	if !found || raw == "" {
		return 0, fmt.Errorf("expected a positive %s quantity, got %q", suffix, value)
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("expected a positive %s quantity, got %q", suffix, value)
	}
	if parsed > ^uint64(0)/multiplier {
		return 0, fmt.Errorf("%s quantity overflows bytes: %q", suffix, value)
	}

	return parsed * multiplier, nil
}
