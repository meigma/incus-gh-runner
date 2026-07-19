package liveacceptance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// instanceState contains the effective Incus fields bound to the runtime assertions.
type instanceState struct {
	Type           string            `json:"type"`
	Profiles       []string          `json:"profiles"`
	Config         map[string]string `json:"config"`
	ExpandedConfig map[string]string `json:"expanded_config"`
}

// instanceRuntimeState contains the daemon runtime PID bound to host process inspection.
type instanceRuntimeState struct {
	PID int `json:"pid"`
}

// guestObservation records the effective VM, firmware, and guest resource state.
type guestObservation struct {
	Instance          string        `json:"instance"`
	KVM               kvmProcess    `json:"kvm"`
	Virtualization    string        `json:"virtualization"`
	SecureBootEnabled bool          `json:"secure_boot_enabled"`
	AgentRoundTrip    bool          `json:"agent_file_round_trip"`
	GuestAPIAbsent    bool          `json:"guest_api_absent"`
	CPUCount          int           `json:"cpu_count"`
	MemoryBytes       uint64        `json:"memory_bytes"`
	RootBytes         uint64        `json:"root_filesystem_bytes"`
	InstanceState     instanceState `json:"instance_state"`
}

// observeGuest proves one running instance uses the exact image, profile, KVM, firmware, and resource limits.
func observeGuest( //nolint:funlen,gocognit // Sequential fail-closed assertions keep the retained evidence order explicit.
	ctx context.Context,
	commands commandRunner,
	writer *evidenceWriter,
	options Options,
	baseline RuntimeBaseline,
	instance string,
) (guestObservation, []byte, error) {
	state, rawState, err := readInstanceState(ctx, commands, options.Project, instance)
	if err != nil {
		return guestObservation{}, nil, err
	}
	if state.Type != "virtual-machine" {
		return guestObservation{}, nil, fmt.Errorf("instance %q is not a virtual machine", instance)
	}
	if len(state.Profiles) != 1 || state.Profiles[0] != options.Profile {
		return guestObservation{}, nil, fmt.Errorf("instance %q does not use exactly the baseline profile", instance)
	}
	if state.ExpandedConfig["security.secureboot"] != "true" {
		return guestObservation{}, nil, fmt.Errorf("instance %q does not require Secure Boot", instance)
	}
	if state.ExpandedConfig["security.guestapi"] != falseValue {
		return guestObservation{}, nil, fmt.Errorf("instance %q exposes the Incus guest API", instance)
	}
	if state.Config["volatile.base_image"] != options.ImageFingerprint {
		return guestObservation{}, nil, fmt.Errorf("instance %q does not use the expected image fingerprint", instance)
	}

	runtimePID, err := readInstanceRuntimePID(ctx, commands, options.Project, instance)
	if err != nil {
		return guestObservation{}, nil, err
	}
	kvm, err := inspectKVMProcess("/proc", runtimePID, instance)
	if err != nil {
		return guestObservation{}, nil, err
	}
	virtualization, err := guestOutput(ctx, commands, options.Project, instance, "systemd-detect-virt")
	if err != nil {
		return guestObservation{}, nil, err
	}
	if strings.TrimSpace(virtualization) != "kvm" {
		return guestObservation{}, nil, fmt.Errorf(
			"instance %q reports unexpected virtualization %q",
			instance,
			strings.TrimSpace(virtualization),
		)
	}
	secureBoot, err := guestOutput(
		ctx,
		commands,
		options.Project,
		instance,
		"bash",
		"-c",
		`set -Eeuo pipefail; shopt -s nullglob; vars=(/sys/firmware/efi/efivars/SecureBoot-*); [[ ${#vars[@]} -eq 1 ]]; od -An -j4 -N1 -t u1 "${vars[0]}"`,
	)
	if err != nil {
		return guestObservation{}, nil, fmt.Errorf("read Secure Boot state from %q: %w", instance, err)
	}
	if strings.TrimSpace(secureBoot) != "1" {
		return guestObservation{}, nil, fmt.Errorf("instance %q did not report active Secure Boot", instance)
	}
	guestAPI, err := guestOutput(
		ctx,
		commands,
		options.Project,
		instance,
		"bash",
		"-c",
		`[[ ! -e /dev/incus ]] && printf absent`,
	)
	if err != nil || strings.TrimSpace(guestAPI) != "absent" {
		return guestObservation{}, nil, fmt.Errorf("instance %q exposes /dev/incus", instance)
	}
	cpuRaw, err := guestOutput(ctx, commands, options.Project, instance, "nproc")
	if err != nil {
		return guestObservation{}, nil, err
	}
	cpuCount, err := strconv.Atoi(strings.TrimSpace(cpuRaw))
	if err != nil || cpuCount != baseline.RunnerCPU {
		return guestObservation{}, nil, fmt.Errorf(
			"instance %q exposes %q CPUs; expected %d",
			instance,
			strings.TrimSpace(cpuRaw),
			baseline.RunnerCPU,
		)
	}
	memoryRaw, err := guestOutput(
		ctx,
		commands,
		options.Project,
		instance,
		"awk",
		`/^MemTotal:/ {printf "%.0f\n", $2 * 1024}`,
		"/proc/meminfo",
	)
	if err != nil {
		return guestObservation{}, nil, err
	}
	memoryBytes, err := parseGuestQuantity(memoryRaw)
	if err != nil || memoryBytes > baseline.RunnerMemoryBytes || memoryBytes < baseline.RunnerMemoryBytes*9/10 {
		return guestObservation{}, nil, fmt.Errorf("instance %q memory does not match the configured ceiling", instance)
	}
	rootRaw, err := guestOutput(ctx, commands, options.Project, instance, "df", "--block-size=1", "--output=size", "/")
	if err != nil {
		return guestObservation{}, nil, err
	}
	rootFields := strings.Fields(rootRaw)
	if len(rootFields) != rootSizeFieldCount {
		return guestObservation{}, nil, fmt.Errorf("instance %q returned unexpected root size output", instance)
	}
	rootBytes, err := parseGuestQuantity(rootFields[1])
	if err != nil || rootBytes > baseline.RunnerRootDiskBytes || rootBytes < baseline.RunnerRootDiskBytes/2 {
		return guestObservation{}, nil, fmt.Errorf(
			"instance %q root filesystem does not match the configured ceiling",
			instance,
		)
	}

	canary, err := agentRoundTrip(ctx, commands, writer, options.Project, instance)
	if err != nil {
		return guestObservation{}, nil, err
	}
	return guestObservation{
		Instance:          instance,
		KVM:               kvm,
		Virtualization:    strings.TrimSpace(virtualization),
		SecureBootEnabled: true,
		AgentRoundTrip:    true,
		GuestAPIAbsent:    true,
		CPUCount:          cpuCount,
		MemoryBytes:       memoryBytes,
		RootBytes:         rootBytes,
		InstanceState:     state,
	}, canary, writer.write(fmt.Sprintf("instance-%s.json", instance), rawState)
}

