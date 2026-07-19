package liveacceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/meigma/incus-gh-runner/internal/liveacceptance/metrics"
	"github.com/meigma/incus-gh-runner/internal/liveacceptance/pressure"
)

// guestPressureArtifacts names the run-scoped unit and files owned by one pressure experiment.
type guestPressureArtifacts struct {
	executable string
	disk       string
	result     string
	stderr     string
	unit       string
}

// guestPressureWorkload records the exact bounded workload requested from the guest helper.
type guestPressureWorkload struct {
	cpuWorkers  int
	memoryBytes int64
	diskBytes   int64
}

// watchdogSample records one pressure-window health observation.
type watchdogSample struct {
	ObservedAt         time.Time     `json:"observed_at"`
	APILatency         time.Duration `json:"api_latency"`
	APISucceeded       bool          `json:"api_succeeded"`
	PeerLatency        time.Duration `json:"peer_latency"`
	PeerSucceeded      bool          `json:"peer_succeeded"`
	HostAvailableBytes uint64        `json:"host_available_bytes"`
}

// egressWatchdogSample records one independent approved-egress observation.
type egressWatchdogSample struct {
	ObservedAt time.Time     `json:"observed_at"`
	Latency    time.Duration `json:"latency"`
	Succeeded  bool          `json:"succeeded"`
}

// watchdogCommandResult carries one concurrent command probe result.
type watchdogCommandResult struct {
	result commandResult
	err    error
}

