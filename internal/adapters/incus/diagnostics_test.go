package incus_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/adapters/incus"
)

func TestDirectoryDiagnosticsSinkStoresProtectedConsoleEvidence(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "diagnostics")
	sink, err := incus.NewDirectoryDiagnosticsSink(directory)
	require.NoError(t, err)

	err = sink.Store(context.Background(), incus.Diagnostics{
		RunnerID: "incus-gh-runner-123",
		Console:  []byte("terminal evidence\n"),
	})

	require.NoError(t, err)
	path := filepath.Join(directory, "incus-gh-runner-123.console.log")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "terminal evidence\n", string(content))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestDirectoryDiagnosticsSinkRejectsUnsafeRunnerID(t *testing.T) {
	t.Parallel()

	sink, err := incus.NewDirectoryDiagnosticsSink(t.TempDir())
	require.NoError(t, err)

	err = sink.Store(context.Background(), incus.Diagnostics{RunnerID: "../unowned"})

	assert.EqualError(t, err, "diagnostics runner ID is not a safe filename")
}

func TestDirectoryDiagnosticsSinkRefusesToReplaceExistingFile(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "diagnostics")
	require.NoError(t, os.Mkdir(directory, 0o700))
	path := filepath.Join(directory, "incus-gh-runner-123.console.log")
	require.NoError(t, os.WriteFile(path, []byte("existing evidence\n"), 0o600))
	sink, err := incus.NewDirectoryDiagnosticsSink(directory)
	require.NoError(t, err)

	err = sink.Store(context.Background(), incus.Diagnostics{
		RunnerID: "incus-gh-runner-123",
		Console:  []byte("replacement\n"),
	})

	require.ErrorIs(t, err, os.ErrExist)
	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "existing evidence\n", string(content))
}

func TestDirectoryDiagnosticsSinkRefusesOversizedConsole(t *testing.T) {
	t.Parallel()

	sink, err := incus.NewDirectoryDiagnosticsSink(t.TempDir())
	require.NoError(t, err)

	err = sink.Store(context.Background(), incus.Diagnostics{
		RunnerID: "incus-gh-runner-123",
		Console:  make([]byte, 1024*1024+1),
	})

	assert.EqualError(t, err, "runner diagnostics exceed 1048576-byte limit")
}

func TestDirectoryDiagnosticsSinkRejectsBroadDirectoryPermissions(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	require.NoError(t, os.Chmod(directory, 0o755))
	sink, err := incus.NewDirectoryDiagnosticsSink(directory)
	require.NoError(t, err)

	err = sink.Store(context.Background(), incus.Diagnostics{RunnerID: "incus-gh-runner-123"})

	assert.EqualError(t, err, "diagnostics directory permissions must be 0700, got 0755")
}