// readInstanceRuntimePID reads the exact runtime PID reported by the Incus instance-state API.
func readInstanceRuntimePID(
	ctx context.Context,
	commands commandRunner,
	project string,
	instance string,
) (int, error) {
	requestContext, cancel := context.WithTimeout(ctx, shortCommandTimeout)
	defer cancel()
	result, err := commands.run(
		requestContext,
		"incus",
		"query",
		fmt.Sprintf("/1.0/instances/%s/state?project=%s", instance, project),
	)
	if err != nil {
		return 0, err
	}
	if checkErr := requireSuccess("read instance runtime state", result); checkErr != nil {
		return 0, checkErr
	}
	var state instanceRuntimeState
	if decodeErr := json.Unmarshal(result.stdout, &state); decodeErr != nil {
		return 0, fmt.Errorf("decode instance runtime state: %w", decodeErr)
	}
	if state.PID <= 0 {
		return 0, errors.New("instance runtime state omitted a positive PID")
	}
	return state.PID, nil
}

// readInstanceState reads the recursive API representation needed for effective VM assertions.
func readInstanceState(
	ctx context.Context,
	commands commandRunner,
	project string,
	instance string,
) (instanceState, []byte, error) {
	requestContext, cancel := context.WithTimeout(ctx, shortCommandTimeout)
	defer cancel()
	result, err := commands.run(
		requestContext,
		"incus",
		"query",
		fmt.Sprintf("/1.0/instances/%s?project=%s&recursion=1", instance, project),
	)
	if err != nil {
		return instanceState{}, nil, err
	}
	if err := requireSuccess("read recursive instance state", result); err != nil {
		return instanceState{}, nil, err
	}
	var state instanceState
	if err := json.Unmarshal(result.stdout, &state); err != nil {
		return instanceState{}, nil, fmt.Errorf("decode instance state: %w", err)
	}
	return state, append([]byte(nil), result.stdout...), nil
}

