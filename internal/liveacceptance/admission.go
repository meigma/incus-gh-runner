package liveacceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const admissionCleanupAbsentConfirmations = 2

// admissionCommandRunner is the command boundary used by the admission proof.
type admissionCommandRunner interface {
	run(context.Context, string, ...string) (commandResult, error)
	incus(context.Context, string, ...string) (commandResult, error)
}

// admissionObservation records the full-capacity project state and rejected extra VM.
type admissionObservation struct {
	ProbeInstance string `json:"probe_instance"`
	Rejected      bool   `json:"rejected"`
	Reason        string `json:"reason"`
}

// instanceListEntry contains the exact name needed for admission-probe inventory.
type instanceListEntry struct {
	Name string `json:"name"`
}

// proveAdmissionCeiling requires a marker-owned third VM to fail while two exact-profile VMs consume capacity.
func proveAdmissionCeiling(
	ctx context.Context,
	commands admissionCommandRunner,
	writer *evidenceWriter,
	options Options,
) (admissionObservation, error) {
	name := admissionProbeName(options.RunID)
	if err := requireInstanceAbsent(ctx, commands, options.Project, name); err != nil {
		return admissionObservation{}, err
	}
	observation, probeErr := executeAdmissionCeiling(ctx, commands, writer, options, name)
	cleanupErr := cleanupAdmissionProbe(ctx, commands, options.Project, name, options.RunID)
	return observation, errors.Join(probeErr, cleanupErr)
}

// executeAdmissionCeiling captures capacity and attempts the marker-owned extra VM initialization.
func executeAdmissionCeiling(
	ctx context.Context,
	commands admissionCommandRunner,
	writer *evidenceWriter,
	options Options,
	name string,
) (admissionObservation, error) {
	stateContext, cancelState := context.WithTimeout(ctx, stateCommandTimeout)
	state, err := commands.run(
		stateContext,
		"incus",
		"project",
		"info",
		options.Project,
		"--format=json",
	)
	cancelState()
	if err != nil {
		return admissionObservation{}, err
	}
	if checkErr := requireSuccess("read full-capacity project state", state); checkErr != nil {
		return admissionObservation{}, checkErr
	}
	if writeErr := writer.write("project-capacity-before.json", state.stdout); writeErr != nil {
		return admissionObservation{}, writeErr
	}

	probeContext, cancelProbe := context.WithTimeout(ctx, admissionTimeout)
	probe, err := commands.incus(
		probeContext,
		options.Project,
		"init",
		options.ImageFingerprint,
		name,
		"--vm",
		"--profile",
		options.Profile,
		"--config",
		acceptanceIDKey+"="+options.RunID,
		"--config",
		disposableVMKey+"=true",
	)
	cancelProbe()
	if err != nil {
		return admissionObservation{}, err
	}
	combined := append(append([]byte(nil), probe.stdout...), probe.stderr...)
	if err := writer.write("project-capacity-rejection.log", combined); err != nil {
		return admissionObservation{}, err
	}
	if probe.succeeded() {
		return admissionObservation{}, errors.New("project admitted a third VM above the configured ceiling")
	}
	reason := strings.TrimSpace(string(probe.stderr))
	if !matchesProjectInstanceLimit(reason, options.Project) {
		return admissionObservation{}, errors.New("third VM failed without an identifiable Incus project-limit reason")
	}
	if err := requireInstanceAbsent(ctx, commands, options.Project, name); err != nil {
		return admissionObservation{}, err
	}
	return admissionObservation{ProbeInstance: name, Rejected: true, Reason: boundedReason(reason)}, nil
}

// admissionProbeName derives one safe, run-scoped instance name within the Incus limit.
func admissionProbeName(runID string) string {
	name := "incus-gh-runner-" + runID + "-limit"
	if len(name) > localNameMaxLength {
		name = name[:localNameMaxLength]
	}
	return name
}

// matchesProjectInstanceLimit recognizes only Incus instance-ceiling errors for the expected project.
func matchesProjectInstanceLimit(reason string, project string) bool {
	reason = strings.TrimSpace(reason)
	totalLimit := fmt.Sprintf("Reached maximum number of instances in project %q", project)
	virtualMachineLimit := fmt.Sprintf(
		"Reached maximum number of instances of type %q in project %q",
		"virtual-machine",
		project,
	)
	return strings.HasSuffix(reason, totalLimit) || strings.HasSuffix(reason, virtualMachineLimit)
}

// boundedReason retains a useful but bounded rejection reason in evidence summaries.
func boundedReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > maximumReasonBytes {
		return reason[:maximumReasonBytes] + "..."
	}
	return reason
}

