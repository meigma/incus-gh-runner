package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/controller"
	"github.com/meigma/incus-gh-runner/internal/provenance"
)

const (
	testFingerprintA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testFingerprintB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// fileWrite records one successful guest file transfer.
type fileWrite struct {
	name    string
	path    string
	content []byte
	mode    int
}

// testFencerFunc adapts a function to the controller registration-fence port.
type testFencerFunc func(context.Context, string) error

// Fence invokes f for runnerID.
func (f testFencerFunc) Fence(ctx context.Context, runnerID string) error {
	return f(ctx, runnerID)
}

// fakeClient provides an in-memory Incus lifecycle for adapter behavior tests.
type fakeClient struct {
	images             map[string]api.Image
	imageAliases       map[string]string
	profiles           map[string]api.Profile
	instances          map[string]api.Instance
	instanceETags      map[string]string
	getInstance        func(context.Context, string) (*api.Instance, string, error)
	statusFiles        map[string][]byte
	statusErrors       map[string]error
	statusRead         func(context.Context, string, string) ([]byte, error)
	consoleLogs        map[string][]byte
	createRequest      api.InstancesPost
	updatedRequest     api.InstancePut
	updateETag         string
	started            []string
	stopped            []string
	stopETags          []string
	deleted            []string
	fileWrites         []fileWrite
	fileAttempts       int
	agentFailures      int
	consoleError       error
	deleteError        error
	createInstanceErr  error
	updateInstanceErr  error
	startInstanceError error
	stopInstanceError  error
}

// newFakeClient creates an empty in-memory Incus client.
func newFakeClient() *fakeClient {
	return &fakeClient{
		images:        make(map[string]api.Image),
		imageAliases:  make(map[string]string),
		profiles:      make(map[string]api.Profile),
		instances:     make(map[string]api.Instance),
		instanceETags: make(map[string]string),
		statusFiles:   make(map[string][]byte),
		statusErrors:  make(map[string]error),
		consoleLogs:   make(map[string][]byte),
		fileWrites:    make([]fileWrite, 0),
		started:       make([]string, 0),
		stopped:       make([]string, 0),
		stopETags:     make([]string, 0),
		deleted:       make([]string, 0),
		fileAttempts:  0,
	}
}

// ResolveImage resolves an image or alias known to the fake.
func (f *fakeClient) ResolveImage(_ context.Context, name string) (*api.Image, error) {
	image, ok := f.images[name]
	if !ok {
		target, aliasOK := f.imageAliases[name]
		if !aliasOK {
			return nil, errNotFound
		}
		image, ok = f.images[target]
		if !ok {
			return nil, errNotFound
		}
	}

	return &image, nil
}

// GetProfile resolves a profile known to the fake.
func (f *fakeClient) GetProfile(_ context.Context, name string) (*api.Profile, error) {
	profile, ok := f.profiles[name]
	if !ok {
		return nil, errNotFound
	}

	return &profile, nil
}

// GetInstances returns a stable snapshot of fake instances.
func (f *fakeClient) GetInstances(context.Context) ([]api.Instance, error) {
	instances := make([]api.Instance, 0, len(f.instances))
	for _, instance := range f.instances {
		instances = append(instances, instance)
	}
	return instances, nil
}

// GetInstance returns one fake instance and its ETag.
func (f *fakeClient) GetInstance(ctx context.Context, name string) (*api.Instance, string, error) {
	if f.getInstance != nil {
		return f.getInstance(ctx, name)
	}
	instance, ok := f.instances[name]
	if !ok {
		return nil, "", errNotFound
	}
	return &instance, f.instanceETags[name], nil
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
	instance := f.instances[request.Name]
	instance.Config[instanceUUIDKey] = stableTestUUID(request.Name)
	f.instances[request.Name] = instance
	f.instanceETags[request.Name] = "etag-" + request.Name
	return nil
}

// UpdateInstance records and materializes one conditional fake instance update.
func (f *fakeClient) UpdateInstance(_ context.Context, name string, request api.InstancePut, etag string) error {
	if f.updateInstanceErr != nil {
		return f.updateInstanceErr
	}
	instance, ok := f.instances[name]
	if !ok {
		return errNotFound
	}
	if etag == "" || etag != f.instanceETags[name] {
		return errors.New("ETag mismatch")
	}
	f.updatedRequest = request
	f.updateETag = etag
	instance.InstancePut = request
	f.instances[name] = instance
	f.instanceETags[name] = etag + "-updated"
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

// StopInstance records the fake conditional stop transition.
func (f *fakeClient) StopInstance(_ context.Context, name string, etag string) error {
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
	f.stopETags = append(f.stopETags, etag)
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
	client.images["runner-image"] = api.Image{
		Fingerprint: testFingerprintA,
		ImagePut:    api.ImagePut{Profiles: []string{}},
	}
	client.profiles["runner"] = api.Profile{Name: "runner"}
	backend := newTestBackend(t, client, Options{Profiles: []string{"runner"}})

	require.NoError(t, backend.Preflight(context.Background()))

	delete(client.profiles, "runner")
	err := backend.Preflight(context.Background())
	require.ErrorContains(t, err, "resolve runner profile")
}

func TestBackendPreflightRequiresFullImageFingerprint(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.images["runner-image"] = api.Image{Fingerprint: "short", ImagePut: api.ImagePut{Profiles: []string{}}}
	backend := newTestBackend(t, client, Options{})

	err := backend.Preflight(context.Background())

	require.ErrorContains(t, err, "image fingerprint must contain 64 hexadecimal characters")
}

// TestBackendPreflightRejectsReservedProfileMetadata protects the launch-digest namespace boundary.
func TestBackendPreflightRejectsReservedProfileMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "Incus volatile namespace", key: "volatile.last_state.idmap"},
		{name: "Incus image metadata namespace", key: "image.os"},
		{name: "controller audit namespace", key: "user.incus-gh-runner.owner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newFakeClient()
			client.images["runner-image"] = api.Image{
				Fingerprint: testFingerprintA,
				ImagePut:    api.ImagePut{Profiles: []string{"runner"}},
			}
			client.profiles["runner"] = api.Profile{
				Name:       "runner",
				ProfilePut: api.ProfilePut{Config: api.ConfigMap{tt.key: "reserved"}},
			}
			backend := newTestBackend(t, client, Options{})

			err := backend.Preflight(context.Background())

			require.ErrorContains(t, err, "uses reserved config key")
			assert.Contains(t, err.Error(), tt.key)
		})
	}
}

