package incus

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/controller"
)

// fileWrite records one successful guest file transfer.
type fileWrite struct {
	name    string
	path    string
	content []byte
	mode    int
}

// fakeClient provides an in-memory Incus lifecycle for adapter behavior tests.
type fakeClient struct {
	images             map[string]bool
	profiles           map[string]bool
	instances          map[string]api.Instance
	statusFiles        map[string][]byte
	statusErrors       map[string]error
	statusRead         func(context.Context, string, string) ([]byte, error)
	consoleLogs        map[string][]byte
	createRequest      api.InstancesPost
	started            []string
	stopped            []string
	deleted            []string
	fileWrites         []fileWrite
	fileAttempts       int
	agentFailures      int
	consoleError       error
	deleteError        error
	createInstanceErr  error
	startInstanceError error
	stopInstanceError  error
}

// newFakeClient creates an empty in-memory Incus client.
func newFakeClient() *fakeClient {
	return &fakeClient{
		images:       make(map[string]bool),
		profiles:     make(map[string]bool),
		instances:    make(map[string]api.Instance),
		statusFiles:  make(map[string][]byte),
		statusErrors: make(map[string]error),
		consoleLogs:  make(map[string][]byte),
		fileWrites:   make([]fileWrite, 0),
		started:      make([]string, 0),
		stopped:      make([]string, 0),
		deleted:      make([]string, 0),
		fileAttempts: 0,
	}
}

// GetImage resolves an image known to the fake.
func (f *fakeClient) GetImage(_ context.Context, name string) error {
	if !f.images[name] {
		return errNotFound
	}
	return nil
}

// GetProfile resolves a profile known to the fake.
func (f *fakeClient) GetProfile(_ context.Context, name string) error {
	if !f.profiles[name] {
		return errNotFound
	}
	return nil
}

// GetInstances returns a stable snapshot of fake instances.
func (f *fakeClient) GetInstances(context.Context) ([]api.Instance, error) {
	instances := make([]api.Instance, 0, len(f.instances))
	for _, instance := range f.instances {
		instances = append(instances, instance)
	}
	return instances, nil
}

// GetInstance returns one fake instance.
func (f *fakeClient) GetInstance(_ context.Context, name string) (*api.Instance, error) {
	instance, ok := f.instances[name]
	if !ok {
		return nil, errNotFound
	}
	return &instance, nil
}

// CreateInstance records and materializes a fake instance request.
func (f *fakeClient) CreateInstance(_ context.Context, request api.InstancesPost) error {
	if f.createInstanceErr != nil {
		return f.createInstanceErr
	}
	f.createRequest = request
	f.instances[request.Name] = api.Instance{
		Name:        request.Name,
		Status:      "Stopped",
		InstancePut: request.InstancePut,
	}
	return nil
}

// StartInstance records the fake start transition.
func (f *fakeClient) StartInstance(_ context.Context, name string) error {
	if f.startInstanceError != nil {
		return f.startInstanceError
	}
	instance := f.instances[name]
	instance.Status = "Running"
	f.instances[name] = instance
	f.started = append(f.started, name)
	return nil
}

// StopInstance records the fake stop transition.
func (f *fakeClient) StopInstance(_ context.Context, name string) error {
	if f.stopInstanceError != nil {
		return f.stopInstanceError
	}
	instance, ok := f.instances[name]
	if !ok {
		return errNotFound
	}
	instance.Status = "Stopped"
	f.instances[name] = instance
	f.stopped = append(f.stopped, name)
	return nil
}

// CreateInstanceFile records successful transfers after configured agent failures.
func (f *fakeClient) CreateInstanceFile(
	_ context.Context,
	name string,
	path string,
	content []byte,
	mode int,
) error {
	f.fileAttempts++
	if f.agentFailures > 0 {
		f.agentFailures--
		return errors.New("agent unavailable")
	}
	f.fileWrites = append(f.fileWrites, fileWrite{
		name:    name,
		path:    path,
		content: append([]byte(nil), content...),
		mode:    mode,
	})
	return nil
}

// GetInstanceFile returns a configured guest status file.
func (f *fakeClient) GetInstanceFile(ctx context.Context, name string, path string) ([]byte, error) {
	if f.statusRead != nil {
		return f.statusRead(ctx, name, path)
	}
	if err := f.statusErrors[name]; err != nil {
		return nil, err
	}
	status, ok := f.statusFiles[name]
	if !ok {
		return nil, errInstanceFileNotFound
	}
	return append([]byte(nil), status...), nil
}

// GetInstanceConsoleLog returns configured terminal diagnostics.
func (f *fakeClient) GetInstanceConsoleLog(_ context.Context, name string) ([]byte, error) {
	if f.consoleError != nil {
		return nil, f.consoleError
	}
	return append([]byte(nil), f.consoleLogs[name]...), nil
}