// applyPressure saturates one runner while measuring Incus, peer, and host availability.
func applyPressure( //nolint:cyclop,funlen,gocognit,gocyclo,nonamedreturns // Concurrent teardown is one lifecycle boundary.
	ctx context.Context,
	commands commandRunner,
	writer *evidenceWriter,
	options Options,
	baseline RuntimeBaseline,
) (report metrics.Report, returnErr error) {
	beforeDaemon, daemonErr := readDaemonState(ctx, commands)
	if daemonErr != nil {
		return metrics.Report{}, daemonErr
	}
	initialMemory, memoryErr := readMemoryState("/proc/meminfo", time.Now().UTC())
	if memoryErr != nil {
		return metrics.Report{}, memoryErr
	}
	startedAt := time.Now().UTC()
	stream, streamErr := writer.openJSONL("watchdog.jsonl")
	if streamErr != nil {
		return metrics.Report{}, streamErr
	}
	streamClosed := false
	defer func() {
		if !streamClosed {
			returnErr = errors.Join(returnErr, stream.close())
		}
	}()
	egressStream, egressStreamErr := writer.openJSONL("egress-watchdog.jsonl")
	if egressStreamErr != nil {
		return metrics.Report{}, egressStreamErr
	}
	egressStreamClosed := false
	defer func() {
		if !egressStreamClosed {
			returnErr = errors.Join(returnErr, egressStream.close())
		}
	}()
	artifacts := newGuestPressureArtifacts(options.RunID)
	workload := newGuestPressureWorkload(baseline)
	defer func() {
		cleanupErr := cleanupGuestPressure(ctx, commands, options, artifacts)
		returnErr = errors.Join(returnErr, cleanupErr)
	}()
	if pushErr := pushPressureExecutable(ctx, commands, options, artifacts); pushErr != nil {
		return metrics.Report{}, pushErr
	}
	pressureStartedAt := time.Now()
	if startErr := startGuestPressure(ctx, commands, options, workload, artifacts); startErr != nil {
		return metrics.Report{}, startErr
	}
	egressContext, cancelEgress := context.WithCancel(ctx)
	egressResults := make(chan egressWatchdogSample, 1)
	egressJoined := false
	go runEgressWatchdog(egressContext, ctx, commands, options, egressResults)
	defer func() {
		cancelEgress()
		if !egressJoined {
			returnErr = errors.Join(returnErr, waitForEgressExit(egressResults))
		}
	}()

	metricInput := metrics.Input{HostMemoryBytes: initialMemory.TotalBytes}
	egressFailures := 0
	egressSamples := 0
	ticker := time.NewTicker(options.PollInterval)
	defer ticker.Stop()
	pressureMinimumDoneAt := pressureStartedAt.Add(options.StressDuration)
	pressureMaximumDoneAt := pressureMinimumDoneAt.Add(commandTimeout)
	initialSample := collectWatchdogSample(ctx, commands, options)
	if sampleErr := appendWatchdogSample(stream, &metricInput, initialSample); sampleErr != nil {
		return metrics.Report{}, sampleErr
	}

	var pressureOutput []byte
	for len(pressureOutput) == 0 {
		select {
		case <-ctx.Done():
			return metrics.Report{}, ctx.Err()
		case egressSample, open := <-egressResults:
			if !open {
				egressResults = nil
				continue
			}
			if writeErr := egressStream.write(egressSample); writeErr != nil {
				return metrics.Report{}, writeErr
			}
			egressSamples++
			if !egressSample.Succeeded {
				egressFailures++
			}
		case <-ticker.C:
			sample := collectWatchdogSample(ctx, commands, options)
			if writeErr := appendWatchdogSample(stream, &metricInput, sample); writeErr != nil {
				return metrics.Report{}, writeErr
			}
			output, ready, readErr := readGuestPressureResult(ctx, commands, options, artifacts)
			if readErr != nil {
				return metrics.Report{}, readErr
			}
			if ready {
				if time.Now().Before(pressureMinimumDoneAt) {
					return metrics.Report{}, errors.New("guest pressure completed before the minimum evidence window")
				}
				pressureOutput = output
				continue
			}
			if time.Now().After(pressureMaximumDoneAt) {
				return metrics.Report{}, errors.New("guest pressure did not finish within its accepted window")
			}
		}
	}
	finalSample := collectWatchdogSample(ctx, commands, options)
	if writeErr := appendWatchdogSample(stream, &metricInput, finalSample); writeErr != nil {
		return metrics.Report{}, writeErr
	}
	cancelEgress()
	if egressResults != nil {
		joinTimer := time.NewTimer(guestEgressTimeout)
	joinEgress:
		for {
			select {
			case egressSample, open := <-egressResults:
				if !open {
					break joinEgress
				}
				if writeErr := egressStream.write(egressSample); writeErr != nil {
					joinTimer.Stop()
					return metrics.Report{}, writeErr
				}
				egressSamples++
				if !egressSample.Succeeded {
					egressFailures++
				}
			case <-joinTimer.C:
				return metrics.Report{}, errors.New("timed out joining approved-egress watchdog")
			}
		}
		joinTimer.Stop()
	}
	egressJoined = true
	if closeErr := egressStream.close(); closeErr != nil {
		return metrics.Report{}, closeErr
	}
	egressStreamClosed = true
	if closeErr := stream.close(); closeErr != nil {
		return metrics.Report{}, closeErr
	}
	streamClosed = true
	if checkErr := validatePressureResult(pressureOutput, options.StressDuration, workload); checkErr != nil {
		return metrics.Report{}, checkErr
	}
	if writeErr := writer.write("guest-pressure-result.json", pressureOutput); writeErr != nil {
		return metrics.Report{}, writeErr
	}

	report = metrics.Evaluate(metricInput)
	if writeErr := writer.writeJSON("health-report.json", report); writeErr != nil {
		return metrics.Report{}, writeErr
	}
	if !report.Passed {
		return report, fmt.Errorf("runtime health thresholds failed: %s", strings.Join(report.Violations, "; "))
	}
	if egressSamples < acceptanceRunnerCount {
		return report, fmt.Errorf("approved peer egress produced only %d pressure-window samples", egressSamples)
	}
	if egressFailures != 0 {
		return report, fmt.Errorf("approved peer egress failed %d times during pressure", egressFailures)
	}
	afterDaemon, daemonErr := readDaemonState(ctx, commands)
	if daemonErr != nil {
		return report, daemonErr
	}
	if beforeDaemon != afterDaemon {
		return report, errors.New("incus daemon restarted during guest pressure")
	}

	kernelContext, cancelKernel := context.WithTimeout(ctx, commandTimeout)
	kernelLog, runErr := commands.run(
		kernelContext,
		"journalctl",
		"--dmesg",
		"--since",
		startedAt.Format(time.RFC3339),
		"--no-pager",
		"--output=short-iso",
	)
	cancelKernel()
	if runErr != nil {
		return report, runErr
	}
	if checkErr := requireSuccess("read pressure-window kernel log", kernelLog); checkErr != nil {
		return report, checkErr
	}
	if writeErr := writer.write("kernel-pressure-window.log", kernelLog.stdout); writeErr != nil {
		return report, writeErr
	}
	if kernelLogHasResourceFailure(kernelLog.stdout) {
		return report, errors.New("host kernel reported a resource or I/O failure during pressure")
	}
	return report, nil
}

