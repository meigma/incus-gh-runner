package liveacceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	incusadapter "github.com/meigma/incus-gh-runner/internal/adapters/incus"
	"github.com/meigma/incus-gh-runner/internal/incusvalidate"
	"github.com/meigma/incus-gh-runner/internal/liveacceptance/metrics"
)

// acceptanceSourceRevision is the linker-injected clean source revision.
var acceptanceSourceRevision string //nolint:gochecknoglobals // Linker injection requires process-global storage.

// acceptanceSourceModified is the linker-injected source-tree state.
var acceptanceSourceModified string //nolint:gochecknoglobals // Linker injection requires process-global storage.

var sourceRevisionPattern = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)

// Summary is the retained high-level result of one live runtime acceptance probe.
type Summary struct {
	// Version identifies the runtime evidence schema.
	Version int `json:"version"`
	// RunID binds this result to the parent hostile-runner harness.
	RunID string `json:"run_id"`
	// StartedAt is the UTC start time of the probe.
	StartedAt time.Time `json:"started_at"`
	// FinishedAt is the UTC completion time of the probe.
	FinishedAt time.Time `json:"finished_at"`
	// Outcome reports whether every implemented runtime gate passed.
	Outcome string `json:"outcome"`
	// BaselineSHA256 binds the evidence to the exact rendered desired state.
	BaselineSHA256 string `json:"baseline_sha256"`
	// ImageFingerprint is the immutable reference image exercised by the probe.
	ImageFingerprint string `json:"image_fingerprint"`
	// AcceptanceSHA256 identifies the exact acceptance executable used on the host and guest.
	AcceptanceSHA256 string `json:"acceptance_sha256"`
	// SourceRevision is the explicitly injected VCS revision used to build the helper.
	SourceRevision string `json:"source_revision"`
	// SourceModified reports the explicitly injected source-tree state used to build the helper.
	SourceModified bool `json:"source_modified"`
	// StressDuration is the accepted steady-state pressure interval.
	StressDuration string `json:"stress_duration"`
	// PollInterval is the requested runtime-health sampling interval.
	PollInterval string `json:"poll_interval"`
	// Guests contains KVM, firmware, agent, and resource observations for both VMs.
	Guests []guestObservation `json:"guests,omitempty"`
	// Admission contains the full-capacity rejection result.
	Admission *admissionObservation `json:"admission,omitempty"`
	// IPv6 contains the deployed-defense no-bypass result.
	IPv6 *ipv6Observation `json:"ipv6,omitempty"`
	// Health contains API, peer, and host-memory measurements during pressure.
	Health *metrics.Report `json:"health,omitempty"`
	// Limitations bounds the security claims this probe can support.
	Limitations []string `json:"limitations"`
	// Error contains a bounded failure description when Outcome is failed.
	Error string `json:"error,omitempty"`
}

