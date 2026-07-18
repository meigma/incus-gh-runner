package incus

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/controller"
)

func TestIncusLifecycleFunctional(t *testing.T) {
	project := os.Getenv("INCUS_GH_RUNNER_TEST_PROJECT")
	image := os.Getenv("INCUS_GH_RUNNER_TEST_IMAGE")
	if project == "" || image == "" {
		t.Skip("set INCUS_GH_RUNNER_TEST_PROJECT and INCUS_GH_RUNNER_TEST_IMAGE to run")
	}
	require.NotEqual(t, "default", project, "functional lifecycle must use a disposable non-default project")

	testContext, cancelTest := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelTest()

	server, err := ConnectUnix(testContext, os.Getenv("INCUS_GH_RUNNER_TEST_SOCKET"), project)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	mailbox := controller.NewMailbox()
	probeSecret := "phase3-probe-" + uuid.NewString()
	diagnosticsObserved := make(chan Diagnostics, 1)
	backend, err := NewBackend(server, Options{
		Image:            image,
		Profiles:         splitProfiles(os.Getenv("INCUS_GH_RUNNER_TEST_PROFILES")),
		Owner:            "phase3-functional-" + uuid.NewString(),
		BootstrapTimeout: 5 * time.Minute,
		Logger:           logger,
		Payloads: PayloadSourceFunc(func(context.Context, string) (Payload, error) {
			return Payload{Version: 1, JITConfig: probeSecret}, nil
		}),
		Diagnostics: DiagnosticsSinkFunc(func(_ context.Context, observed Diagnostics) error {
			mailbox.Publish(controller.Demand{})
			select {
			case diagnosticsObserved <- observed:
			default:
			}
			return nil
		}),
	})
	require.NoError(t, err)
	require.NoError(t, backend.Preflight(testContext))

	ctrl, err := controller.New(controller.Options{
		Backend:           backend,
		Demand:            mailbox.Updates(),
		Logger:            logger,
		MinRunners:        0,
		MaxRunners:        1,
		Workers:           1,
		ReconcileInterval: time.Second,
		OperationTimeout:  5 * time.Minute,
		ShutdownTimeout:   30 * time.Second,
	})
	require.NoError(t, err)

	result := make(chan error, 1)
	go func() {
		result <- ctrl.Run(testContext)
	}()
	mailbox.Publish(controller.Demand{AssignedJobs: 1})

	var diagnostics Diagnostics
	select {
	case diagnostics = <-diagnosticsObserved:
	case resultErr := <-result:
		require.NoError(t, resultErr)
		t.Fatal("controller stopped before collecting diagnostics")
	case <-time.After(10 * time.Minute):
		t.Fatal("Incus lifecycle did not reach diagnostic collection and deletion")
	}

	require.Eventually(t, func() bool {
		runners, inventoryErr := backend.ListOwned(testContext)
		return inventoryErr == nil && len(runners) == 0
	}, 30*time.Second, 250*time.Millisecond, "functional lifecycle must return to zero owned instances")

	cancelTest()
	select {
	case resultErr := <-result:
		require.NoError(t, resultErr)
	case <-time.After(30 * time.Second):
		t.Fatal("controller did not stop after functional proof")
	}

	assert.Contains(t, string(diagnostics.Console), "incus-gh-runner-guest action=poweroff")
	assert.False(t, bytes.Contains(diagnostics.Console, []byte(probeSecret)), "probe payload must not leak to console")
}

// splitProfiles parses the optional comma-separated functional-test profile list.
func splitProfiles(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	profiles := make([]string, 0, len(parts))
	for _, part := range parts {
		if profile := strings.TrimSpace(part); profile != "" {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}