// newGuestPressureArtifacts derives collision-resistant guest names without exposing the caller's run ID to a shell.
func newGuestPressureArtifacts(runID string) guestPressureArtifacts {
	digest := sha256.Sum256([]byte(runID))
	token := hex.EncodeToString(digest[:8])
	prefix := "incus-gh-runner-" + token + "-runtime-acceptance"
	return guestPressureArtifacts{
		executable: "/run/" + prefix,
		disk:       "/var/tmp/" + prefix + ".bin",
		result:     "/run/" + prefix + "-result.json",
		stderr:     "/run/" + prefix + "-stderr.log",
		unit:       prefix + ".service",
	}
}

// newGuestPressureWorkload derives the exact bounded pressure intent from the validated runner baseline.
func newGuestPressureWorkload(baseline RuntimeBaseline) guestPressureWorkload {
	return guestPressureWorkload{
		cpuWorkers: min(baseline.RunnerCPU*pressureCPUFactor, pressure.MaxCPUWorkers),
		memoryBytes: boundedInt64(
			baseline.RunnerMemoryBytes/acceptanceRunnerCount,
			pressure.MaxMemoryBytes,
		),
		diskBytes: boundedInt64(
			baseline.RunnerRootDiskBytes/rootPressureDiskDivisor,
			pressure.MaxDiskBytes,
		),
	}
}

// pushPressureExecutable copies this non-shipped probe through the VM agent with an exact mode.
func pushPressureExecutable(
	ctx context.Context,
	commands incusCommandRunner,
	options Options,
	artifacts guestPressureArtifacts,
) error {
	requestContext, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	result, err := commands.incus(
		requestContext,
		options.Project,
		"file",
		"push",
		"--mode=0755",
		options.SelfPath,
		options.VMA+artifacts.executable,
	)
	if err != nil {
		return err
	}
	return requireSuccess("push guest pressure executable", result)
}

// startGuestPressure starts the bounded workload as one exact transient guest service and proves its PID is live.
func startGuestPressure(
	ctx context.Context,
	commands incusCommandRunner,
	options Options,
	workload guestPressureWorkload,
	artifacts guestPressureArtifacts,
) error {
	pressureContext, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	pressureCommand, err := commands.incus(
		pressureContext,
		options.Project,
		incusExecArgument,
		options.VMA,
		"--",
		"systemd-run",
		"--quiet",
		"--collect",
		"--service-type=exec",
		"--property=TimeoutStopSec=10s",
		"--property=StandardOutput=truncate:"+artifacts.result,
		"--property=StandardError=truncate:"+artifacts.stderr,
		"--unit",
		artifacts.unit,
		artifacts.executable,
		"guest-pressure",
		"--duration",
		options.StressDuration.String(),
		"--cpu-workers",
		strconv.Itoa(workload.cpuWorkers),
		"--memory-bytes",
		strconv.FormatInt(workload.memoryBytes, 10),
		"--disk-path",
		artifacts.disk,
		"--disk-bytes",
		strconv.FormatInt(workload.diskBytes, 10),
	)
	if err != nil {
		return err
	}
	if checkErr := requireSuccess("start bounded guest pressure", pressureCommand); checkErr != nil {
		return checkErr
	}
	_, err = guestOutput(
		ctx,
		commands,
		options.Project,
		options.VMA,
		"bash",
		"-c",
		`set -Eeuo pipefail; for _ in $(seq 1 20); do state="$(systemctl show "$1" --property=ActiveState --value 2>/dev/null || true)"; pid="$(systemctl show "$1" --property=MainPID --value 2>/dev/null || true)"; if [[ "$state" == active && "$pid" =~ ^[1-9][0-9]*$ ]] && kill -0 "$pid" 2>/dev/null; then exit 0; fi; sleep 0.25; done; exit 1`,
		"bash",
		artifacts.unit,
	)
	if err != nil {
		return fmt.Errorf("prove guest pressure unit started: %w", err)
	}
	return nil
}