// Run executes the disposable-host runtime probe and always attempts to finalize retained evidence.
func Run(ctx context.Context, rawOptions Options) (Summary, error) {
	options := rawOptions.withDefaults()
	baseline, loadErr := LoadRuntimeBaseline(options.BaselinePath)
	if loadErr != nil {
		return Summary{}, loadErr
	}
	if validateErr := options.validate(baseline); validateErr != nil {
		return Summary{}, validateErr
	}
	if runtime.GOOS != "linux" {
		return Summary{}, errors.New("runtime acceptance probe requires Linux")
	}
	if os.Geteuid() != 0 {
		return Summary{}, errors.New("runtime acceptance probe must run as root on the disposable host")
	}
	if kvmErr := requireKVMDevice("/dev/kvm"); kvmErr != nil {
		return Summary{}, kvmErr
	}
	acceptanceDigest, digestErr := sha256File(options.SelfPath)
	if digestErr != nil {
		return Summary{}, digestErr
	}
	sourceRevision, sourceModified, provenanceErr := buildProvenance()
	if provenanceErr != nil {
		return Summary{}, provenanceErr
	}

	writer, evidenceErr := newEvidenceWriter(options.EvidenceDirectory)
	if evidenceErr != nil {
		return Summary{}, evidenceErr
	}
	summary := Summary{
		Version:          1,
		RunID:            options.RunID,
		StartedAt:        time.Now().UTC(),
		Outcome:          failedValue,
		BaselineSHA256:   bytesSHA256(baseline.sourceJSON),
		ImageFingerprint: options.ImageFingerprint,
		AcceptanceSHA256: acceptanceDigest,
		SourceRevision:   sourceRevision,
		SourceModified:   sourceModified,
		StressDuration:   options.StressDuration.String(),
		PollInterval:     options.PollInterval.String(),
		Limitations: []string{
			"IPv6 denial is a defense-in-depth result and does not attribute the drop to one overlapping control.",
			"Secure Boot was reported active; rejection of a valid but untrusted EFI payload was not independently exercised.",
			"Configured NIC and ZFS disk-I/O throughput ceilings were not independently benchmarked.",
			"Pressure proves bounded host and control-plane survival, not aggregate project CPU or memory runtime throttling.",
			"The local Incus Unix-socket identity remains root-equivalent; this gate requires a dedicated host and does not prove least-privilege Incus authorization.",
		},
	}
	probeErr := writer.write("baseline.json", baseline.sourceJSON)
	if probeErr == nil {
		probeContext, cancelProbe := context.WithTimeout(ctx, options.StressDuration+probeOverheadTimeout)
		probeErr = runProbe(probeContext, commandRunner{}, writer, options, baseline, &summary)
		cancelProbe()
	}
	if probeErr == nil {
		summary.Outcome = "passed"
	} else {
		summary.Error = boundedReason(probeErr.Error())
	}
	summary.FinishedAt = time.Now().UTC()
	resultData, finalizeErr := evidenceJSON(summary)
	if finalizeErr == nil {
		finalizeErr = writer.writeChecksums("result.json", resultData)
	}
	if finalizeErr == nil {
		finalizeErr = writer.write("result.json", resultData)
	}
	return summary, errors.Join(probeErr, finalizeErr)
}

// runProbe executes the ordered runtime gates while the parent harness owns both runner VMs.
func runProbe(
	ctx context.Context,
	commands commandRunner,
	writer *evidenceWriter,
	options Options,
	baseline RuntimeBaseline,
	summary *Summary,
) error {
	if err := verifyMutationGate(ctx, commands, options); err != nil {
		return err
	}
	if err := validateLiveBaseline(ctx, baseline); err != nil {
		return fmt.Errorf("pre-probe baseline validation: %w", err)
	}
	if err := captureHostState(ctx, commands, writer, "before"); err != nil {
		return err
	}

	canaries := make([][]byte, 0, acceptanceRunnerCount)
	for _, instance := range []string{options.VMA, options.VMB} {
		observation, canary, err := observeGuest(ctx, commands, writer, options, baseline, instance)
		if err != nil {
			return err
		}
		summary.Guests = append(summary.Guests, observation)
		canaries = append(canaries, canary)
	}

	admission, admissionErr := proveAdmissionCeiling(ctx, commands, writer, options)
	if admissionErr != nil {
		return admissionErr
	}
	summary.Admission = &admission
	ipv6, ipv6Err := proveIPv6NoBypass(ctx, commands, writer, options)
	if ipv6Err != nil {
		return ipv6Err
	}
	summary.IPv6 = &ipv6
	health, pressureErr := applyPressure(ctx, commands, writer, options, baseline)
	if pressureErr != nil {
		return pressureErr
	}
	summary.Health = &health

	if err := captureHostState(ctx, commands, writer, "after"); err != nil {
		return err
	}
	if err := validateLiveBaseline(ctx, baseline); err != nil {
		return fmt.Errorf("post-probe baseline validation: %w", err)
	}
	for _, canary := range canaries {
		if err := writer.scanFor(canary); err != nil {
			return err
		}
	}
	currentDigest, digestErr := sha256File(options.SelfPath)
	if digestErr != nil {
		return digestErr
	}
	if currentDigest != summary.AcceptanceSHA256 {
		return errors.New("acceptance executable changed during the runtime probe")
	}
	return nil
}

