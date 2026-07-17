package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRootCommandPrintsConfiguredMessage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{
		Out:   &stdout,
		Viper: viper.New(),
	})
	root.SetArgs([]string{"--message", "hello from cobra"})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, "hello from cobra\n", stdout.String())
}

func TestRootCommandReadsMessageFromEnvironment(t *testing.T) {
	t.Setenv("INCUS_GH_RUNNER_MESSAGE", "hello from viper")

	var stdout bytes.Buffer
	root := NewRootCommand(Options{
		Out:   &stdout,
		Viper: viper.New(),
	})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, "hello from viper\n", stdout.String())
}
