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