// DeleteInstance removes a fake instance.
func (f *fakeClient) DeleteInstance(_ context.Context, name string) error {
	if f.deleteError != nil {
		return f.deleteError
	}
	if _, ok := f.instances[name]; !ok {
		return errNotFound
	}
	delete(f.instances, name)
	f.deleted = append(f.deleted, name)
	return nil
}

func TestBackendPreflightChecksConfiguredReferences(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.images["runner-image"] = true
	client.profiles["runner"] = true
	backend := newTestBackend(t, client, Options{Profiles: []string{"runner"}})

	require.NoError(t, backend.Preflight(context.Background()))

	delete(client.profiles, "runner")
	err := backend.Preflight(context.Background())
	require.ErrorContains(t, err, "resolve runner profile")
}

func TestBackendListOwnedMapsLifecycleAndFiltersOwnership(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	client := newFakeClient()
	client.instances = map[string]api.Instance{
		"unowned": {
			Name:   "unowned",
			Status: "Stopped",
			InstancePut: api.InstancePut{
				Config: api.ConfigMap{ownershipKey: "someone-else"},
			},
		},
		"stopped":  ownedInstance("stopped", "Stopped", now),
		"starting": ownedInstance("starting", "Running", now),
		"missing":  ownedInstance("missing", "Running", now),
		"working":  ownedInstance("working", "Running", now),
		"expired":  ownedInstance("expired", "Running", now.Add(-2*time.Minute)),
	}
	client.statusFiles["starting"] = []byte("{\"version\":1,\"state\":\"starting\"}")
	client.statusFiles["working"] = []byte("{\"version\":1,\"state\":\"running\"}")
	backend := newTestBackend(t, client, Options{
		Now:              func() time.Time { return now },
		BootstrapTimeout: time.Minute,
	})

	runners, err := backend.ListOwned(context.Background())

	require.NoError(t, err)
	assert.ElementsMatch(t, []controller.Runner{
		{ID: "stopped", State: controller.RunnerTerminal},
		{ID: "starting", State: controller.RunnerProvisioning},
		{ID: "missing", State: controller.RunnerProvisioning},
		{ID: "working", State: controller.RunnerReady},
		{ID: "expired", State: controller.RunnerTerminal},
	}, runners)
}

