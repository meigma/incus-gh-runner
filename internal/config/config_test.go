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
	t.Setenv("INCUS_GH_RUNNER_GITHUB_MESSAGE_POLL_TIMEOUT", "7s")
	t.Setenv(config.EnvGitHubToken, "development-token")
	t.Setenv("INCUS_GH_RUNNER_JOB_PROOF_HOST_ID", " builder-host-01 ")
	t.Setenv(config.EnvJobProofSigningKeyFile, " /run/credentials/incus-gh-runner.service/machine-provenance-key ")
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
	assert.Equal(t, 7*time.Second, cfg.GitHub.MessagePollTimeout)
	assert.Equal(t, "development-token", cfg.GitHub.Token)
	assert.Equal(t, "default", cfg.GitHub.RunnerGroup)
	assert.Equal(t, 5*time.Minute, cfg.Incus.BootstrapTimeout)
	assert.Equal(t, "builder-host-01", cfg.JobProof.HostID)
	assert.Equal(t, "/run/credentials/incus-gh-runner.service/machine-provenance-key", cfg.JobProof.SigningKeyFile)
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
			name: "negative GitHub message poll timeout",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.MessagePollTimeout = -time.Second
			},
			want: "github.message_poll_timeout must not be negative",
		},
		{
			name: "GitHub message poll timeout below safe minimum",
			mutate: func(cfg *config.Config) {
				cfg.GitHub.MessagePollTimeout = 4999 * time.Millisecond
			},
			want: "github.message_poll_timeout must be at least 5s when set",
		},
		{
			name: "job proof host without key",
			mutate: func(cfg *config.Config) {
				cfg.JobProof.HostID = "builder-host-01"
			},
			want: "job_proof.host_id and job_proof.signing_key_file must be configured together",
		},
		{
			name: "job proof key without host",
			mutate: func(cfg *config.Config) {
				cfg.JobProof.SigningKeyFile = "/run/credentials/machine-provenance-key"
			},
			want: "job_proof.host_id and job_proof.signing_key_file must be configured together",
		},
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
			IncusConnection: config.IncusConnection{
				Socket: "/var/lib/incus/unix.socket",
			},
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
		{
			name: "missing Incus transport",
			mutate: func(cfg *config.Config) {
				cfg.Incus.Socket = ""
			},
			want: "configure exactly one of incus.socket or incus.url",
		},
		{
			name: "both Incus transports",
			mutate: func(cfg *config.Config) {
				cfg.Incus.URL = "https://incus.example:8443"
			},
			want: "configure exactly one of incus.socket or incus.url",
		},
		{
			name: "HTTPS without server certificate",
			mutate: func(cfg *config.Config) {
				cfg.Incus.Socket = ""
				cfg.Incus.URL = "https://incus.example:8443"
				cfg.Incus.ClientCertFile = "/client.crt"
				cfg.Incus.ClientKeyFile = "/client.key"
			},
			want: "incus.server_cert_file is required",
		},
		{
			name: "TLS files with Unix socket",
			mutate: func(cfg *config.Config) {
				cfg.Incus.ClientCertFile = "/client.crt"
			},
			want: "incus.client_cert_file is only valid with incus.url",
		},
		{
			name: "plaintext Incus URL",
			mutate: func(cfg *config.Config) {
				cfg.Incus.Socket = ""
				cfg.Incus.URL = "http://incus.example:8443"
				cfg.Incus.ClientCertFile = "/client.crt"
				cfg.Incus.ClientKeyFile = "/client.key"
				cfg.Incus.ServerCertFile = "/server.crt"
			},
			want: "incus.url must be an absolute HTTPS URL",
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
	httpsIncus := valid
	httpsIncus.Incus.Socket = ""
	httpsIncus.Incus.URL = "https://incus.example:8443"
	httpsIncus.Incus.ClientCertFile = "/etc/incus-gh-runner/client.crt"
	httpsIncus.Incus.ClientKeyFile = "/etc/incus-gh-runner/client.key"
	httpsIncus.Incus.ServerCertFile = "/etc/incus-gh-runner/server.crt"
	assert.NoError(t, httpsIncus.ValidateRuntime())
}

func TestIncusConnectionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connection config.IncusConnection
		wantErr    string
	}{
		{
			name:       "unix socket",
			connection: config.IncusConnection{Socket: "/var/lib/incus/unix.socket"},
		},
		{
			name: "HTTPS with pinned credentials",
			connection: config.IncusConnection{
				URL:            "https://incus.example:8443",
				ClientCertFile: "/client.crt",
				ClientKeyFile:  "/client.key",
				ServerCertFile: "/server.crt",
			},
		},
		{
			name:    "neither transport",
			wantErr: "configure exactly one of incus.socket or incus.url",
		},
		{
			name: "both transports",
			connection: config.IncusConnection{
				Socket: "/var/lib/incus/unix.socket",
				URL:    "https://incus.example:8443",
			},
			wantErr: "configure exactly one of incus.socket or incus.url",
		},
		{
			name: "HTTPS without client certificate",
			connection: config.IncusConnection{
				URL:            "https://incus.example:8443",
				ClientKeyFile:  "/client.key",
				ServerCertFile: "/server.crt",
			},
			wantErr: "incus.client_cert_file is required",
		},
		{
			name: "HTTPS without client key",
			connection: config.IncusConnection{
				URL:            "https://incus.example:8443",
				ClientCertFile: "/client.crt",
				ServerCertFile: "/server.crt",
			},
			wantErr: "incus.client_key_file is required",
		},
		{
			name: "HTTPS without server certificate",
			connection: config.IncusConnection{
				URL:            "https://incus.example:8443",
				ClientCertFile: "/client.crt",
				ClientKeyFile:  "/client.key",
			},
			wantErr: "incus.server_cert_file is required",
		},
		{
			name: "client key with unix socket",
			connection: config.IncusConnection{
				Socket:        "/var/lib/incus/unix.socket",
				ClientKeyFile: "/client.key",
			},
			wantErr: "incus.client_key_file is only valid with incus.url",
		},
		{
			name: "server certificate with unix socket",
			connection: config.IncusConnection{
				Socket:         "/var/lib/incus/unix.socket",
				ServerCertFile: "/server.crt",
			},
			wantErr: "incus.server_cert_file is only valid with incus.url",
		},
		{
			name: "plaintext URL",
			connection: config.IncusConnection{
				URL:            "http://incus.example:8443",
				ClientCertFile: "/client.crt",
				ClientKeyFile:  "/client.key",
				ServerCertFile: "/server.crt",
			},
			wantErr: "incus.url must be an absolute HTTPS URL",
		},
		{
			name: "URL with userinfo",
			connection: config.IncusConnection{
				URL:            "https://user@incus.example:8443",
				ClientCertFile: "/client.crt",
				ClientKeyFile:  "/client.key",
				ServerCertFile: "/server.crt",
			},
			wantErr: "incus.url must be an absolute HTTPS URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.connection.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestIncusConnectionRejectsNonRootHTTPSURL(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{
		"https://incus.example:8443/api",
		"https://incus.example:8443?project=other",
		"https://incus.example:8443?",
		"https://incus.example:8443/#fragment",
	} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			connection := config.IncusConnection{
				URL:            endpoint,
				ClientCertFile: "/client.crt",
				ClientKeyFile:  "/client.key",
				ServerCertFile: "/server.crt",
			}
			require.Error(t, connection.Validate())
		})
	}
}

func TestLoadBindsIncusHTTPSConnection(t *testing.T) {
	t.Setenv("INCUS_GH_RUNNER_INCUS_URL", "https://incus.example:8443")
	t.Setenv("INCUS_GH_RUNNER_INCUS_CLIENT_CERT_FILE", "/run/credentials/incus-gh-runner.service/incus-client.crt")
	t.Setenv("INCUS_GH_RUNNER_INCUS_CLIENT_KEY_FILE", "/run/credentials/incus-gh-runner.service/incus-client-key")
	t.Setenv("INCUS_GH_RUNNER_INCUS_SERVER_CERT_FILE", "/etc/incus-gh-runner/server.crt")
	vp := viper.New()
	require.NoError(t, config.ConfigureViper(vp))

	cfg, err := config.Load(vp)

	require.NoError(t, err)
	assert.Equal(t, "https://incus.example:8443", cfg.Incus.URL)
	assert.Equal(t, "/run/credentials/incus-gh-runner.service/incus-client.crt", cfg.Incus.ClientCertFile)
	assert.Equal(t, "/run/credentials/incus-gh-runner.service/incus-client-key", cfg.Incus.ClientKeyFile)
	assert.Equal(t, "/etc/incus-gh-runner/server.crt", cfg.Incus.ServerCertFile)
}