func TestBackendCreateUsesPreflightImageDespiteAliasRetarget(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.imageAliases["runner-image"] = testFingerprintA
	client.images[testFingerprintA] = api.Image{
		Fingerprint: testFingerprintA,
		ImagePut:    api.ImagePut{Profiles: []string{}},
	}
	client.images[testFingerprintB] = api.Image{
		Fingerprint: testFingerprintB,
		ImagePut:    api.ImagePut{Profiles: []string{}},
	}
	backend := newTestBackend(t, client, Options{NewID: func() string { return "alias-race" }})
	require.NoError(t, backend.Preflight(context.Background()))
	client.imageAliases["runner-image"] = testFingerprintB

	_, err := backend.Create(context.Background())

	require.NoError(t, err)
	assert.Equal(t, testFingerprintA, client.createRequest.Source.Fingerprint)
	assert.Equal(t, testFingerprintA, client.createRequest.Config[imageKey])
	assert.Equal(t, "runner-image", client.createRequest.Config[imageReferenceKey])
}

func TestEffectiveProfileNamesMatchesIncusSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configured    []string
		imageProfiles []string
		want          []string
	}{
		{
			name:          "configured",
			configured:    []string{"runner"},
			imageProfiles: []string{"image-profile"},
			want:          []string{"runner"},
		},
		{name: "from image profiles", imageProfiles: []string{"image-profile"}, want: []string{"image-profile"}},
		{name: "explicitly empty image profiles", imageProfiles: []string{}, want: []string{}},
		{name: "default", want: []string{defaultProfileName}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, effectiveProfileNames(tt.configured, tt.imageProfiles))
		})
	}
}

