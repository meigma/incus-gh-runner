package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
)

func TestVersionFlagPrintsBuildMetadata(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand(Options{
		Out: &stdout,
		Err: &stderr,
		Build: BuildInfo{
			Version: "0.1.0",
			Commit:  "abc1234",
			Date:    "2026-05-08T10:00:00Z",
		},
	})
	root.SetArgs([]string{"--version"})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, "incus-gh-runner 0.1.0 (abc1234) built 2026-05-08T10:00:00Z\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRootCommandLoadsFileEnvironmentAndFlagPrecedence(t *testing.T) {
	t.Setenv("INCUS_GH_RUNNER_CAPACITY_MAX_RUNNERS", "3")
	t.Setenv(config.EnvGitHubToken, "development-token")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`github:
  config_url: https://github.com/meigma/incus-gh-runner
  scale_set: incus-runners
  runner_group: default
incus:
  project: runner-test
  image: incus-gh-runner:test
  profiles: [default]
  owner: runner-test-owner
  bootstrap_timeout: 3m
  diagnostics_dir: /tmp/incus-gh-runner-diagnostics
capacity:
  min_runners: 1
  max_runners: 2
concurrency:
  incus_operations: 1
reconcile_interval: 2s
timeouts:
  incus_operation: 1m
  shutdown: 10s
`), 0o600))

	var received config.Config
	root := NewRootCommand(Options{
		Viper: viper.New(),
		Run: func(_ context.Context, cfg config.Config) error {
			received = cfg
			return nil
		},
	})
	root.SetArgs([]string{"--config", configPath, "--max-runners", "4"})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, 1, received.Capacity.MinRunners)
	assert.Equal(t, 4, received.Capacity.MaxRunners)
	assert.Equal(t, 1, received.Concurrency.IncusOperations)
	assert.Equal(t, 2*time.Second, received.ReconcileInterval)
	assert.Equal(t, time.Minute, received.Timeouts.IncusOperation)
	assert.Equal(t, 10*time.Second, received.Timeouts.Shutdown)
	assert.Equal(t, "https://github.com/meigma/incus-gh-runner", received.GitHub.ConfigURL)
	assert.Equal(t, "incus-runners", received.GitHub.ScaleSet)
	assert.Equal(t, "development-token", received.GitHub.Token)
	assert.Equal(t, "runner-test", received.Incus.Project)
	assert.Equal(t, []string{"default"}, received.Incus.Profiles)
	assert.Equal(t, 3*time.Minute, received.Incus.BootstrapTimeout)
}

func TestRootCommandAllowsMissingOptionalConfig(t *testing.T) {
	t.Parallel()

	var received config.Config
	root := NewRootCommand(Options{
		Viper:             viper.New(),
		DefaultConfigPath: filepath.Join(t.TempDir(), "missing.yaml"),
		Run: func(_ context.Context, cfg config.Config) error {
			received = cfg
			return nil
		},
	})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, config.Defaults(), received)
}

