package runtime

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
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
			} else {
				assert.Nil(t, prepared.signer)
			}
		})
	}
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