func TestBackendListOwnedFailsClosedOnGuestStatusUncertainty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    []byte
		statusErr error
		wantError string
	}{
		{
			name:      "instance disappeared",
			statusErr: errNotFound,
			wantError: "incus resource not found",
		},
		{
			name:      "timeout",
			statusErr: context.DeadlineExceeded,
			wantError: "context deadline exceeded",
		},
		{
			name:      "transport failure",
			statusErr: errors.New("connection reset"),
			wantError: "connection reset",
		},
		{
			name:      "permission failure",
			statusErr: errors.New("permission denied"),
			wantError: "permission denied",
		},
		{
			name:      "malformed JSON",
			status:    []byte(`{"version":`),
			wantError: "decode guest status",
		},
		{
			name:      "unsupported version",
			status:    []byte(`{"version":2,"state":"running"}`),
			wantError: "unsupported guest status version 2",
		},
		{
			name:      "unknown state",
			status:    []byte(`{"version":1,"state":"mystery"}`),
			wantError: `unknown guest status state "mystery"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
			client := newFakeClient()
			client.instances["runner"] = ownedInstance("runner", "Running", now)
			client.statusFiles["runner"] = tt.status
			client.statusErrors["runner"] = tt.statusErr
			backend := newTestBackend(t, client, Options{Now: func() time.Time { return now }})

			runners, err := backend.ListOwned(context.Background())

			require.ErrorContains(t, err, tt.wantError)
			assert.Nil(t, runners, "uncertain inventory must not expose a partial snapshot")
		})
	}
}

func TestBackendListOwnedBudgetsEachGuestStatusRead(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	client := newFakeClient()
	client.instances["a-slow"] = ownedInstance("a-slow", "Running", now.Add(-2*time.Minute))
	client.instances["b-active"] = ownedInstance("b-active", "Running", now.Add(-2*time.Minute))
	activeRead := make(chan struct{}, 1)
	client.statusRead = func(ctx context.Context, name string, _ string) ([]byte, error) {
		if name == "a-slow" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		activeRead <- struct{}{}
		return []byte(`{"version":1,"state":"running"}`), nil
	}
	backend := newTestBackend(t, client, Options{
		Now:               func() time.Time { return now },
		BootstrapTimeout:  time.Minute,
		StatusReadTimeout: 20 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	runners, err := backend.ListOwned(ctx)

	require.ErrorContains(t, err, "a-slow")
	assert.Nil(t, runners, "one uncertain runner must invalidate the complete snapshot")
	require.NoError(t, ctx.Err(), "the slow runner must not consume the parent inventory deadline")
	select {
	case <-activeRead:
	default:
		t.Fatal("expected the later active runner to receive its own status-read budget")
	}
}

func TestBackendCreateOwnsStartsAndCommitsPayload(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	client := newFakeClient()
	client.agentFailures = 2
	backend := newTestBackend(t, client, Options{
		Profiles:          []string{"default", "runner"},
		Now:               func() time.Time { return now },
		NewID:             func() string { return "runner-id" },
		AgentPollInterval: time.Millisecond,
		Payloads: PayloadSourceFunc(func(_ context.Context, runnerID string) (Payload, error) {
			assert.Equal(t, "incus-gh-runner-runner-id", runnerID)
			return Payload{Version: 1, JITConfig: "secret-jit-config"}, nil
		}),
	})

	runner, err := backend.Create(context.Background())

	require.NoError(t, err)
	assert.Equal(t, controller.Runner{
		ID:    "incus-gh-runner-runner-id",
		State: controller.RunnerProvisioning,
	}, runner)
	assert.Equal(t, api.InstanceTypeVM, client.createRequest.Type)
	assert.Equal(t, "runner-image", client.createRequest.Source.Alias)
	assert.Equal(t, []string{"default", "runner"}, client.createRequest.Profiles)
	assert.Equal(t, "test-owner", client.createRequest.Config[ownershipKey])
	assert.Equal(t, "runner-id", client.createRequest.Config[correlationKey])
	assert.Equal(t, now.Format(time.RFC3339Nano), client.createRequest.Config[createdAtKey])
	assert.Equal(t, []string{"incus-gh-runner-runner-id"}, client.started)
	require.Len(t, client.fileWrites, 2)
	assert.Equal(t, payloadPath, client.fileWrites[0].path)
	assert.Equal(t, readyPath, client.fileWrites[1].path)
	assert.Equal(t, 0o600, client.fileWrites[0].mode)
	assert.Equal(t, 4, client.fileAttempts)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(client.fileWrites[0].content, &payload))
	assert.InDelta(t, 1, payload["version"], 0)
	assert.Equal(t, "secret-jit-config", payload["jit_config"])
}

func TestBackendDeleteRequiresOwnershipAndStoresDiagnostics(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.instances["owned"] = ownedInstance("owned", "Running", time.Now())
	client.instances["unowned"] = api.Instance{
		Name:   "unowned",
		Status: "Stopped",
		InstancePut: api.InstancePut{
			Config: api.ConfigMap{ownershipKey: "someone-else"},
		},
	}
	client.consoleLogs["owned"] = []byte("secret-safe console")
	var stored Diagnostics
	backend := newTestBackend(t, client, Options{
		Diagnostics: DiagnosticsSinkFunc(func(_ context.Context, diagnostics Diagnostics) error {
			stored = diagnostics
			return nil
		}),
	})

	err := backend.Delete(context.Background(), "unowned")
	require.ErrorContains(t, err, "refusing to delete unowned")
	assert.Empty(t, client.deleted)

	require.NoError(t, backend.Delete(context.Background(), "owned"))
	assert.Equal(t, []string{"owned"}, client.stopped)
	assert.Equal(t, []string{"owned"}, client.deleted)
	assert.Equal(t, Diagnostics{RunnerID: "owned", Console: []byte("secret-safe console")}, stored)
	require.NoError(t, backend.Delete(context.Background(), "owned"), "delete should be idempotent")
}

// newTestBackend constructs a backend with deterministic valid defaults.
func newTestBackend(t *testing.T, client client, overrides Options) *Backend {
	t.Helper()

	options := Options{
		Image:            "runner-image",
		Owner:            "test-owner",
		BootstrapTimeout: time.Minute,
		Payloads: PayloadSourceFunc(func(context.Context, string) (Payload, error) {
			return Payload{Version: 1, JITConfig: "test-config"}, nil
		}),
	}
	if overrides.Profiles != nil {
		options.Profiles = overrides.Profiles
	}
	if overrides.BootstrapTimeout != 0 {
		options.BootstrapTimeout = overrides.BootstrapTimeout
	}
	if overrides.Payloads != nil {
		options.Payloads = overrides.Payloads
	}
	if overrides.Diagnostics != nil {
		options.Diagnostics = overrides.Diagnostics
	}
	if overrides.Now != nil {
		options.Now = overrides.Now
	}
	if overrides.NewID != nil {
		options.NewID = overrides.NewID
	}
	if overrides.AgentPollInterval != 0 {
		options.AgentPollInterval = overrides.AgentPollInterval
	}
	if overrides.StatusReadTimeout != 0 {
		options.StatusReadTimeout = overrides.StatusReadTimeout
	}

	backend, err := newBackend(client, options)
	require.NoError(t, err)
	return backend
}

// ownedInstance creates a fake instance carrying the test ownership marker.
func ownedInstance(name string, status string, createdAt time.Time) api.Instance {
	return api.Instance{
		Name:      name,
		Status:    status,
		CreatedAt: createdAt,
		InstancePut: api.InstancePut{
			Config: api.ConfigMap{
				ownershipKey: "test-owner",
				createdAtKey: createdAt.UTC().Format(time.RFC3339Nano),
			},
		},
	}
}
