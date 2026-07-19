package liveacceptance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// commandResult contains bounded output and timing for one external probe command.
type commandResult struct {
	stdout   []byte
	stderr   []byte
	duration time.Duration
	exitCode int
}

// boundedCommandBuffer retains command output up to a fixed byte limit while allowing the child to exit normally.
type boundedCommandBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

// Write retains only the allowed prefix and records whether the child produced excess output.
func (b *boundedCommandBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := max(b.limit-b.buffer.Len(), 0)
	if len(data) > remaining {
		b.overflow = true
		data = data[:remaining]
	}
	_, _ = b.buffer.Write(data)
	return written, nil
}

// bytes returns the retained output prefix.
func (b *boundedCommandBuffer) bytes() []byte {
	return b.buffer.Bytes()
}

// succeeded reports whether the command returned a zero exit status.
func (r commandResult) succeeded() bool {
	return r.exitCode == 0
}

// commandRunner executes trusted, argument-separated host commands without a shell.
type commandRunner struct{}

// incusCommandRunner is the narrow command boundary used by guest-agent probes.
type incusCommandRunner interface {
	incus(context.Context, string, ...string) (commandResult, error)
}

// run executes one command and preserves stdout, stderr, duration, and exit status.
func (commandRunner) run(ctx context.Context, name string, arguments ...string) (commandResult, error) {
	started := time.Now()
	command := exec.CommandContext(ctx, name, arguments...)
	stdout := boundedCommandBuffer{limit: maximumCommandStdoutBytes}
	stderr := boundedCommandBuffer{limit: maximumCommandStderrBytes}
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := commandResult{
		stdout:   stdout.bytes(),
		stderr:   stderr.bytes(),
		duration: time.Since(started),
		exitCode: 0,
	}
	if stdout.overflow || stderr.overflow {
		return result, fmt.Errorf(
			"execute %s: command output exceeded the %d-byte stdout or %d-byte stderr limit",
			name,
			maximumCommandStdoutBytes,
			maximumCommandStderrBytes,
		)
	}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, nil
	}
	result.exitCode = -1
	return result, fmt.Errorf("execute %s: %w", name, err)
}

// requireSuccess converts one non-zero command result into a secret-safe error.
func requireSuccess(label string, result commandResult) error {
	if result.succeeded() {
		return nil
	}
	detail := strings.TrimSpace(string(result.stderr))
	if len(detail) > maximumCommandErrorBytes {
		detail = detail[:maximumCommandErrorBytes] + "..."
	}
	if detail == "" {
		detail = fmt.Sprintf("exit status %d", result.exitCode)
	}
	return fmt.Errorf("%s failed: %s", label, detail)
}

// incus runs one Incus command in the selected project.
func (commandRunner) incus(ctx context.Context, project string, arguments ...string) (commandResult, error) {
	projectArguments := make([]string, 0, len(arguments)+acceptanceRunnerCount)
	projectArguments = append(projectArguments, "--project", project)
	projectArguments = append(projectArguments, arguments...)
	return commandRunner{}.run(ctx, "incus", projectArguments...)
}
