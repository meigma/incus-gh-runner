package config_test

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
)

func TestLoadUsesDefaultsAndExplicitEnvironment(t *testing.T) {
	t.Setenv("INCUS_GH_RUNNER_CAPACITY_MIN_RUNNERS", "2")
	t.Setenv("INCUS_GH_RUNNER_CAPACITY_MAX_RUNNERS", "4")
	t.Setenv("INCUS_GH_RUNNER_TIMEOUTS_SHUTDOWN", "45s")
	t.Setenv(config.EnvGitHubToken, "development-token")
	vp := viper.New()
	require.NoError(t, config.ConfigureViper(vp))

	cfg, err := config.Load(vp)

	require.NoError(t, err)
	assert.Equal(t, 2, cfg.Capacity.MinRunners)
	assert.Equal(t, 2, cfg.Concurrency.IncusOperations)
	assert.Equal(t, time.Second, cfg.ReconcileInterval)
	assert.Equal(t, 45*time.Second, cfg.Timeouts.Shutdown)
	assert.Equal(t, "development-token", cfg.GitHub.Token)
	assert.Equal(t, "default", cfg.GitHub.RunnerGroup)
	assert.Equal(t, 5*time.Minute, cfg.Incus.BootstrapTimeout)
}

func TestLoadDoesNotDecodeGitHubTokenFromConfiguration(t *testing.T) {
	t.Setenv(config.EnvGitHubToken, "")
	vp := viper.New()
	require.NoError(t, config.ConfigureViper(vp))
	vp.Set("github.token", "file-token-must-be-ignored")

	cfg, err := config.Load(vp)

	require.NoError(t, err)
	assert.Empty(t, cfg.GitHub.Token)
}

func TestValidateRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	valid := config.Config{
		Capacity: config.Capacity{MinRunners: 0, MaxRunners: 1},
		Concurrency: config.Concurrency{
			IncusOperations: 1,
		},
		ReconcileInterval: time.Second,
		Timeouts: config.Timeouts{
			IncusOperation: time.Minute,
			Shutdown:       time.Second,
		},
	}
	tests := []struct {
		name   string
		mutate func(*config.Config)
		want   string
	}{
		{
			name: "negative minimum",
			mutate: func(cfg *config.Config) {
				cfg.Capacity.MinRunners = -1
			},
			want: "capacity.min_runners must not be negative",
		},
		{
			name: "maximum below minimum",
			mutate: func(cfg *config.Config) {
				cfg.Capacity.MinRunners = 2
			},
			want: "capacity.max_runners must be at least capacity.min_runners",
		},
		{
			name: "no workers",
			mutate: func(cfg *config.Config) {
				cfg.Concurrency.IncusOperations = 0
			},
			want: "concurrency.incus_operations must be positive",
		},
		{
			name: "no reconciliation interval",
			mutate: func(cfg *config.Config) {
				cfg.ReconcileInterval = 0
			},
			want: "reconcile_interval must be positive",
		},
		{
			name: "no operation timeout",
			mutate: func(cfg *config.Config) {
				cfg.Timeouts.IncusOperation = 0
			},
			want: "timeouts.incus_operation must be positive",
		},
		{
			name: "no shutdown timeout",
			mutate: func(cfg *config.Config) {
				cfg.Timeouts.Shutdown = 0
			},
			want: "timeouts.shutdown must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			tt.mutate(&cfg)

			assert.EqualError(t, cfg.Validate(), tt.want)
		})
	}
}

func TestValidateRuntimeRequiresCompleteAdapterConfiguration(t *testing.T) {
	t.Parallel()

	valid := config.Config{
		GitHub: config.GitHub{
			ConfigURL:   "https://github.com/meigma/incus-gh-runner",
			ScaleSet:    "incus-phase4",
			RunnerGroup: "default",
			Token:       "development-token",
		},
		Incus: config.Incus{
			Project:          "runner-test",
			Image:            "incus-gh-runner:test",
			Profiles:         []string{"default"},
			Owner:            "phase4-test",
			BootstrapTimeout: time.Minute,
		},
		Capacity:          config.Capacity{MinRunners: 0, MaxRunners: 1},
		Concurrency:       config.Concurrency{IncusOperations: 1},
		ReconcileInterval: time.Second,
		Timeouts: config.Timeouts{
			IncusOperation: time.Minute,
			Shutdown:       time.Second,
		},
	}
	tests := []struct {
		name   string
		mutate func(*config.Config)
		want   string
	}{
		{
			name: "missing GitHub URL",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = ""
			},
			want: "github.config_url must be an absolute HTTP URL",
		},
		{
			name: "missing scale set",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ScaleSet = ""
			},
			want: "github.scale_set is required",
		},
		{
			name: "missing credentials",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.Token = ""
			},
			want: "github credentials are required",
		},
		{
			name: "mixed credential types",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.App = config.GitHubApp{ClientID: "1", InstallationID: 2, PrivateKeyFile: "/key.pem"}
			},
			want: "configure either github.app or INCUS_GH_RUNNER_GITHUB_TOKEN, not both",
		},
		{
			name: "incomplete GitHub App",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.Token = ""
				cfg.GitHub.App = config.GitHubApp{ClientID: "1"}
			},
			want: "github.app.installation_id must be positive",
		},
		{
			name: "missing Incus project",
			mutate: func(cfg *config.Config) {
				cfg.Incus.Project = ""
			},
			want: "incus.project is required",
		},
		{
			name: "empty Incus profile",
			mutate: func(cfg *config.Config) {
				cfg.Incus.Profiles = []string{"default", ""}
			},
			want: "incus.profiles must not contain empty names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			tt.mutate(&cfg)

			assert.EqualError(t, cfg.ValidateRuntime(), tt.want)
		})
	}

	assert.NoError(t, valid.ValidateRuntime())
}
