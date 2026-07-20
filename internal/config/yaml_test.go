package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
)

func TestValidateYAMLAcceptsExactConfiguration(t *testing.T) {
	t.Parallel()

	data := []byte(`github:
  config_url: https://github.com/meigma/incus-gh-runner
  scale_set: incus-runners
  runner_group: default
  app:
    client_id: Iv1.example
    installation_id: 1234
    private_key_file: /run/credentials/github-app-key.pem
incus:
  project: runners
  image: incus-gh-runner:v1
  profiles: [default, runner]
  owner: production
  bootstrap_timeout: 5m
job_proof:
  host_id: builder-host-01
  signing_key_file: /run/credentials/incus-gh-runner.service/machine-provenance-key
capacity:
  min_runners: 0
  max_runners: 4
reconcile_interval: 1s
`)

	assert.NoError(t, config.ValidateYAML(data))
}

func TestValidateYAMLRejectsInexactConfiguration(t *testing.T) {
	t.Parallel()

	const secret = "must-not-appear-in-the-error"
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{
			name:    "unknown top-level field",
			data:    "runner_group: default\n",
			wantErr: `unknown configuration field "runner_group"`,
		},
		{
			name: "misspelled nested field",
			data: "github:\n" +
				"  runner_gropu: default\n",
			wantErr: `unknown configuration field "github.runner_gropu"`,
		},
		{
			name: "forbidden YAML token",
			data: "github:\n" +
				"  token: " + secret + "\n",
			wantErr: `unknown configuration field "github.token"`,
		},
		{
			name: "duplicate field",
			data: "capacity:\n" +
				"  max_runners: 2\n" +
				"  max_runners: 4\n",
			wantErr: "max_runners",
		},
		{
			name: "quoted integer",
			data: "capacity:\n" +
				"  max_runners: \"4\"\n",
			wantErr: `configuration field "capacity.max_runners" must be an integer`,
		},
		{
			name: "fractional integer",
			data: "capacity:\n" +
				"  max_runners: 4.5\n",
			wantErr: `configuration field "capacity.max_runners" must be an integer`,
		},
		{
			name: "invalid explicitly tagged integer",
			data: "github:\n" +
				"  app:\n" +
				"    installation_id: !!int " + secret + "\n",
			wantErr: `configuration field "github.app.installation_id" must be a valid integer`,
		},
		{
			name: "overflowing integer",
			data: "github:\n" +
				"  app:\n" +
				"    installation_id: !!int 999999999999999999999999999999\n",
			wantErr: `configuration field "github.app.installation_id" must be a valid integer`,
		},
		{
			name: "numeric string setting",
			data: "github:\n" +
				"  config_url: 1234\n",
			wantErr: `configuration field "github.config_url" must be a string`,
		},
		{
			name: "scalar profiles",
			data: "incus:\n" +
				"  profiles: default\n",
			wantErr: `configuration field "incus.profiles" must be a sequence`,
		},
		{
			name:    "numeric duration",
			data:    "reconcile_interval: 1\n",
			wantErr: `configuration field "reconcile_interval" must be a duration string`,
		},
		{
			name:    "invalid duration",
			data:    "reconcile_interval: " + secret + "\n",
			wantErr: `configuration field "reconcile_interval" must be a valid duration string`,
		},
		{
			name: "alias",
			data: "github:\n" +
				"  config_url: &url https://github.com/meigma/incus-gh-runner\n" +
				"  scale_set: *url\n",
			wantErr: `configuration field "github.scale_set" must not use YAML aliases`,
		},
		{
			name: "unknown alias",
			data: "github:\n" +
				"  scale_set: *" + secret + "\n",
			wantErr: "decode YAML configuration: invalid YAML",
		},
		{
			name: "multiple documents",
			data: "capacity:\n" +
				"  max_runners: 2\n" +
				"---\n" +
				"capacity:\n" +
				"  max_runners: 4\n",
			wantErr: "configuration must contain exactly one YAML document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateYAML([]byte(tt.data))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NotContains(t, err.Error(), secret)
		})
	}
}
