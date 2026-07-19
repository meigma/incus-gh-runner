package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
)

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
