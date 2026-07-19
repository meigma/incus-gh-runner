package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	incuspolicy "github.com/meigma/incus-gh-runner/deploy/incus"
)

// TestValidateIncusBaselineBoundsInputBeforeSocketAccess proves oversized files cannot exhaust memory or reach Incus.
func TestValidateIncusBaselineBoundsInputBeforeSocketAccess(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join(t.TempDir(), "oversized.json")
	file, err := os.Create(baselinePath)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(incuspolicy.MaximumBaselineBytes+1))
	require.NoError(t, file.Close())

	_, err = validateIncusBaseline(context.Background(), baselinePath, "/missing/incus.socket")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "baseline exceeds")
	assert.NotContains(t, err.Error(), "connect Incus")
}

// TestReadIncusBaselineRejectsNonRegularFiles proves special inputs cannot block the validator's file read.
func TestReadIncusBaselineRejectsNonRegularFiles(t *testing.T) {
	t.Parallel()

	_, err := readIncusBaseline(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a regular file")
}
