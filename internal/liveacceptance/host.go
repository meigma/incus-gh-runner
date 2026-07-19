package liveacceptance

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// daemonState records the Incus daemon identity used to detect restarts during pressure.
type daemonState struct {
	MainPID   int `json:"main_pid"`
	NRestarts int `json:"restart_count"`
}

// readDaemonState reads the stable systemd properties used by the runtime watchdog.
func readDaemonState(ctx context.Context, commands commandRunner) (daemonState, error) {
	requestContext, cancel := context.WithTimeout(ctx, shortCommandTimeout)
	defer cancel()
	result, err := commands.run(
		requestContext,
		"systemctl",
		"show",
		"incus.service",
		"--property=MainPID",
		"--property=NRestarts",
	)
	if err != nil {
		return daemonState{}, err
	}
	if checkErr := requireSuccess("read Incus daemon state", result); checkErr != nil {
		return daemonState{}, checkErr
	}
	return parseDaemonState(result.stdout)
}

// parseDaemonState parses named systemd properties without depending on their output order.
func parseDaemonState(output []byte) (daemonState, error) {
	properties := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		name, value, found := strings.Cut(line, "=")
		if found {
			properties[name] = value
		}
	}
	pid, err := strconv.Atoi(properties["MainPID"])
	if err != nil || pid <= 0 {
		return daemonState{}, errors.New("systemd returned an invalid Incus daemon PID")
	}
	restarts, err := strconv.Atoi(properties["NRestarts"])
	if err != nil || restarts < 0 {
		return daemonState{}, errors.New("systemd returned an invalid Incus restart count")
	}
	return daemonState{MainPID: pid, NRestarts: restarts}, nil
}

// memoryState contains host memory totals sampled during guest pressure.
type memoryState struct {
	ObservedAt     time.Time `json:"observed_at"`
	TotalBytes     uint64    `json:"total_bytes"`
	AvailableBytes uint64    `json:"available_bytes"`
}

// readMemoryState parses the host kernel's memory totals without invoking another process.
func readMemoryState(path string, observedAt time.Time) (memoryState, error) {
	file, err := os.Open(path)
	if err != nil {
		return memoryState{}, fmt.Errorf("open host memory state: %w", err)
	}
	defer file.Close()

	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < propertyFieldCount {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		if name != "MemTotal" && name != "MemAvailable" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return memoryState{}, fmt.Errorf("parse host %s: %w", name, err)
		}
		values[name] = value * kibibyte
	}
	if err := scanner.Err(); err != nil {
		return memoryState{}, fmt.Errorf("scan host memory state: %w", err)
	}
	if values["MemTotal"] == 0 || values["MemAvailable"] == 0 {
		return memoryState{}, errors.New("host memory state omitted required values")
	}
	return memoryState{
		ObservedAt:     observedAt.UTC(),
		TotalBytes:     values["MemTotal"],
		AvailableBytes: values["MemAvailable"],
	}, nil
}

// kernelLogHasResourceFailure detects host failures that invalidate the resource-survival claim.
func kernelLogHasResourceFailure(log []byte) bool {
	lower := strings.ToLower(string(log))
	patterns := []string{
		"out of memory",
		"oom-killer",
		"blocked for more than",
		"watchdog: bug",
		"i/o error",
		"zfs: error",
		"zfs fault",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
