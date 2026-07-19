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
	t.Setenv("INCUS_GH_RUNNER_RETRY_MAXIMUM", "20s")
	t.Setenv(config.EnvGitHubToken, "development-token")
	vp := viper.New()
	require.NoError(t, config.ConfigureViper(vp))

	cfg, err := config.Load(vp)

	require.NoError(t, err)
	assert.Equal(t, 2, cfg.Capacity.MinRunners)
	assert.Equal(t, 2, cfg.Concurrency.IncusOperations)
	assert.Equal(t, time.Second, cfg.ReconcileInterval)
	assert.Equal(t, 45*time.Second, cfg.Timeouts.Shutdown)
	assert.Equal(t, time.Second, cfg.Retry.Initial)
	assert.Equal(t, 20*time.Second, cfg.Retry.Maximum)
	assert.Equal(t, "development-token", cfg.GitHub.Token)
	assert.Equal(t, "default", cfg.GitHub.RunnerGroup)
	assert.Equal(t, 5*time.Minute, cfg.Incus.BootstrapTimeout)
}

func TestLoadRejectsGitHubTokenFromConfigurationWithoutLeakingIt(t *testing.T) {
	t.Setenv(config.EnvGitHubToken, "")
	vp := viper.New()
	require.NoError(t, config.ConfigureViper(vp))
	const secret = "file-token-must-not-appear"
	vp.Set("github.token", secret)

	_, err := config.Load(vp)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
	assert.NotContains(t, err.Error(), secret)
}