func TestRootCommandRejectsMissingExplicitConfig(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	root := NewRootCommand(Options{Viper: viper.New()})
	root.SetArgs([]string{"--config", missing})

	err := root.ExecuteContext(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read configuration")
	assert.Contains(t, err.Error(), missing)
}

func TestRootCommandRejectsInexactConfigurationBeforeRun(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`github:
  runner_gropu: default
`), 0o600))
	called := false
	root := NewRootCommand(Options{
		Viper: viper.New(),
		Run: func(context.Context, config.Config) error {
			called = true
			return nil
		},
	})
	root.SetArgs([]string{"--config", configPath})

	err := root.ExecuteContext(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown configuration field "github.runner_gropu"`)
	assert.False(t, called)
}

func TestRootCommandRedactsInvalidScalarBeforeRun(t *testing.T) {
	t.Parallel()

	const secret = "must-not-appear-in-the-error"
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`github:
  app:
    installation_id: !!int `+secret+`
`), 0o600))
	called := false
	root := NewRootCommand(Options{
		Viper: viper.New(),
		Run: func(context.Context, config.Config) error {
			called = true
			return nil
		},
	})
	root.SetArgs([]string{"--config", configPath})

	err := root.ExecuteContext(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `configuration field "github.app.installation_id" must be a valid integer`)
	assert.NotContains(t, err.Error(), secret)
	assert.False(t, called)
}

func TestRootCommandPassesExecutionContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := NewRootCommand(Options{
		Viper:             viper.New(),
		DefaultConfigPath: filepath.Join(t.TempDir(), "missing.yaml"),
		Run: func(ctx context.Context, _ config.Config) error {
			assert.ErrorIs(t, ctx.Err(), context.Canceled)
			return nil
		},
	})

	require.NoError(t, root.ExecuteContext(ctx))
}

// TestValidateCommandBypassesControllerInitialization proves the subcommand has an isolated startup path.
func TestValidateCommandBypassesControllerInitialization(t *testing.T) {
	t.Setenv(config.EnvGitHubTokenFile, filepath.Join(t.TempDir(), "missing-token"))
	invalidConfigPath := filepath.Join(t.TempDir(), "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidConfigPath, []byte("unknown: true\n"), 0o600))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	controllerCalled := false
	validationCalled := false
	root := NewRootCommand(Options{
		Out:               &stdout,
		Err:               &stderr,
		Viper:             viper.New(),
		DefaultConfigPath: invalidConfigPath,
		Run: func(context.Context, config.Config) error {
			controllerCalled = true
			return nil
		},
		Validate: func(_ context.Context, baselinePath string, socketPath string) (ValidationResult, error) {
			validationCalled = true
			assert.Equal(t, "baseline.json", baselinePath)
			assert.Equal(t, "/run/incus/unix.socket", socketPath)
			return ValidationResult{Notices: []string{"retain compensating control"}}, nil
		},
	})
	root.SetArgs([]string{"validate", "--socket", "/run/incus/unix.socket", "baseline.json"})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.True(t, validationCalled)
	assert.False(t, controllerCalled)
	assert.Equal(t, "Incus isolation baseline matches baseline.json\n", stdout.String())
	assert.Equal(t, "NOTICE: retain compensating control\n", stderr.String())
}

// TestValidateCommandUsesTheDocumentedDefaultSocket proves the stable validation flag default.
func TestValidateCommandUsesTheDocumentedDefaultSocket(t *testing.T) {
	var receivedSocket string
	root := NewRootCommand(Options{
		Validate: func(_ context.Context, _ string, socketPath string) (ValidationResult, error) {
			receivedSocket = socketPath
			return ValidationResult{}, nil
		},
	})
	root.SetArgs([]string{"validate", "baseline.json"})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, defaultValidationSocketPath, receivedSocket)
}

// TestValidateCommandRequiresOneBaseline proves the operand contract.
func TestValidateCommandRequiresOneBaseline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "missing baseline", args: []string{"validate"}},
		{name: "extra baseline", args: []string{"validate", "one.json", "two.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			called := false
			root := NewRootCommand(Options{
				Validate: func(context.Context, string, string) (ValidationResult, error) {
					called = true
					return ValidationResult{}, nil
				},
			})
			root.SetArgs(tt.args)

			require.Error(t, root.ExecuteContext(context.Background()))
			assert.False(t, called)
		})
	}
}

// TestValidateCommandPassesExecutionContext proves cancellation reaches the validation adapter.
func TestValidateCommandPassesExecutionContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := NewRootCommand(Options{
		Validate: func(ctx context.Context, _ string, _ string) (ValidationResult, error) {
			assert.ErrorIs(t, ctx.Err(), context.Canceled)
			return ValidationResult{}, nil
		},
	})
	root.SetArgs([]string{"validate", "baseline.json"})

	require.NoError(t, root.ExecuteContext(ctx))
}

// TestRootCommandRejectsUnexpectedOperands proves a misspelled subcommand cannot start the controller.
func TestRootCommandRejectsUnexpectedOperands(t *testing.T) {
	t.Parallel()

	called := false
	root := NewRootCommand(Options{
		Run: func(context.Context, config.Config) error {
			called = true
			return nil
		},
	})
	root.SetArgs([]string{"validte"})

	require.Error(t, root.ExecuteContext(context.Background()))
	assert.False(t, called)
}