// readGuestPressureResult returns the completed JSON document or reports that the exact unit is still active.
func readGuestPressureResult(
	ctx context.Context,
	commands incusCommandRunner,
	options Options,
	artifacts guestPressureArtifacts,
) ([]byte, bool, error) {
	raw, err := guestOutput(
		ctx,
		commands,
		options.Project,
		options.VMA,
		"bash",
		"-c",
		`set -Eeuo pipefail
output="$(systemctl show "$2" --property=LoadState --property=ActiveState --property=MainPID)"
load_state="$(sed -n 's/^LoadState=//p' <<<"$output")"
active_state="$(sed -n 's/^ActiveState=//p' <<<"$output")"
main_pid="$(sed -n 's/^MainPID=//p' <<<"$output")"
[[ -n "$load_state" && -n "$active_state" && "$main_pid" =~ ^[0-9]+$ ]]
if [[ "$load_state" == loaded && ( "$active_state" == active || "$active_state" == activating || "$active_state" == deactivating ) ]]; then
  printf 'pending\n'
  exit 0
fi
if [[ "$load_state" == not-found ]] || [[ "$load_state" == loaded && ( "$active_state" == inactive || "$active_state" == failed ) && "$main_pid" == 0 ]]; then
  if [[ -s "$1" ]]; then
    printf 'ready\n'
    cat "$1"
    exit 0
  fi
  printf 'failed\n'
  [[ ! -f "$3" ]] || tail -c 300 "$3"
  exit 0
fi
exit 1`,
		"bash",
		artifacts.result,
		artifacts.unit,
		artifacts.stderr,
	)
	if err != nil {
		return nil, false, err
	}
	return classifyGuestPressureStatus(raw)
}

// classifyGuestPressureStatus distinguishes a complete result from an active or failed transient unit.
func classifyGuestPressureStatus(raw string) ([]byte, bool, error) {
	status, detail, found := strings.Cut(raw, "\n")
	if !found {
		return nil, false, errors.New("guest pressure status omitted its result delimiter")
	}
	switch status {
	case readyValue:
		return []byte(detail), true, nil
	case "pending":
		return nil, false, nil
	case failedValue:
		return nil, false, fmt.Errorf("guest pressure unit failed before writing a result: %s", boundedReason(detail))
	default:
		return nil, false, errors.New("guest pressure returned an unknown result status")
	}
}

