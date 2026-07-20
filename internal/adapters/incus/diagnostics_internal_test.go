package incus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryDiagnosticsSinkDoesNotPruneExistingDestinationAtCapacity(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "diagnostics")
	require.NoError(t, os.Mkdir(directory, diagnosticsDirectoryMode))
	target := filepath.Join(directory, "runner-000.console.log")
	for index := range maximumDiagnosticsFiles {
		path := filepath.Join(directory, fmt.Sprintf("runner-%03d.console.log", index))
		require.NoError(t, os.WriteFile(path, []byte("existing"), 0o600))
		timestamp := time.Unix(int64(index+1), 0)
		require.NoError(t, os.Chtimes(path, timestamp, timestamp))
	}
	sink, err := NewDirectoryDiagnosticsSink(directory)
	require.NoError(t, err)

	err = sink.Store(context.Background(), Diagnostics{
		RunnerID: "runner-000",
		Console:  []byte("replacement"),
	})

	require.ErrorIs(t, err, os.ErrExist)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "existing", string(content))
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	assert.Len(t, entries, maximumDiagnosticsFiles)
}

func TestPruneDiagnosticsRemovesOversizedAndOldestFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	oldest := filepath.Join(directory, "oldest.console.log")
	newest := filepath.Join(directory, "newest.console.log")
	oversized := filepath.Join(directory, "oversized.console.log")
	require.NoError(t, os.WriteFile(oldest, []byte("old"), 0o600))
	require.NoError(t, os.WriteFile(newest, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(oversized, make([]byte, maximumConsoleLogBytes+1), 0o600))
	require.NoError(t, os.Chtimes(oldest, time.Unix(1, 0), time.Unix(1, 0)))
	require.NoError(t, os.Chtimes(newest, time.Unix(2, 0), time.Unix(2, 0)))

	err := pruneDiagnostics(directory, 1)

	require.NoError(t, err)
	_, oldestErr := os.Stat(oldest)
	require.ErrorIs(t, oldestErr, os.ErrNotExist)
	_, oversizedErr := os.Stat(oversized)
	require.ErrorIs(t, oversizedErr, os.ErrNotExist)
	content, err := os.ReadFile(newest)
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
}
