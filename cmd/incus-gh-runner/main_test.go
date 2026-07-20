package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	incuspolicy "github.com/meigma/incus-gh-runner/deploy/incus"
	"github.com/meigma/incus-gh-runner/internal/provenance"
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

// TestVerifyJobProof authenticates a real envelope through the command's file adapter.
func TestVerifyJobProof(t *testing.T) {
	t.Parallel()

	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	signer, err := provenance.NewSigner(privateKey)
	require.NoError(t, err)
	payload := provenance.Payload{
		Version:  provenance.Version,
		Claim:    provenance.Claim,
		IssuedAt: time.Date(2026, 7, 20, 20, 15, 32, 0, time.UTC),
		Host: provenance.Host{
			ID:                "builder-host-01",
			ControllerVersion: "1.1.0",
			ControllerCommit:  "0123456789abcdef",
		},
		GitHub: provenance.GitHub{
			Owner:           "meigma",
			Repository:      "builder-images",
			WorkflowRef:     "meigma/builder-images/.github/workflows/build.yml@refs/heads/main",
			WorkflowRunID:   123,
			JobID:           "job-123",
			RunnerRequestID: 456,
			RunnerID:        789,
			RunnerName:      "incus-gh-runner-test",
			EventName:       "workflow_dispatch",
			ScaleSetID:      42,
			ScaleSetName:    "incus-linux-x64",
		},
		Machine: provenance.Machine{
			IncusProject:              "github-runners",
			InstanceName:              "incus-gh-runner-test",
			InstanceUUID:              "fedcba98-7654-3210-fedc-ba9876543210",
			ImageFingerprint:          strings.Repeat("1", 64),
			LaunchConfigurationSHA256: strings.Repeat("2", 64),
			Profiles:                  []provenance.Profile{},
		},
	}
	envelope, err := signer.Sign(t.Context(), payload)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(privateKey.Public())
	require.NoError(t, err)
	directory := t.TempDir()
	proofPath := filepath.Join(directory, "job-proof.dsse.json")
	publicKeyPath := filepath.Join(directory, "machine-provenance-key.pub.pem")
	require.NoError(t, os.WriteFile(proofPath, envelope, 0o600))
	require.NoError(t, os.WriteFile(
		publicKeyPath,
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}),
		0o600,
	))

	verified, err := verifyJobProof(t.Context(), proofPath, publicKeyPath, "builder-host-01")

	require.NoError(t, err)
	assert.Contains(t, string(verified), `"job_id":"job-123"`)
	_, err = verifyJobProof(t.Context(), proofPath, publicKeyPath, "builder-host-02")
	require.ErrorContains(t, err, "host ID does not match")
}
