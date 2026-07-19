package liveacceptance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInspectKVMProcessRequiresTheAPIPIDExactVMAndDevice proves another emulator cannot satisfy the KVM gate.
func TestInspectKVMProcessRequiresTheAPIPIDExactVMAndDevice(t *testing.T) {
	t.Parallel()

	procRoot := t.TempDir()
	writeProcessFixture(t, procRoot, "100", []string{"qemu-system-x86_64", "-name", "runner-a"}, true)
	writeProcessFixture(t, procRoot, "101", []string{"qemu-system-x86_64", "-name", "runner-b"}, false)
	writeProcessFixture(t, procRoot, "102", []string{"unrelated", "runner-b"}, true)

	observed, err := inspectKVMProcess(procRoot, 100, "runner-a")
	require.NoError(t, err)
	assert.Equal(t, 100, observed.PID)
	assert.Equal(t, []string{"7"}, observed.FDs)

	_, err = inspectKVMProcess(procRoot, 101, "runner-b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have /dev/kvm open")

	_, err = inspectKVMProcess(procRoot, 100, "runner")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not QEMU")
}

// writeProcessFixture creates one isolated process-filesystem shape for KVM discovery.
func writeProcessFixture(t *testing.T, procRoot string, pid string, arguments []string, hasKVM bool) {
	t.Helper()
	directory := filepath.Join(procRoot, pid)
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "fd"), 0o700))
	cmdline := []byte{}
	for _, argument := range arguments {
		cmdline = append(cmdline, []byte(argument)...)
		cmdline = append(cmdline, 0)
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "cmdline"), cmdline, 0o600))
	if hasKVM {
		require.NoError(t, os.Symlink("/dev/kvm", filepath.Join(directory, "fd", "7")))
	}
}
