package incus

import (
	"context"
	"os"
	"testing"

	incusclient "github.com/lxc/incus/v7/client"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
)

// connectFunctionalTestServer opens the Incus client used by live lifecycle tests.
//
// HTTPS credentials from INCUS_GH_RUNNER_TEST_URL and the matching cert files
// take precedence. A nonempty INCUS_GH_RUNNER_TEST_SOCKET uses Connect. An
// unset socket keeps the historical default Unix-socket fallback.
func connectFunctionalTestServer(ctx context.Context, t *testing.T, project string) incusclient.InstanceServer {
	t.Helper()
	if remoteURL := os.Getenv("INCUS_GH_RUNNER_TEST_URL"); remoteURL != "" {
		server, err := Connect(ctx, config.IncusConnection{
			URL:            remoteURL,
			ClientCertFile: os.Getenv("INCUS_GH_RUNNER_TEST_CLIENT_CERT_FILE"),
			ClientKeyFile:  os.Getenv("INCUS_GH_RUNNER_TEST_CLIENT_KEY_FILE"),
			ServerCertFile: os.Getenv("INCUS_GH_RUNNER_TEST_SERVER_CERT_FILE"),
		}, project)
		require.NoError(t, err)
		return server
	}
	socket := os.Getenv("INCUS_GH_RUNNER_TEST_SOCKET")
	if socket != "" {
		server, err := Connect(ctx, config.IncusConnection{Socket: socket}, project)
		require.NoError(t, err)
		return server
	}
	server, err := ConnectUnix(ctx, "", project)
	require.NoError(t, err)
	return server
}
