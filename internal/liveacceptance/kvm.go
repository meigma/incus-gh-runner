package liveacceptance

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// kvmProcess identifies the VM monitor process and its open KVM file descriptors.
type kvmProcess struct {
	PID int      `json:"pid"`
	FDs []string `json:"kvm_fds"`
}

// inspectKVMProcess proves the API-reported runtime PID is the exact named QEMU process with KVM open.
func inspectKVMProcess(procRoot string, pid int, instance string) (kvmProcess, error) {
	processRoot := filepath.Join(procRoot, strconv.Itoa(pid))
	cmdline, err := os.ReadFile(filepath.Join(processRoot, "cmdline"))
	if err != nil {
		return kvmProcess{}, fmt.Errorf("read API-reported runtime PID %d: %w", pid, err)
	}
	arguments := bytes.Split(bytes.TrimSuffix(cmdline, []byte{0}), []byte{0})
	if len(arguments) == 0 || !bytes.Contains(arguments[0], []byte("qemu-system")) ||
		!argumentsContainPair(arguments, []byte("-name"), []byte(instance)) {
		return kvmProcess{}, fmt.Errorf("API-reported PID %d is not QEMU for %q", pid, instance)
	}
	fds, err := kvmFileDescriptors(filepath.Join(processRoot, "fd"))
	if err != nil {
		return kvmProcess{}, fmt.Errorf("inspect KVM descriptors for PID %d: %w", pid, err)
	}
	if len(fds) == 0 {
		return kvmProcess{}, fmt.Errorf("QEMU PID %d for %q does not have /dev/kvm open", pid, instance)
	}
	return kvmProcess{PID: pid, FDs: fds}, nil
}

// argumentsContainPair matches two adjacent, complete process arguments.
func argumentsContainPair(arguments [][]byte, key []byte, value []byte) bool {
	for index := 0; index+1 < len(arguments); index++ {
		if bytes.Equal(arguments[index], key) && bytes.Equal(arguments[index+1], value) {
			return true
		}
	}
	return false
}

// kvmFileDescriptors returns descriptor names whose targets are exactly the host KVM device.
func kvmFileDescriptors(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	fds := make([]string, 0, 1)
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(directory, entry.Name()))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if target == "/dev/kvm" {
			fds = append(fds, entry.Name())
		}
	}
	sort.Strings(fds)
	return fds, nil
}
