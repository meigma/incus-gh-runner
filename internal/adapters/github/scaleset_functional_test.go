package github

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaleSetSessionFunctional(t *testing.T) {
	configURL := os.Getenv("INCUS_GH_RUNNER_TEST_GITHUB_CONFIG_URL")
	scaleSetName := os.Getenv("INCUS_GH_RUNNER_TEST_GITHUB_SCALE_SET")
	token := os.Getenv("INCUS_GH_RUNNER_GITHUB_TOKEN")
	if configURL == "" || scaleSetName == "" || token == "" {
		t.Skip("set GitHub config URL, scale set, and token environment variables to run")
	}

	testContext, cancelTest := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelTest()
	client, err := NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{
		GitHubConfigURL:     configURL,
		PersonalAccessToken: token,
		SystemInfo: scaleset.SystemInfo{
			System:    "incus-gh-runner",
			Version:   "functional-test",
			CommitSHA: "none",
			Subsystem: "github-preflight",
		},
	})
	require.NoError(t, err)
	resolved, err := ResolveScaleSet(testContext, client, ScaleSetOptions{
		Name:        scaleSetName,
		RunnerGroup: scaleset.DefaultRunnerGroup,
	})
	require.NoError(t, err)

	session, err := client.MessageSessionClient(testContext, resolved.ID(), "functional-preflight")
	require.NoError(t, err)
	assert.NotNil(t, session.Session().Statistics)
	require.NoError(t, session.Close(testContext))
}
