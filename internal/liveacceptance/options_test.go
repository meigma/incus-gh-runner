package liveacceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/incusvalidate"
)

// TestOptionsRequireTenMinuteEvidencePressure proves short diagnostic runs cannot emit passing acceptance evidence.
func TestOptionsRequireTenMinuteEvidencePressure(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	executable := filepath.Join(directory, "acceptance")
	require.NoError(t, os.WriteFile(executable, []byte("probe"), 0o700))
	baseline := RuntimeBaseline{
		MaximumRunners: acceptanceRunnerCount,
		Manifest: incusvalidate.Baseline{
			Names: incusvalidate.Names{Project: "acceptance", Profile: "runner"},
		},
	}
	options := Options{
		Project:           "acceptance",
		Profile:           "runner",
		ImageFingerprint:  strings.Repeat("a", 64),
		VMA:               "runner-a",
		VMB:               "runner-b",
		RunID:             "hostile-test",
		AllowedURL:        "https://github.com",
		EvidenceDirectory: filepath.Join(directory, "evidence"),
		StressDuration:    time.Minute,
		PollInterval:      time.Second,
		SelfPath:          executable,
	}

	err := options.validate(baseline)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "between 10 and 15 minutes")
}