// cleanupGuestPressure stops the exact transient unit and removes only its run-scoped disposable files.
func cleanupGuestPressure(
	ctx context.Context,
	commands incusCommandRunner,
	options Options,
	artifacts guestPressureArtifacts,
) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), commandTimeout)
	defer cancel()
	script := `set -Eeuo pipefail
unit="$1"
executable="$2"
pressure_file="$3"
result_file="$4"
stderr_file="$5"
read_unit_state() {
  local output
  output="$(systemctl show "$unit" --property=LoadState --property=ActiveState --property=MainPID)"
  load_state="$(sed -n 's/^LoadState=//p' <<<"$output")"
  active_state="$(sed -n 's/^ActiveState=//p' <<<"$output")"
  main_pid="$(sed -n 's/^MainPID=//p' <<<"$output")"
  [[ -n "$load_state" && -n "$active_state" && "$main_pid" =~ ^[0-9]+$ ]]
}
unit_stopped() {
  [[ "$load_state" == not-found ]] || {
    [[ "$load_state" == loaded && ( "$active_state" == inactive || "$active_state" == failed ) && "$main_pid" == 0 ]]
  }
}
remove_artifacts() {
  rm -f -- "$executable" "$pressure_file" "$result_file" "$stderr_file"
  [[ ! -e "$executable" && ! -e "$pressure_file" && ! -e "$result_file" && ! -e "$stderr_file" ]]
}
rm -f -- "$executable"
[[ ! -e "$executable" ]]
read_unit_state
if unit_stopped; then
  remove_artifacts
  exit 0
fi
[[ "$load_state" == loaded ]]
if ! systemctl stop --no-block "$unit" >/dev/null; then
  read_unit_state
  if unit_stopped; then
    remove_artifacts
    exit 0
  fi
  exit 1
fi
for _ in $(seq 1 20); do
  read_unit_state
  if unit_stopped; then
    remove_artifacts
    exit 0
  fi
  sleep 0.25
done
if ! systemctl kill --kill-whom=all --signal=KILL "$unit" >/dev/null; then
  read_unit_state
  unit_stopped || exit 1
fi
for _ in $(seq 1 20); do
  read_unit_state
  if unit_stopped; then
    remove_artifacts
    exit 0
  fi
  sleep 0.25
done
exit 1`
	_, err := guestOutput(
		cleanupContext,
		commands,
		options.Project,
		options.VMA,
		"bash",
		"-c",
		script,
		"bash",
		artifacts.unit,
		artifacts.executable,
		artifacts.disk,
		artifacts.result,
		artifacts.stderr,
	)
	if err != nil {
		return fmt.Errorf("clean up guest pressure workload: %w", err)
	}
	return nil
}

// runEgressWatchdog probes approved egress on an independent cadence until cancellation.
func runEgressWatchdog(
	scheduleContext context.Context,
	probeContext context.Context,
	commands incusCommandRunner,
	options Options,
	result chan<- egressWatchdogSample,
) {
	defer close(result)
	interval := options.PollInterval * egressSamplePeriod
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-scheduleContext.Done():
			return
		default:
		}
		startedAt := time.Now()
		err := guestEgress(
			probeContext,
			commands,
			options.Project,
			options.VMB,
			options.AllowedURL,
			options.EgressProxy,
		)
		if probeContext.Err() != nil {
			return
		}
		sample := egressWatchdogSample{
			ObservedAt: time.Now().UTC(),
			Latency:    time.Since(startedAt),
			Succeeded:  err == nil,
		}
		result <- sample
		select {
		case <-ticker.C:
		case <-scheduleContext.Done():
			return
		}
	}
}

// waitForEgressExit drains and bounds joining the independent approved-egress watchdog.
func waitForEgressExit(result <-chan egressWatchdogSample) error {
	timer := time.NewTimer(guestEgressTimeout)
	defer timer.Stop()
	for {
		select {
		case _, open := <-result:
			if !open {
				return nil
			}
		case <-timer.C:
			return errors.New("timed out joining approved-egress watchdog")
		}
	}
}

// boundedInt64 caps an unsigned baseline quantity at an already-positive signed pressure limit.
func boundedInt64(value uint64, ceiling int64) int64 {
	if ceiling <= 0 {
		return 0
	}
	unsignedCeiling := uint64(ceiling)
	if value >= unsignedCeiling {
		return ceiling
	}
	return int64(value) //nolint:gosec // The explicit ceiling guard proves the conversion fits.
}