func TestBackendCreateFailsClosedOnProfileDrift(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.images["runner-image"] = api.Image{
		Fingerprint: testFingerprintA,
		ImagePut:    api.ImagePut{Profiles: []string{}},
	}
	client.profiles["runner"] = api.Profile{
		Name:       "runner",
		ProfilePut: api.ProfilePut{Config: api.ConfigMap{"limits.cpu": "2"}},
	}
	backend := newTestBackend(t, client, Options{Profiles: []string{"runner"}})
	require.NoError(t, backend.Preflight(context.Background()))
	client.profiles["runner"] = api.Profile{
		Name:       "runner",
		ProfilePut: api.ProfilePut{Config: api.ConfigMap{"limits.cpu": "8"}},
	}

	_, err := backend.Create(context.Background())

	require.ErrorContains(t, err, `runner profile "runner" changed after preflight`)
	assert.Empty(t, client.createRequest.Name, "profile drift must fail before Incus create")
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
	client.images["runner-image"] = api.Image{
		Fingerprint: testFingerprintA,
		ImagePut:    api.ImagePut{Profiles: []string{}},
	}
	client.profiles["default"] = api.Profile{
		Name: "default",
		ProfilePut: api.ProfilePut{
			Config: api.ConfigMap{"limits.cpu": "2", "limits.memory": "4GiB"},
		},
	}
	client.profiles["runner"] = api.Profile{
		Name: "runner",
		ProfilePut: api.ProfilePut{
			Config: api.ConfigMap{"limits.cpu": "4"},
			Devices: api.DevicesMap{
				"root": {"type": "disk", "pool": "runner", "path": "/"},
			},
		},
	}
	client.agentFailures = 2
	backend := newTestBackend(t, client, Options{
		Profiles:          []string{"default", "runner"},
		Now:               func() time.Time { return now },
		NewID:             func() string { return "runner-id" },
		AgentPollInterval: time.Millisecond,
		Payloads: PayloadSourceFunc(func(_ context.Context, runnerID string) (Payload, error) {
			assert.Equal(t, "incus-gh-runner-runner-id", runnerID)
			assert.Equal(t, "Stopped", client.instances[runnerID].Status)
			assert.Empty(t, client.started, "the VM must remain stopped until its JIT reference is durable")
			payload := validTestPayload(runnerID)
			payload.JITConfig = "secret-jit-config"
			return payload, nil
		}),
	})
	require.NoError(t, backend.Preflight(context.Background()))

	runner, err := backend.Create(context.Background())

	require.NoError(t, err)
	assert.Equal(t, controller.Runner{
		ID:    "incus-gh-runner-runner-id",
		State: controller.RunnerProvisioning,
	}, runner)
	assert.Equal(t, api.InstanceTypeVM, client.createRequest.Type)
	assert.Empty(t, client.createRequest.Source.Alias)
	assert.Equal(t, testFingerprintA, client.createRequest.Source.Fingerprint)
	assert.NotNil(t, client.createRequest.Profiles)
	assert.Empty(t, client.createRequest.Profiles)
	encodedRequest, err := json.Marshal(client.createRequest)
	require.NoError(t, err)
	assert.Contains(t, string(encodedRequest), `"profiles":[]`, "the detached profile list must survive SDK encoding")
	assert.Equal(t, "4", client.createRequest.Config["limits.cpu"])
	assert.Equal(t, "4GiB", client.createRequest.Config["limits.memory"])
	assert.Equal(
		t,
		map[string]string{"type": "disk", "pool": "runner", "path": "/"},
		client.createRequest.Devices["root"],
	)
	assert.Equal(t, "test-owner", client.createRequest.Config[ownershipKey])
	assert.Equal(t, "runner-id", client.createRequest.Config[correlationKey])
	assert.Equal(t, now.Format(time.RFC3339Nano), client.createRequest.Config[createdAtKey])
	assert.Equal(t, testFingerprintA, client.createRequest.Config[imageKey])
	assert.Equal(t, "runner-image", client.createRequest.Config[imageReferenceKey])
	var profileReferences []profileReference
	require.NoError(t, json.Unmarshal([]byte(client.createRequest.Config[profilesKey]), &profileReferences))
	require.Len(t, profileReferences, 2)
	assert.Equal(t, []string{"default", "runner"}, []string{profileReferences[0].Name, profileReferences[1].Name})
	assert.NotEmpty(t, profileReferences[0].SHA256)
	assert.NotEmpty(t, profileReferences[1].SHA256)
	expectedLaunchDigest, err := provenance.LaunchDigest(provenance.LaunchInput{
		Version:          provenance.Version,
		InstanceType:     provenance.LaunchInstanceType,
		ImageFingerprint: testFingerprintA,
		Profiles: []provenance.Profile{
			{Name: profileReferences[0].Name, SHA256: profileReferences[0].SHA256},
			{Name: profileReferences[1].Name, SHA256: profileReferences[1].SHA256},
		},
		Config: map[string]string{"limits.cpu": "4", "limits.memory": "4GiB"},
		Devices: map[string]map[string]string{
			"root": {"type": "disk", "pool": "runner", "path": "/"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, expectedLaunchDigest, client.createRequest.Config[launchDigestKey])
	assert.Equal(t, "41", client.updatedRequest.Config[jitRunnerIDKey])
	assert.Equal(t, "incus-gh-runner-runner-id", client.updatedRequest.Config[jitRunnerNameKey])
	assert.Equal(t, "73", client.updatedRequest.Config[jitScaleSetIDKey])
	assert.Equal(t, stableTestUUID("incus-gh-runner-runner-id"), client.updatedRequest.Config[boundInstanceUUIDKey])
	assert.Equal(t, expectedLaunchDigest, client.updatedRequest.Config[launchDigestKey])
	assert.Equal(t, "etag-incus-gh-runner-runner-id", client.updateETag)
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

// TestBackendSnapshotCorrelatesJobAndReconstructsLaunchIdentity proves signed machine authority.
func TestBackendSnapshotCorrelatesJobAndReconstructsLaunchIdentity(t *testing.T) {
	t.Parallel()

	backend, client, event := newProofSnapshotBackend(t)
	instance := client.instances[event.RunnerName]
	instance.Config["volatile.last_state.power"] = "RUNNING"
	instance.Config["image.os"] = "Ubuntu"
	client.instances[event.RunnerName] = instance

	machine, err := backend.Snapshot(context.Background(), event)

	require.NoError(t, err)
	assert.Equal(t, provenance.Machine{
		IncusProject:              "test-project",
		InstanceName:              event.RunnerName,
		InstanceUUID:              stableTestUUID(event.RunnerName),
		ImageFingerprint:          testFingerprintA,
		LaunchConfigurationSHA256: client.instances[event.RunnerName].Config[launchDigestKey],
		Profiles:                  []provenance.Profile{},
	}, machine)
}

// TestBackendSnapshotRejectsCorrelationAndLaunchDrift proves every signed machine field is host-authoritative.
func TestBackendSnapshotRejectsCorrelationAndLaunchDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*api.Instance, *provenance.JobStarted)
		wantErr string
	}{
		{
			name: "runner ID mismatch",
			mutate: func(_ *api.Instance, event *provenance.JobStarted) {
				event.RunnerID++
			},
			wantErr: "JIT registration metadata does not match",
		},
		{
			name: "scale set mismatch",
			mutate: func(_ *api.Instance, event *provenance.JobStarted) {
				event.ScaleSetID++
			},
			wantErr: "JIT registration metadata does not match",
		},
		{
			name: "stopped machine",
			mutate: func(instance *api.Instance, _ *provenance.JobStarted) {
				instance.Status = "Stopped"
			},
			wantErr: "non-running instance",
		},
		{
			name: "same-name replacement",
			mutate: func(instance *api.Instance, _ *provenance.JobStarted) {
				instance.Config[instanceUUIDKey] = stableTestUUID("replacement")
			},
			wantErr: "replacement instance",
		},
		{
			name: "config drift",
			mutate: func(instance *api.Instance, _ *provenance.JobStarted) {
				instance.Config["limits.cpu"] = "8"
			},
			wantErr: "launch configuration drift detected",
		},
		{
			name: "device drift",
			mutate: func(instance *api.Instance, _ *provenance.JobStarted) {
				instance.Devices["extra"] = map[string]string{"type": "disk", "path": "/extra"}
			},
			wantErr: "launch configuration drift detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			backend, client, event := newProofSnapshotBackend(t)
			instance := client.instances[event.RunnerName]
			instance.Config = maps.Clone(instance.Config)
			instance.Devices = maps.Clone(instance.Devices)
			tt.mutate(&instance, &event)
			client.instances[event.RunnerName] = instance

			_, err := backend.Snapshot(context.Background(), event)

			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// newProofSnapshotBackend creates one running runner with a durable JIT binding.
func newProofSnapshotBackend(t *testing.T) (*Backend, *fakeClient, provenance.JobStarted) {
	t.Helper()
	client := newFakeClient()
	client.images["runner-image"] = api.Image{
		Fingerprint: testFingerprintA,
		ImagePut:    api.ImagePut{Profiles: []string{}},
	}
	backend := newTestBackend(t, client, Options{NewID: func() string { return "runner-id" }})
	require.NoError(t, backend.Preflight(context.Background()))
	runner, err := backend.Create(context.Background())
	require.NoError(t, err)

	return backend, client, provenance.JobStarted{
		Owner:           "meigma",
		Repository:      "incus-gh-runner",
		WorkflowRef:     "meigma/incus-gh-runner/.github/workflows/test.yml@refs/heads/master",
		WorkflowRunID:   101,
		JobID:           "job-1",
		RunnerRequestID: 202,
		RunnerID:        41,
		RunnerName:      runner.ID,
		EventName:       "push",
		ScaleSetID:      73,
		ScaleSetName:    "incus-runners",
	}
}

// TestBackendCreateDoesNotBootWithoutDurableJITBinding proves every failed binding leaves the VM stopped.
func TestBackendCreateDoesNotBootWithoutDurableJITBinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		mutatePayload   func(*Payload)
		configureClient func(*fakeClient)
		wantErr         string
	}{
		{
			name: "empty runner reference",
			mutatePayload: func(payload *Payload) {
				payload.Runner.ID = 0
			},
			wantErr: "runtime payload",
		},
		{
			name: "mismatched runner name",
			mutatePayload: func(payload *Payload) {
				payload.Runner.Name = "another-runner"
			},
			wantErr: "runtime payload",
		},
		{
			name: "empty scale set reference",
			mutatePayload: func(payload *Payload) {
				payload.Runner.ScaleSetID = 0
			},
			wantErr: "runtime payload",
		},
		{
			name: "stale instance ETag",
			configureClient: func(client *fakeClient) {
				client.getInstance = func(_ context.Context, name string) (*api.Instance, string, error) {
					instance := client.instances[name]
					return &instance, "stale-etag", nil
				}
			},
			wantErr: "ETag mismatch",
		},
		{
			name: "failed conditional update",
			configureClient: func(client *fakeClient) {
				client.updateInstanceErr = errors.New("update unavailable")
			},
			wantErr: "update unavailable",
		},
		{
			name: "same-name replacement after update",
			configureClient: func(client *fakeClient) {
				lookups := 0
				client.getInstance = func(_ context.Context, name string) (*api.Instance, string, error) {
					instance := client.instances[name]
					lookups++
					if lookups == 2 {
						instance.Config = maps.Clone(instance.Config)
						instance.Config[instanceUUIDKey] = stableTestUUID("replacement")
					}
					return &instance, client.instanceETags[name], nil
				}
			},
			wantErr: "replacement Incus instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newFakeClient()
			var fenced []string
			client.images["runner-image"] = api.Image{
				Fingerprint: testFingerprintA,
				ImagePut:    api.ImagePut{Profiles: []string{}},
			}
			backend := newTestBackend(t, client, Options{
				NewID: func() string { return "runner-id" },
				RunnerFencer: testFencerFunc(func(_ context.Context, runnerID string) error {
					fenced = append(fenced, runnerID)
					return nil
				}),
				Payloads: PayloadSourceFunc(func(_ context.Context, runnerName string) (Payload, error) {
					payload := validTestPayload(runnerName)
					if tt.mutatePayload != nil {
						tt.mutatePayload(&payload)
					}
					return payload, nil
				}),
			})
			require.NoError(t, backend.Preflight(context.Background()))
			if tt.configureClient != nil {
				tt.configureClient(client)
			}

			_, err := backend.Create(context.Background())

			require.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, []string{"incus-gh-runner-runner-id"}, fenced)
			assert.Empty(t, client.started, "a runner without a durable JIT binding must not boot")
			assert.Empty(
				t,
				client.fileWrites,
				"a runner without a durable JIT binding must not receive the JIT payload",
			)
		})
	}
}

