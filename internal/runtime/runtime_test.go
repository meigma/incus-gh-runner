package runtime

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubadapter "github.com/meigma/incus-gh-runner/internal/adapters/github"
	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/controller"
)

func TestRunRejectsInvalidConfigurationBeforeConstructingAdapters(t *testing.T) {
	t.Parallel()

	cfg := config.Defaults()
	cfg.GitHub = config.GitHub{
		ConfigURL:   "http://github.com/meigma/incus-gh-runner",
		ScaleSet:    "incus-runners",
		RunnerGroup: "default",
		TokenFile:   filepath.Join(t.TempDir(), "missing-token"),
	}
	cfg.Incus.Project = "runners"
	cfg.Incus.Image = "incus-gh-runner:v1"
	cfg.Incus.Owner = "production"

	err := Run(context.Background(), cfg, BuildInfo{}, nil)

	require.EqualError(
		t,
		err,
		"validate runtime configuration: "+
			"github.config_url must be an absolute HTTPS GitHub organization or repository URL",
	)
}

// TestPrepareJobProofSigner proves optional startup loading is bounded and secret-safe.
func TestPrepareJobProofSigner(t *testing.T) {
	t.Parallel()

	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	validPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
	const secret = "private-material-must-not-appear"
	tests := []struct {
		name       string
		prepare    func(*testing.T) config.JobProof
		wantSigner bool
		wantErr    string
	}{
		{
			name:    "disabled",
			prepare: func(*testing.T) config.JobProof { return config.JobProof{} },
		},
		{
			name: "valid Ed25519 credential",
			prepare: func(t *testing.T) config.JobProof {
				t.Helper()
				path := filepath.Join(t.TempDir(), "machine-provenance-key")
				require.NoError(t, os.WriteFile(path, validPEM, 0o600))
				return config.JobProof{HostID: "builder-host-01", SigningKeyFile: path}
			},
			wantSigner: true,
		},
		{
			name: "malformed credential",
			prepare: func(t *testing.T) config.JobProof {
				t.Helper()
				path := filepath.Join(t.TempDir(), "machine-provenance-key")
				require.NoError(t, os.WriteFile(path, []byte(secret), 0o600))
				return config.JobProof{HostID: "builder-host-01", SigningKeyFile: path}
			},
			wantErr: "load job proof signing key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prepared, prepareErr := prepareJobProofSigner(tt.prepare(t))

			if tt.wantErr != "" {
				require.ErrorContains(t, prepareErr, tt.wantErr)
				assert.NotContains(t, prepareErr.Error(), secret)
				assert.Nil(t, prepared.signer)
				return
			}
			require.NoError(t, prepareErr)
			if tt.wantSigner {
				assert.NotNil(t, prepared.signer)
				assert.NotNil(t, prepared.verifier)
			} else {
				assert.Nil(t, prepared.signer)
				assert.Nil(t, prepared.verifier)
			}
		})
	}
}

// TestDemandSourceOptionsLeaveDisabledJobProofSinkNil exercises the job callback with proofs disabled.
func TestDemandSourceOptionsLeaveDisabledJobProofSinkNil(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg := config.Defaults()
	cfg.GitHub.ScaleSet = "incus-runners"
	options := demandSourceOptions(cfg, 73, logger, nil)
	require.Nil(t, options.JobStartedSink)
	session := &jobStartedMessageSession{}
	source, err := githubadapter.NewDemandSource(session, options)
	require.NoError(t, err)

	err = source.Run(context.Background(), func(_ controller.Demand) {})

	require.ErrorContains(t, err, "test message stream complete")
	assert.Contains(t, logs.String(), "GitHub Actions job started")
	assert.NotContains(t, logs.String(), "GitHub Actions job proof event dropped")
}

func TestResolvePersonalAccessToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(*testing.T) config.GitHub
		want    string
		wantErr string
	}{
		{
			name: "uses environment value",
			prepare: func(*testing.T) config.GitHub {
				return config.GitHub{Token: "environment-token"}
			},
			want: "environment-token",
		},
		{
			name: "reads and trims credential file",
			prepare: func(t *testing.T) config.GitHub {
				t.Helper()
				path := filepath.Join(t.TempDir(), "github-token")
				require.NoError(t, os.WriteFile(path, []byte(" file-token\n"), 0o600))
				return config.GitHub{TokenFile: path}
			},
			want: "file-token",
		},
		{
			name: "rejects missing credential file",
			prepare: func(t *testing.T) config.GitHub {
				t.Helper()
				return config.GitHub{TokenFile: filepath.Join(t.TempDir(), "missing")}
			},
			wantErr: "read GitHub personal access token",
		},
		{
			name: "rejects empty credential file",
			prepare: func(t *testing.T) config.GitHub {
				t.Helper()
				path := filepath.Join(t.TempDir(), "github-token")
				require.NoError(t, os.WriteFile(path, []byte(" \n"), 0o600))
				return config.GitHub{TokenFile: path}
			},
			wantErr: "GitHub personal access token file is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			settings := tt.prepare(t)

			got, err := resolvePersonalAccessToken(settings)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// jobStartedMessageSession emits one job-start event through the upstream GitHub listener.
type jobStartedMessageSession struct {
	emitted bool
}

// GetMessage returns one job-start message before ending the test stream.
func (s *jobStartedMessageSession) GetMessage(
	context.Context,
	int,
	int,
) (*scaleset.RunnerScaleSetMessage, error) {
	if s.emitted {
		return nil, errors.New("test message stream complete")
	}
	s.emitted = true
	return &scaleset.RunnerScaleSetMessage{
		MessageID:  1,
		Statistics: &scaleset.RunnerScaleSetStatistic{},
		JobStartedMessages: []*scaleset.JobStarted{{
			RunnerID:   41,
			RunnerName: "runner-123",
			JobMessageBase: scaleset.JobMessageBase{
				JobID: "job-1",
			},
		}},
	}, nil
}

// DeleteMessage accepts the job-start message acknowledgement.
func (*jobStartedMessageSession) DeleteMessage(context.Context, int) error {
	return nil
}

// AcquireJobs accepts any unexpected job acquisition request.
func (*jobStartedMessageSession) AcquireJobs(_ context.Context, requestIDs []int64) ([]int64, error) {
	return requestIDs, nil
}

// Session returns valid initial statistics for the upstream listener.
func (*jobStartedMessageSession) Session() scaleset.RunnerScaleSetSession {
	return scaleset.RunnerScaleSetSession{
		SessionID:  uuid.New(),
		Statistics: &scaleset.RunnerScaleSetStatistic{},
	}
}