func TestLoadBindsPersonalAccessTokenFile(t *testing.T) {
	t.Setenv(config.EnvGitHubTokenFile, " /run/credentials/incus-gh-runner/github-token ")
	vp := viper.New()
	require.NoError(t, config.ConfigureViper(vp))

	cfg, err := config.Load(vp)

	require.NoError(t, err)
	assert.Equal(t, "/run/credentials/incus-gh-runner/github-token", cfg.GitHub.TokenFile)
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
		Retry: config.Retry{Initial: time.Second, Maximum: time.Minute},
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
		{
			name: "no initial retry delay",
			mutate: func(cfg *config.Config) {
				cfg.Retry.Initial = 0
			},
			want: "retry.initial must be positive",
		},
		{
			name: "retry maximum below initial delay",
			mutate: func(cfg *config.Config) {
				cfg.Retry.Maximum = time.Millisecond
			},
			want: "retry.maximum must be at least retry.initial",
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
			ScaleSet:    "incus-runners",
			RunnerGroup: "default",
			Token:       "development-token",
		},
		Incus: config.Incus{
			Project:          "runner-test",
			Image:            "incus-gh-runner:test",
			Profiles:         []string{"default"},
			Owner:            "runner-test-owner",
			BootstrapTimeout: time.Minute,
		},
		Capacity:          config.Capacity{MinRunners: 0, MaxRunners: 1},
		Concurrency:       config.Concurrency{IncusOperations: 1},
		ReconcileInterval: time.Second,
		Timeouts: config.Timeouts{
			IncusOperation: time.Minute,
			Shutdown:       time.Second,
		},
		Retry: config.Retry{Initial: time.Second, Maximum: time.Minute},
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
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "plaintext GitHub URL",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "http://github.com/meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "plaintext loopback URL",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "http://localhost/meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with userinfo",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://token@github.com/meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with query",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma/incus-gh-runner?scope=other"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with fragment",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma/incus-gh-runner#other"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with encoded path separator",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma%2Fincus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with extra path segment",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma/incus-gh-runner/actions"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL without a scope path",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with dot path segment",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma/../incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with surrounding whitespace",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = " https://github.com/meigma/incus-gh-runner "
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "hosted GitHub URL with explicit port",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com:443/meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "www GitHub URL with explicit port",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://www.github.com:443/meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub Enterprise Cloud URL with explicit port",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://meigma.ghe.com:443/meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GitHub URL with trailing DNS dot",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com./meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "GHES URL with out-of-range port",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.example.com:65536/meigma/incus-gh-runner"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "organization using default runner group",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma"
			},
			want: "github.runner_group must name a non-default runner group for organization scope",
		},
		{
			name: "organization using padded mixed-case default runner group",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma"
				cfg.GitHub.RunnerGroup = " Default "
			},
			want: "github.runner_group must name a non-default runner group for organization scope",
		},
		{
			name: "repository using a custom runner group",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.RunnerGroup = "Build Runners"
			},
			want: "github.runner_group must be default for repository scope",
		},
		{
			name: "organization runner group with injected query field",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma"
				cfg.GitHub.RunnerGroup = "default&x=y"
			},
			want: "github.runner_group contains characters that are unsafe in GitHub API queries",
		},
		{
			name: "organization runner group with fragment",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma"
				cfg.GitHub.RunnerGroup = "default#suffix"
			},
			want: "github.runner_group contains characters that are unsafe in GitHub API queries",
		},
		{
			name: "organization runner group with encoded default name",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma"
				cfg.GitHub.RunnerGroup = "%64efault"
			},
			want: "github.runner_group contains characters that are unsafe in GitHub API queries",
		},
		{
			name: "organization runner group with plus",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma"
				cfg.GitHub.RunnerGroup = "Build+Runners"
			},
			want: "github.runner_group contains characters that are unsafe in GitHub API queries",
		},
		{
			name: "organization runner group with semicolon",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/meigma"
				cfg.GitHub.RunnerGroup = "Build;Runners"
			},
			want: "github.runner_group contains characters that are unsafe in GitHub API queries",
		},
		{
			name: "enterprise URL is unsupported",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ConfigURL = "https://github.com/enterprises/meigma"
			},
			want: "github.config_url must be an absolute HTTPS GitHub organization or repository URL",
		},
		{
			name: "missing scale set",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ScaleSet = ""
			},
			want: "github.scale_set is required",
		},
		{
			name: "scale set with injected query field",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ScaleSet = "incus-runners&runnerGroupId=1"
			},
			want: "github.scale_set contains characters that are unsafe in GitHub API queries",
		},
		{
			name: "scale set with fragment",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.ScaleSet = "incus-runners#other"
			},
			want: "github.scale_set contains characters that are unsafe in GitHub API queries",
		},
		{
			name: "missing credentials",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.Token = ""
			},
			want: "github credentials are required",
		},
		{
			name: "mixed PAT sources",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.TokenFile = "/run/credentials/github-token"
			},
			want: "configure either github.token_file or INCUS_GH_RUNNER_GITHUB_TOKEN, not both",
		},
		{
			name: "mixed credential types",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.App = config.GitHubApp{ClientID: "1", InstallationID: 2, PrivateKeyFile: "/key.pem"}
			},
			want: "configure either github.app or a personal access token, not both",
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
	trailingSlashRepository := valid
	trailingSlashRepository.GitHub.ConfigURL = "https://github.com/meigma/incus-gh-runner/"
	assert.NoError(t, trailingSlashRepository.ValidateRuntime())
	ghesRepository := valid
	ghesRepository.GitHub.ConfigURL = "https://github.example.com/meigma/incus-gh-runner"
	assert.NoError(t, ghesRepository.ValidateRuntime())
	ghesRepositoryWithPort := valid
	ghesRepositoryWithPort.GitHub.ConfigURL = "https://github.example.com:8443/meigma/incus-gh-runner"
	assert.NoError(t, ghesRepositoryWithPort.ValidateRuntime())
	organization := valid
	organization.GitHub.ConfigURL = "https://github.com/meigma"
	organization.GitHub.RunnerGroup = "incus-gh-runner-prod"
	assert.NoError(t, organization.ValidateRuntime())
	organizationWithSpacedGroup := organization
	organizationWithSpacedGroup.GitHub.RunnerGroup = "Build Runners"
	assert.NoError(t, organizationWithSpacedGroup.ValidateRuntime())
	ghesOrganization := organization
	ghesOrganization.GitHub.ConfigURL = "https://github.example.com/meigma"
	assert.NoError(t, ghesOrganization.ValidateRuntime())
	fileCredentials := valid
	fileCredentials.GitHub.Token = ""
	fileCredentials.GitHub.TokenFile = "/run/credentials/github-token"
	assert.NoError(t, fileCredentials.ValidateRuntime())
}