// requireInstanceAbsent confirms that one exact generated instance name does not exist.
func requireInstanceAbsent(
	ctx context.Context,
	commands admissionCommandRunner,
	project string,
	instance string,
) error {
	exists, err := instanceExists(ctx, commands, project, instance)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("generated admission-probe instance already exists: %s", instance)
	}
	return nil
}

// instanceExists reads successful project inventory and matches one exact instance name.
func instanceExists(
	ctx context.Context,
	commands admissionCommandRunner,
	project string,
	instance string,
) (bool, error) {
	requestContext, cancel := context.WithTimeout(ctx, shortCommandTimeout)
	defer cancel()
	result, err := commands.incus(requestContext, project, listArgument, "--format=json")
	if err != nil {
		return false, err
	}
	if checkErr := requireSuccess("list admission-probe instances", result); checkErr != nil {
		return false, checkErr
	}
	var instances []instanceListEntry
	if decodeErr := json.Unmarshal(result.stdout, &instances); decodeErr != nil {
		return false, fmt.Errorf("decode admission-probe inventory: %w", decodeErr)
	}
	for _, candidate := range instances {
		if candidate.Name == instance {
			return true, nil
		}
	}
	return false, nil
}

// cleanupAdmissionProbe removes one exact marker-owned capacity probe using a fresh bounded context.
func cleanupAdmissionProbe(
	ctx context.Context,
	commands admissionCommandRunner,
	project string,
	instance string,
	runID string,
) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationTimeout)
	defer cancel()
	return cleanupAdmissionProbeEventually(
		cleanupContext,
		commands,
		project,
		instance,
		runID,
		ipv6ListenerPollInterval,
		admissionCleanupAbsentConfirmations,
	)
}

// cleanupAdmissionProbeEventually deletes a marker-owned probe and requires stable observed absence.
func cleanupAdmissionProbeEventually(
	ctx context.Context,
	commands admissionCommandRunner,
	project string,
	instance string,
	runID string,
	pollInterval time.Duration,
	absentConfirmations int,
) error {
	if absentConfirmations < 1 {
		return errors.New("admission cleanup requires at least one absence confirmation")
	}

	consecutiveAbsent := 0
	deleteOutstanding := false
	for {
		exists, err := instanceExists(ctx, commands, project, instance)
		if err != nil {
			return fmt.Errorf("verify admission-probe cleanup: %w", err)
		}
		if !exists {
			consecutiveAbsent++
			deleteOutstanding = false
			if consecutiveAbsent >= absentConfirmations {
				return nil
			}
			if err := waitForAdmissionCleanupPoll(ctx, pollInterval); err != nil {
				return fmt.Errorf("wait for admission-probe cleanup: %w", err)
			}
			continue
		}

		consecutiveAbsent = 0
		if !deleteOutstanding {
			if err := deleteMarkedInstance(ctx, commands, project, instance, runID); err != nil {
				return fmt.Errorf("delete admission-probe during cleanup: %w", err)
			}
			deleteOutstanding = true
		}
		if err := waitForAdmissionCleanupPoll(ctx, pollInterval); err != nil {
			return fmt.Errorf("wait for admission-probe cleanup: %w", err)
		}
	}
}

// waitForAdmissionCleanupPoll waits for the next inventory observation or cancellation.
func waitForAdmissionCleanupPoll(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// deleteMarkedInstance removes one generated instance only after re-reading both acceptance markers.
func deleteMarkedInstance(
	ctx context.Context,
	commands admissionCommandRunner,
	project string,
	instance string,
	runID string,
) error {
	actualID, err := instanceConfigValue(ctx, commands, project, instance, acceptanceIDKey)
	if err != nil {
		return err
	}
	disposable, err := instanceConfigValue(ctx, commands, project, instance, disposableVMKey)
	if err != nil {
		return err
	}
	if actualID != runID || disposable != trueValue {
		return errors.New("refusing to delete admission-probe instance whose markers changed")
	}
	deleteContext, cancel := context.WithTimeout(ctx, mutationTimeout)
	defer cancel()
	result, err := commands.incus(deleteContext, project, "delete", "--force", instance)
	if err != nil {
		return err
	}
	return requireSuccess("delete marker-owned admission-probe instance", result)
}

// instanceConfigValue reads one marker value from a generated acceptance instance.
func instanceConfigValue(
	ctx context.Context,
	commands admissionCommandRunner,
	project string,
	instance string,
	key string,
) (string, error) {
	requestContext, cancel := context.WithTimeout(ctx, shortCommandTimeout)
	defer cancel()
	result, err := commands.incus(requestContext, project, "config", "get", instance, key)
	if err != nil {
		return "", err
	}
	if err := requireSuccess("read admission-probe marker", result); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(result.stdout)), nil
}