// TestBackendCreateReportsProvisioningAndFenceFailures preserves both actionable errors.
func TestBackendCreateReportsProvisioningAndFenceFailures(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.images["runner-image"] = api.Image{
		Fingerprint: testFingerprintA,
		ImagePut:    api.ImagePut{Profiles: []string{}},
	}
	client.startInstanceError = errors.New("start unavailable")
	backend := newTestBackend(t, client, Options{
		NewID: func() string { return "runner-id" },
		RunnerFencer: testFencerFunc(func(context.Context, string) error {
			return errors.New("fence unavailable")
		}),
	})
	require.NoError(t, backend.Preflight(context.Background()))

	_, err := backend.Create(context.Background())

	require.ErrorContains(t, err, "start unavailable")
	require.ErrorContains(t, err, "fence unavailable")
	assert.Empty(t, client.fileWrites)
}

func TestBackendDeleteRequiresOwnershipAndStoresDiagnostics(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.instances["owned"] = ownedInstance("owned", "Running", time.Now())
	client.instanceETags["owned"] = "owned-etag"
	client.instances["unowned"] = api.Instance{
		Name:   "unowned",
		Status: "Stopped",
		InstancePut: api.InstancePut{
			Config: api.ConfigMap{ownershipKey: "someone-else"},
		},
	}
	client.consoleLogs["owned"] = []byte("secret-safe console")
	var stored Diagnostics
	var fenced []string
	backend := newTestBackend(t, client, Options{
		RunnerFencer: testFencerFunc(func(_ context.Context, runnerID string) error {
			assert.Empty(t, client.deleted, "registration fencing must precede instance deletion")
			fenced = append(fenced, runnerID)
			return nil
		}),
		Diagnostics: DiagnosticsSinkFunc(func(_ context.Context, diagnostics Diagnostics) error {
			stored = diagnostics
			return nil
		}),
	})

	err := backend.Delete(context.Background(), "unowned")
	require.ErrorContains(t, err, "unowned Incus instance")
	assert.Empty(t, client.deleted)

	require.NoError(t, backend.Delete(context.Background(), "owned"))
	assert.Equal(t, []string{"owned"}, client.stopped)
	assert.Equal(t, []string{"owned-etag"}, client.stopETags)
	assert.Equal(t, []string{"owned"}, client.deleted)
	assert.Equal(t, []string{"owned"}, fenced)
	assert.Equal(t, Diagnostics{RunnerID: "owned", Console: []byte("secret-safe console")}, stored)
	require.NoError(t, backend.Delete(context.Background(), "owned"), "delete should be idempotent")
}