// guestOutput runs one bounded command through the Incus VM agent and returns stdout.
func guestOutput(
	ctx context.Context,
	commands incusCommandRunner,
	project string,
	instance string,
	arguments ...string,
) (string, error) {
	requestContext, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	incusArguments := append([]string{incusExecArgument, instance, "--"}, arguments...)
	result, err := commands.incus(requestContext, project, incusArguments...)
	if err != nil {
		return "", err
	}
	if err := requireSuccess("execute guest probe", result); err != nil {
		return "", err
	}
	return string(result.stdout), nil
}

// parseGuestQuantity parses one unsigned decimal quantity returned by a guest tool.
func parseGuestQuantity(value string) (uint64, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed == 0 {
		return 0, errors.New("guest returned an invalid positive quantity")
	}
	return parsed, nil
}

// agentRoundTrip proves host-to-agent push and pull without retaining the synthetic canary.
func agentRoundTrip(
	ctx context.Context,
	commands commandRunner,
	writer *evidenceWriter,
	project string,
	instance string,
) ([]byte, error) {
	canaryBytes := make([]byte, canaryRandomBytes)
	if _, err := rand.Read(canaryBytes); err != nil {
		return nil, fmt.Errorf("generate agent canary: %w", err)
	}
	canary := []byte("agent-canary-" + hex.EncodeToString(canaryBytes))
	directory, err := os.MkdirTemp(writer.directory, ".agent-round-trip-")
	if err != nil {
		return nil, fmt.Errorf("create agent round-trip directory: %w", err)
	}
	defer os.RemoveAll(directory)
	source := filepath.Join(directory, "source")
	destination := filepath.Join(directory, "destination")
	if writeErr := os.WriteFile(source, canary, 0o600); writeErr != nil {
		return nil, fmt.Errorf("write agent canary: %w", writeErr)
	}

	pushContext, cancelPush := context.WithTimeout(ctx, commandTimeout)
	push, err := commands.incus(
		pushContext,
		project,
		"file",
		"push",
		source,
		instance+"/run/incus-gh-runner-agent-canary",
	)
	cancelPush()
	if err != nil {
		return nil, err
	}
	if checkErr := requireSuccess("push synthetic agent canary", push); checkErr != nil {
		return nil, checkErr
	}
	pullContext, cancelPull := context.WithTimeout(ctx, commandTimeout)
	pull, err := commands.incus(
		pullContext,
		project,
		"file",
		"pull",
		instance+"/run/incus-gh-runner-agent-canary",
		destination,
	)
	cancelPull()
	if err != nil {
		return nil, err
	}
	if checkErr := requireSuccess("pull synthetic agent canary", pull); checkErr != nil {
		return nil, checkErr
	}
	returned, err := os.ReadFile(destination)
	if err != nil {
		return nil, fmt.Errorf("read returned agent canary: %w", err)
	}
	if sha256.Sum256(returned) != sha256.Sum256(canary) {
		return nil, errors.New("agent round-trip changed the synthetic canary")
	}
	_, _ = guestOutput(ctx, commands, project, instance, "rm", "-f", "/run/incus-gh-runner-agent-canary")
	return canary, nil
}