// verifyMutationGate independently re-checks the parent harness project and instance markers.
func verifyMutationGate(ctx context.Context, commands commandRunner, options Options) error {
	if os.Getenv("INCUS_GH_RUNNER_LIVE_MUTATION") != mutationOptIn {
		return errors.New("runtime probe requires the exact disposable-project mutation opt-in")
	}
	requestContext, cancel := context.WithTimeout(ctx, shortCommandTimeout)
	projectMarker, err := commands.run(requestContext, "incus", "project", "get", options.Project, disposableKey)
	cancel()
	if err != nil {
		return err
	}
	if err := requireSuccess("read disposable project marker", projectMarker); err != nil {
		return err
	}
	if strings.TrimSpace(string(projectMarker.stdout)) != trueValue {
		return errors.New("runtime probe requires a project marked disposable")
	}
	for _, instance := range []string{options.VMA, options.VMB} {
		actualID, err := instanceConfigValue(ctx, commands, options.Project, instance, acceptanceIDKey)
		if err != nil {
			return err
		}
		disposable, err := instanceConfigValue(ctx, commands, options.Project, instance, disposableVMKey)
		if err != nil {
			return err
		}
		if actualID != options.RunID || disposable != trueValue {
			return fmt.Errorf("runner VM %q does not carry the expected acceptance markers", instance)
		}
	}
	return nil
}

// validateLiveBaseline compares the policy-derived manifest with fresh local Incus state.
func validateLiveBaseline(ctx context.Context, baseline RuntimeBaseline) error {
	requestContext, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	reader, err := incusadapter.ConnectValidationReader(requestContext, "/var/lib/incus/unix.socket")
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = incusvalidate.Validate(requestContext, acceptanceManifest(baseline.Manifest), reader)
	return err
}

// acceptanceManifest adds only the exact disposable-project marker required by the live mutation gate.
func acceptanceManifest(manifest incusvalidate.Baseline) incusvalidate.Baseline {
	manifest.Project.Config = maps.Clone(manifest.Project.Config)
	manifest.Project.Config[disposableKey] = trueValue
	return manifest
}

// requireKVMDevice requires the host acceleration device before any probe mutation begins.
func requireKVMDevice(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect host KVM device: %w", err)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return errors.New("host KVM path is not a character device")
	}
	return nil
}

// captureHostState retains daemon, ZFS, and kernel state around the hostile pressure window.
func captureHostState(ctx context.Context, commands commandRunner, writer *evidenceWriter, suffix string) error {
	daemon, err := readDaemonState(ctx, commands)
	if err != nil {
		return err
	}
	if err := writer.writeJSON("incus-daemon-"+suffix+".json", daemon); err != nil {
		return err
	}
	for name, arguments := range map[string][]string{
		"incus-info-" + suffix + ".txt":   {"info"},
		"zpool-status-" + suffix + ".txt": {"status", "-x"},
		"zpool-list-" + suffix + ".txt":   {listArgument, "-Hp", "-o", "name,health,size,alloc,free"},
	} {
		commandName := "incus"
		if strings.HasPrefix(name, "zpool-") {
			commandName = "zpool"
		}
		requestContext, cancel := context.WithTimeout(ctx, commandTimeout)
		result, runErr := commands.run(requestContext, commandName, arguments...)
		cancel()
		if runErr != nil {
			return runErr
		}
		if err := requireSuccess("capture "+name, result); err != nil {
			return err
		}
		if err := writer.write(name, result.stdout); err != nil {
			return err
		}
		if commandName == "zpool" && arguments[0] == "status" && !zpoolHealthy(result.stdout) {
			return errors.New("ZFS pool health check did not report a healthy host")
		}
	}
	return nil
}

// zpoolHealthy recognizes the healthy summary returned by supported OpenZFS releases.
func zpoolHealthy(output []byte) bool {
	lower := strings.ToLower(string(output))
	return strings.Contains(lower, "all pools are healthy") || strings.Contains(lower, "is healthy")
}

// bytesSHA256 returns the lowercase digest of the exact validated evidence bytes.
func bytesSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// sha256File returns the digest of one exact regular file.
func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read acceptance executable for provenance: %w", err)
	}
	return bytesSHA256(data), nil
}

// buildProvenance validates the source identity explicitly injected by the documented build command.
func buildProvenance() (string, bool, error) {
	return parseBuildProvenance(acceptanceSourceRevision, acceptanceSourceModified)
}

// parseBuildProvenance rejects missing, malformed, or ambiguous source identity values.
func parseBuildProvenance(revision string, modified string) (string, bool, error) {
	if !sourceRevisionPattern.MatchString(revision) {
		return "", false, errors.New("acceptance helper lacks an explicitly injected source revision")
	}
	switch modified {
	case falseValue:
		return revision, false, nil
	case trueValue:
		return "", false, errors.New("acceptance helper was built from modified source")
	default:
		return "", false, errors.New("acceptance helper lacks an explicitly injected source-modified state")
	}
}