// TestBackendDeleteStopsBeforeMutationWhenRegistrationFenceFails protects recovered runners.
func TestBackendDeleteStopsBeforeMutationWhenRegistrationFenceFails(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.instances["owned"] = ownedInstance("owned", "Running", time.Now())
	client.instanceETags["owned"] = "owned-etag"
	backend := newTestBackend(t, client, Options{
		RunnerFencer: testFencerFunc(func(context.Context, string) error {
			return errors.New("fence unavailable")
		}),
	})

	err := backend.Delete(context.Background(), "owned")

	require.ErrorContains(t, err, "fence unavailable")
	assert.Empty(t, client.stopped)
	assert.Empty(t, client.deleted)
}

// TestBackendDeleteFencesRecoveredProvisioningCrashPoints closes every JIT allocation window.
func TestBackendDeleteFencesRecoveredProvisioningCrashPoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   string
		metadata bool
	}{
		{name: "after JIT allocation", status: "Stopped"},
		{name: "after metadata persistence", status: "Stopped", metadata: true},
		{name: "after VM start", status: "Running", metadata: true},
		{name: "after partial payload delivery", status: "Running", metadata: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newFakeClient()
			instance := ownedInstance("runner", tt.status, time.Now())
			if tt.metadata {
				instance.Config[jitRunnerIDKey] = "41"
				instance.Config[jitRunnerNameKey] = "runner"
				instance.Config[jitScaleSetIDKey] = "73"
			}
			client.instances["runner"] = instance
			client.instanceETags["runner"] = "runner-etag"
			var fenced []string
			backend := newTestBackend(t, client, Options{
				RunnerFencer: testFencerFunc(func(_ context.Context, runnerID string) error {
					assert.Empty(t, client.stopped)
					assert.Empty(t, client.deleted)
					fenced = append(fenced, runnerID)
					return nil
				}),
			})

			err := backend.Delete(context.Background(), "runner")

			require.NoError(t, err)
			assert.Equal(t, []string{"runner"}, fenced)
			assert.Equal(t, []string{"runner"}, client.deleted)
		})
	}
}