// appendWatchdogSample retains one sample and adds it to the threshold evaluator input.
func appendWatchdogSample(
	stream *jsonlWriter,
	input *metrics.Input,
	sample watchdogSample,
) error {
	if err := stream.write(sample); err != nil {
		return err
	}
	input.API = append(input.API, metrics.ProbeSample{
		At:        sample.ObservedAt,
		Latency:   sample.APILatency,
		Succeeded: sample.APISucceeded,
	})
	input.Peer = append(input.Peer, metrics.ProbeSample{
		At:        sample.ObservedAt,
		Latency:   sample.PeerLatency,
		Succeeded: sample.PeerSucceeded,
	})
	input.Memory = append(input.Memory, metrics.MemorySample{
		At:             sample.ObservedAt,
		AvailableBytes: sample.HostAvailableBytes,
	})
	return nil
}

// collectWatchdogSample measures API, peer, optional egress, and host memory concurrently.
func collectWatchdogSample(
	ctx context.Context,
	commands commandRunner,
	options Options,
) watchdogSample {
	apiResult := make(chan watchdogCommandResult, 1)
	peerResult := make(chan watchdogCommandResult, 1)
	go collectAPIHeartbeat(ctx, commands, apiResult)
	go collectPeerHeartbeat(ctx, commands, options, peerResult)

	api := <-apiResult
	peer := <-peerResult
	observedAt := time.Now().UTC()
	memory, memoryErr := readMemoryState("/proc/meminfo", observedAt)
	return watchdogSample{
		ObservedAt:    observedAt,
		APILatency:    api.result.duration,
		APISucceeded:  api.err == nil && api.result.succeeded(),
		PeerLatency:   peer.result.duration,
		PeerSucceeded: peer.err == nil && peer.result.succeeded(),
		HostAvailableBytes: func() uint64 {
			if memoryErr != nil {
				return 0
			}
			return memory.AvailableBytes
		}(),
	}
}

// collectAPIHeartbeat performs one bounded local Incus API request.
func collectAPIHeartbeat(ctx context.Context, commands commandRunner, result chan<- watchdogCommandResult) {
	requestContext, cancel := context.WithTimeout(ctx, metrics.DefaultAPIMaxLimit)
	defer cancel()
	heartbeat, err := commands.run(requestContext, "incus", "query", "/1.0")
	result <- watchdogCommandResult{result: heartbeat, err: err}
}

// collectPeerHeartbeat performs one bounded agent request against the unaffected runner.
func collectPeerHeartbeat(
	ctx context.Context,
	commands commandRunner,
	options Options,
	result chan<- watchdogCommandResult,
) {
	requestContext, cancel := context.WithTimeout(ctx, metrics.DefaultAPIMaxLimit)
	defer cancel()
	heartbeat, err := commands.incus(requestContext, options.Project, incusExecArgument, options.VMB, "--", "true")
	result <- watchdogCommandResult{result: heartbeat, err: err}
}

// validatePressureResult requires the guest helper to bind and materially execute all requested workloads.
func validatePressureResult(
	data []byte,
	expectedDuration time.Duration,
	workload guestPressureWorkload,
) error {
	var result pressure.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode guest pressure result: %w", err)
	}
	if result.CPUWorkers != workload.cpuWorkers || result.CPUHashes == 0 {
		return errors.New("guest pressure did not exercise the requested CPU workers")
	}
	if result.MemoryBytes != workload.memoryBytes {
		return errors.New("guest pressure did not hold the requested memory")
	}
	if result.DiskTargetBytes != workload.diskBytes {
		return errors.New("guest pressure result did not bind the requested disk target")
	}
	diskFloor := min(workload.diskBytes, int64(minimumMaterialDiskBytes))
	if result.DiskFileBytes < diskFloor || result.DiskFileBytes > workload.diskBytes ||
		result.DiskBytesWritten < result.DiskFileBytes {
		return errors.New("guest pressure did not perform the minimum material disk workload")
	}
	if result.Elapsed < expectedDuration || result.Elapsed > expectedDuration+30*time.Second {
		return errors.New("guest pressure duration fell outside the accepted window")
	}
	return nil
}