func TestBackendDeleteRequiresETagBeforeStop(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	client.instances["owned"] = ownedInstance("owned", "Running", time.Now())
	backend := newTestBackend(t, client, Options{})

	err := backend.Delete(context.Background(), "owned")

	require.ErrorContains(t, err, `refusing to stop Incus instance "owned" without an ETag`)
	assert.Empty(t, client.stopped)
	assert.Empty(t, client.deleted)
}

func TestBackendDeleteRefusesSameNameReplacement(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	original := ownedInstance("runner", "Running", now)
	replacement := ownedInstance("runner", "Stopped", now.Add(time.Second))
	replacement.Config[instanceUUIDKey] = stableTestUUID("replacement")
	unowned := original
	unowned.Config = map[string]string{
		ownershipKey:    "someone-else",
		instanceUUIDKey: original.Config[instanceUUIDKey],
	}

	tests := []struct {
		name      string
		sequence  []api.Instance
		wantStops int
		wantError string
	}{
		{
			name:      "replacement before stop",
			sequence:  []api.Instance{original, replacement},
			wantError: "refusing to stop replacement",
		},
		{
			name:      "replacement after stop",
			sequence:  []api.Instance{original, original, replacement},
			wantStops: 1,
			wantError: "refusing to diagnostics replacement",
		},
		{
			name:      "replacement before delete",
			sequence:  []api.Instance{original, original, original, replacement},
			wantStops: 1,
			wantError: "refusing to delete replacement",
		},
		{
			name:      "ownership removed before delete",
			sequence:  []api.Instance{original, original, original, unowned},
			wantStops: 1,
			wantError: "refusing to delete unowned Incus instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newFakeClient()
			client.instances["runner"] = original
			lookup := 0
			client.getInstance = func(context.Context, string) (*api.Instance, string, error) {
				require.Less(t, lookup, len(tt.sequence))
				instance := tt.sequence[lookup]
				lookup++
				return &instance, fmt.Sprintf("etag-%d", lookup), nil
			}
			backend := newTestBackend(t, client, Options{})

			err := backend.Delete(context.Background(), "runner")

			require.ErrorContains(t, err, tt.wantError)
			assert.Len(t, client.stopped, tt.wantStops)
			assert.Empty(t, client.deleted, "replacement must never be deleted")
		})
	}
}

// newTestBackend constructs a backend with deterministic valid defaults.
func newTestBackend(t *testing.T, client client, overrides Options) *Backend {
	t.Helper()

	options := Options{
		Project:          "test-project",
		Image:            "runner-image",
		Owner:            "test-owner",
		BootstrapTimeout: time.Minute,
		RunnerFencer: testFencerFunc(func(context.Context, string) error {
			return nil
		}),
		Payloads: PayloadSourceFunc(func(_ context.Context, runnerID string) (Payload, error) {
			return validTestPayload(runnerID), nil
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
	if overrides.RunnerFencer != nil {
		options.RunnerFencer = overrides.RunnerFencer
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

// validTestPayload creates a complete fake JIT response for runnerName.
func validTestPayload(runnerName string) Payload {
	return Payload{
		Version:   1,
		JITConfig: "test-config",
		Runner: JITRunnerReference{
			ID:         41,
			Name:       runnerName,
			ScaleSetID: 73,
		},
	}
}

// ownedInstance creates a fake instance carrying the test ownership marker.
func ownedInstance(name string, status string, createdAt time.Time) api.Instance {
	return api.Instance{
		Name:      name,
		Status:    status,
		CreatedAt: createdAt,
		InstancePut: api.InstancePut{
			Config: api.ConfigMap{
				ownershipKey:    "test-owner",
				createdAtKey:    createdAt.UTC().Format(time.RFC3339Nano),
				instanceUUIDKey: stableTestUUID(name),
			},
		},
	}
}

// stableTestUUID derives a valid stable identity from name.
func stableTestUUID(name string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}
