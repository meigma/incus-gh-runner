package incus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	incusclient "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"

	"github.com/meigma/incus-gh-runner/internal/controller"
	"github.com/meigma/incus-gh-runner/internal/provenance"
)

const (
	defaultNamePrefix      = "incus-gh-runner-"
	defaultAgentPoll       = 250 * time.Millisecond
	defaultStatusRead      = 5 * time.Second
	payloadPath            = "/run/incus-gh-runner/payload.json"
	readyPath              = "/run/incus-gh-runner/payload.ready"
	statusPath             = "/run/incus-gh-runner/status.json"
	guestStatusStarting    = "starting"
	guestStatusStopped     = "stopped"
	ownershipKey           = "user.incus-gh-runner.owner"
	correlationKey         = "user.incus-gh-runner.correlation-id"
	createdAtKey           = "user.incus-gh-runner.created-at"
	imageKey               = "user.incus-gh-runner.image"
	imageReferenceKey      = "user.incus-gh-runner.image-reference"
	profilesKey            = "user.incus-gh-runner.profiles"
	instanceUUIDKey        = "volatile.uuid"
	defaultProfileName     = "default"
	maximumInstanceNameLen = 63
	imageFingerprintLength = sha256.Size * 2
	payloadFileMode        = 0o600
)

// profileReference records one immutable profile input in instance audit metadata.
type profileReference struct {
	// Name is the operator-facing profile name resolved during preflight.
	Name string `json:"name"`
	// SHA256 identifies the effective profile configuration and devices.
	SHA256 string `json:"sha256"`
}

// runtimeIdentity is the immutable Incus environment captured by preflight.
type runtimeIdentity struct {
	// imageFingerprint is the full content-addressed image identity.
	imageFingerprint string
	// profiles record the ordered profile inputs used during materialization.
	profiles []profileReference
	// profileMetadata is the serialized audit form written to instance config.
	profileMetadata string
	// config is the effective profile configuration captured during preflight.
	config api.ConfigMap
	// devices are the effective profile devices captured during preflight.
	devices api.DevicesMap
}

// Payload is the versioned runtime input consumed by the one-shot guest.
type Payload struct {
	// Version identifies the guest payload contract.
	Version int
	// JITConfig is the opaque one-runner GitHub registration configuration.
	JITConfig string
}

// PayloadSource supplies a fresh runtime payload for each new runner.
type PayloadSource interface {
	// Payload returns the runtime input for runnerID without logging its secrets.
	Payload(ctx context.Context, runnerID string) (Payload, error)
}

// PayloadSourceFunc adapts a function to PayloadSource.
type PayloadSourceFunc func(ctx context.Context, runnerID string) (Payload, error)

// Payload calls f with the runner allocation identity.
func (f PayloadSourceFunc) Payload(ctx context.Context, runnerID string) (Payload, error) {
	return f(ctx, runnerID)
}

// Diagnostics contains terminal evidence collected before instance deletion.
type Diagnostics struct {
	// RunnerID is the owned instance identity.
	RunnerID string
	// Console is the serial console content and may contain sensitive workload output.
	Console []byte
}

// DiagnosticsSink stores terminal evidence outside the controller logs.
type DiagnosticsSink interface {
	// Store persists diagnostics synchronously before the instance is removed.
	Store(ctx context.Context, diagnostics Diagnostics) error
}

// DiagnosticsSinkFunc adapts a function to DiagnosticsSink.
type DiagnosticsSinkFunc func(ctx context.Context, diagnostics Diagnostics) error

// Store calls f with terminal diagnostics.
func (f DiagnosticsSinkFunc) Store(ctx context.Context, diagnostics Diagnostics) error {
	return f(ctx, diagnostics)
}

// Options configures the ownership-scoped Incus backend.
type Options struct {
	// Image is an existing local image alias or fingerprint.
	Image string
	// Profiles are existing profile sources pinned and materialized into every runner VM.
	Profiles []string
	// Owner is the exact durable ownership value required before any mutation.
	Owner string
	// NamePrefix prefixes generated instance names.
	NamePrefix string
	// BootstrapTimeout bounds how long an unready running instance counts as capacity.
	BootstrapTimeout time.Duration
	// Payloads supplies the guest runtime contract for each new instance.
	Payloads PayloadSource
	// Diagnostics receives serial-console evidence before deletion.
	Diagnostics DiagnosticsSink
	// Logger receives secret-safe lifecycle events.
	Logger *slog.Logger
	// Now supplies the current time for deterministic lifecycle tests.
	Now func() time.Time
	// NewID supplies a unique, log-safe correlation identifier.
	NewID func() string
	// AgentPollInterval controls retries while waiting for Incus agent file transfer.
	AgentPollInterval time.Duration
	// StatusReadTimeout caps each individual guest-status observation.
	StatusReadTimeout time.Duration
}

// Backend implements the controller runner lifecycle through Incus.
type Backend struct {
	client   client
	options  Options
	identity *runtimeIdentity
	mu       sync.RWMutex
}

// NewBackend constructs an ownership-scoped backend from an Incus server.
func NewBackend(server incusclient.InstanceServer, options Options) (*Backend, error) {
	client, err := newServerClient(server)
	if err != nil {
		return nil, err
	}

	return newBackend(client, options)
}

// newBackend constructs a backend around the narrow client used by its behavior tests.
func newBackend(client client, options Options) (*Backend, error) {
	if client == nil {
		return nil, errors.New("incus client is required")
	}
	options = options.withDefaults()
	if strings.TrimSpace(options.Image) == "" {
		return nil, errors.New("incus image is required")
	}
	if strings.TrimSpace(options.Owner) == "" {
		return nil, errors.New("incus ownership identity is required")
	}
	if options.BootstrapTimeout <= 0 {
		return nil, errors.New("bootstrap timeout must be positive")
	}
	if options.Payloads == nil {
		return nil, errors.New("payload source is required")
	}
	if len(options.NamePrefix)+36 > maximumInstanceNameLen {
		return nil, fmt.Errorf("instance name prefix is too long: %q", options.NamePrefix)
	}

	return &Backend{client: client, options: options}, nil
}

// withDefaults fills optional operational dependencies.
func (o Options) withDefaults() Options {
	if o.NamePrefix == "" {
		o.NamePrefix = defaultNamePrefix
	}
	if o.Diagnostics == nil {
		o.Diagnostics = DiagnosticsSinkFunc(func(context.Context, Diagnostics) error { return nil })
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.NewID == nil {
		o.NewID = uuid.NewString
	}
	if o.AgentPollInterval <= 0 {
		o.AgentPollInterval = defaultAgentPoll
	}
	if o.StatusReadTimeout <= 0 {
		o.StatusReadTimeout = defaultStatusRead
	}

	return o
}

// Preflight pins the configured image and effective profiles without mutating Incus.
func (b *Backend) Preflight(ctx context.Context) error {
	image, err := b.client.ResolveImage(ctx, b.options.Image)
	if err != nil {
		return fmt.Errorf("resolve runner image %q: %w", b.options.Image, err)
	}
	if image == nil {
		return fmt.Errorf("resolve runner image %q: empty image", b.options.Image)
	}
	if fingerprintErr := validateImageFingerprint(image.Fingerprint); fingerprintErr != nil {
		return fmt.Errorf("resolve runner image %q: %w", b.options.Image, fingerprintErr)
	}

	profileNames := effectiveProfileNames(b.options.Profiles, image.Profiles)
	identity := &runtimeIdentity{
		imageFingerprint: image.Fingerprint,
		profiles:         make([]profileReference, 0, len(profileNames)),
		config:           make(api.ConfigMap),
		devices:          make(api.DevicesMap),
	}
	for _, profileName := range profileNames {
		if strings.TrimSpace(profileName) == "" {
			return errors.New("incus profile names must not be empty")
		}
		profile, profileErr := b.client.GetProfile(ctx, profileName)
		if profileErr != nil {
			return fmt.Errorf("resolve runner profile %q: %w", profileName, profileErr)
		}
		digest, digestErr := profileDigest(profile)
		if digestErr != nil {
			return fmt.Errorf("digest runner profile %q: %w", profileName, digestErr)
		}
		identity.profiles = append(identity.profiles, profileReference{Name: profileName, SHA256: digest})
		maps.Copy(identity.config, profile.Config)
		for name, device := range profile.Devices {
			identity.devices[name] = maps.Clone(device)
		}
	}
	profileMetadata, err := json.Marshal(identity.profiles)
	if err != nil {
		return fmt.Errorf("encode runner profile identities: %w", err)
	}
	identity.profileMetadata = string(profileMetadata)

	b.mu.Lock()
	b.identity = identity
	b.mu.Unlock()
	b.options.Logger.InfoContext(
		ctx,
		"Incus runtime identity pinned",
		"image_reference", b.options.Image,
		"image_fingerprint", image.Fingerprint,
		"profiles", identity.profiles,
	)

	return nil
}

// validateImageFingerprint requires a full SHA-256 image identity.
func validateImageFingerprint(fingerprint string) error {
	if len(fingerprint) != imageFingerprintLength {
		return fmt.Errorf("image fingerprint must contain %d hexadecimal characters", imageFingerprintLength)
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return fmt.Errorf("image fingerprint is not hexadecimal: %w", err)
	}

	return nil
}

// effectiveProfileNames reproduces Incus image and default-profile selection.
func effectiveProfileNames(configured []string, imageProfiles []string) []string {
	if len(configured) > 0 {
		return append(make([]string, 0, len(configured)), configured...)
	}
	if imageProfiles != nil {
		return append(make([]string, 0, len(imageProfiles)), imageProfiles...)
	}

	return []string{defaultProfileName}
}

// profileDigest hashes the effective profile configuration and devices.
func profileDigest(profile *api.Profile) (string, error) {
	if profile == nil {
		return "", errors.New("profile is nil")
	}

	return provenance.ProfileDigest(profile.Config, profile.Devices)
}

// pinnedIdentity returns an isolated copy of the preflight runtime identity.
func (b *Backend) pinnedIdentity() (*runtimeIdentity, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.identity == nil {
		return nil, errors.New("incus backend preflight has not completed")
	}

	identity := &runtimeIdentity{
		imageFingerprint: b.identity.imageFingerprint,
		profiles:         append([]profileReference(nil), b.identity.profiles...),
		profileMetadata:  b.identity.profileMetadata,
		config:           maps.Clone(b.identity.config),
		devices:          make(api.DevicesMap, len(b.identity.devices)),
	}
	for name, device := range b.identity.devices {
		identity.devices[name] = maps.Clone(device)
	}

	return identity, nil
}

// verifyProfiles rejects drift from the effective profiles pinned by preflight.
func (b *Backend) verifyProfiles(ctx context.Context, expected []profileReference) error {
	for _, reference := range expected {
		profile, err := b.client.GetProfile(ctx, reference.Name)
		if err != nil {
			return fmt.Errorf("re-resolve runner profile %q: %w", reference.Name, err)
		}
		digest, err := profileDigest(profile)
		if err != nil {
			return fmt.Errorf("re-digest runner profile %q: %w", reference.Name, err)
		}
		if digest != reference.SHA256 {
			return fmt.Errorf("runner profile %q changed after preflight", reference.Name)
		}
	}

	return nil
}

// ListOwned returns only exact-marker instances and their observed lifecycle state.
func (b *Backend) ListOwned(ctx context.Context) ([]controller.Runner, error) {
	instances, err := b.client.GetInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Incus instances: %w", err)
	}

	owned := make([]api.Instance, 0, len(instances))
	for _, instance := range instances {
		if instance.Config[ownershipKey] != b.options.Owner {
			continue
		}
		owned = append(owned, instance)
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].Name < owned[j].Name })

	runners := make([]controller.Runner, 0, len(owned))
	inspectionErrors := make([]error, 0)
	for index, instance := range owned {
		statusContext, cancel := b.statusReadContext(ctx, len(owned)-index)
		state, err := b.runnerState(statusContext, instance)
		cancel()
		if err != nil {
			inspectionErrors = append(
				inspectionErrors,
				fmt.Errorf("inspect owned instance %q: %w", instance.Name, err),
			)
			continue
		}
		runners = append(runners, controller.Runner{ID: instance.Name, State: state})
	}
	if len(inspectionErrors) != 0 {
		return nil, errors.Join(inspectionErrors...)
	}

	return runners, nil
}

// statusReadContext gives one runner a bounded, fair share of the inventory deadline.
func (b *Backend) statusReadContext(ctx context.Context, remaining int) (context.Context, context.CancelFunc) {
	budget := b.options.StatusReadTimeout
	if deadline, ok := ctx.Deadline(); ok {
		fairShare := time.Until(deadline) / time.Duration(remaining)
		budget = min(budget, fairShare)
	}

	return context.WithTimeout(ctx, budget)
}

// Create creates, starts, and injects one owned runner VM.
func (b *Backend) Create(ctx context.Context) (controller.Runner, error) {
	identity, err := b.pinnedIdentity()
	if err != nil {
		return controller.Runner{}, err
	}
	if profileErr := b.verifyProfiles(ctx, identity.profiles); profileErr != nil {
		return controller.Runner{}, profileErr
	}

	correlationID := b.options.NewID()
	if correlationID == "" || strings.ContainsAny(correlationID, " /\\") {
		return controller.Runner{}, errors.New("generated correlation ID is not a safe instance-name component")
	}
	name := b.options.NamePrefix + correlationID
	if len(name) > maximumInstanceNameLen {
		return controller.Runner{}, fmt.Errorf("generated instance name exceeds %d characters", maximumInstanceNameLen)
	}

	config := maps.Clone(identity.config)
	config[ownershipKey] = b.options.Owner
	config[correlationKey] = correlationID
	config[createdAtKey] = b.options.Now().UTC().Format(time.RFC3339Nano)
	config[imageKey] = identity.imageFingerprint
	config[imageReferenceKey] = b.options.Image
	config[profilesKey] = identity.profileMetadata
	request := api.InstancesPost{
		Name: name,
		Type: api.InstanceTypeVM,
		InstancePut: api.InstancePut{
			Config:   config,
			Devices:  identity.devices,
			Profiles: []string{},
		},
		Source: api.InstanceSource{Type: "image", Fingerprint: identity.imageFingerprint},
	}
	if createErr := b.client.CreateInstance(ctx, request); createErr != nil {
		return controller.Runner{}, fmt.Errorf("create owned instance %q: %w", name, createErr)
	}
	if startErr := b.client.StartInstance(ctx, name); startErr != nil {
		return controller.Runner{}, fmt.Errorf("start owned instance %q: %w", name, startErr)
	}

	payload, err := b.options.Payloads.Payload(ctx, name)
	if err != nil {
		return controller.Runner{}, fmt.Errorf("obtain runtime payload for %q: %w", name, err)
	}
	if payload.Version != 1 || strings.TrimSpace(payload.JITConfig) == "" {
		return controller.Runner{}, fmt.Errorf("runtime payload for %q is invalid", name)
	}
	encoded, err := json.Marshal(map[string]any{
		"version":    payload.Version,
		"jit_config": payload.JITConfig,
	})
	if err != nil {
		return controller.Runner{}, fmt.Errorf("encode runtime payload for %q: %w", name, err)
	}
	if pushErr := b.pushPayload(ctx, name, encoded); pushErr != nil {
		return controller.Runner{}, pushErr
	}

	b.options.Logger.InfoContext(
		ctx,
		"owned Incus runner started",
		"runner_id", name,
		"correlation_id", correlationID,
		"image_fingerprint", identity.imageFingerprint,
		"profiles", identity.profiles,
	)
	return controller.Runner{ID: name, State: controller.RunnerProvisioning}, nil
}

// pushPayload waits for the guest agent and commits the runtime input with the ready marker.
func (b *Backend) pushPayload(ctx context.Context, name string, payload []byte) error {
	ticker := time.NewTicker(b.options.AgentPollInterval)
	defer ticker.Stop()

	payloadWritten := false
	for {
		if !payloadWritten {
			if err := b.client.CreateInstanceFile(ctx, name, payloadPath, payload, payloadFileMode); err == nil {
				payloadWritten = true
			}
		} else if err := b.client.CreateInstanceFile(ctx, name, readyPath, nil, payloadFileMode); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Incus agent on %q: %w", name, ctx.Err())
		case <-ticker.C:
		}
	}
}

// Delete verifies stable identity, collects diagnostics, and removes one instance.
func (b *Backend) Delete(ctx context.Context, runnerID string) error {
	instance, _, err := b.client.GetInstance(ctx, runnerID)
	if errors.Is(err, errNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get instance %q before delete: %w", runnerID, err)
	}
	instanceUUID, err := b.verifyInstanceIdentity(runnerID, instance, "delete candidate")
	if err != nil {
		return err
	}
	if stopErr := b.stopInstance(ctx, runnerID, instanceUUID, instance); stopErr != nil {
		return stopErr
	}
	if diagnosticsErr := b.collectDiagnostics(ctx, runnerID, instanceUUID); diagnosticsErr != nil {
		return diagnosticsErr
	}
	if _, _, verifyErr := b.getVerifiedInstance(ctx, runnerID, instanceUUID, "delete"); verifyErr != nil {
		return verifyErr
	}
	if deleteErr := b.client.DeleteInstance(ctx, runnerID); deleteErr != nil && !errors.Is(deleteErr, errNotFound) {
		return fmt.Errorf("delete owned instance %q: %w", runnerID, deleteErr)
	}
	b.options.Logger.InfoContext(ctx, "owned Incus runner deleted", "runner_id", runnerID)
	return nil
}

// stopInstance conditionally stops the original instance when it is still running.
func (b *Backend) stopInstance(
	ctx context.Context,
	runnerID string,
	instanceUUID string,
	initial *api.Instance,
) error {
	if strings.EqualFold(initial.Status, "stopped") {
		return nil
	}
	current, etag, err := b.getVerifiedInstance(ctx, runnerID, instanceUUID, "stop")
	if err != nil {
		return err
	}
	if strings.EqualFold(current.Status, "stopped") {
		return nil
	}
	if strings.TrimSpace(etag) == "" {
		return fmt.Errorf("refusing to stop Incus instance %q without an ETag", runnerID)
	}
	if stopErr := b.client.StopInstance(ctx, runnerID, etag); stopErr != nil {
		if errors.Is(stopErr, errNotFound) {
			return fmt.Errorf("instance %q disappeared before conditional stop", runnerID)
		}
		return fmt.Errorf("stop owned instance %q before delete: %w", runnerID, stopErr)
	}

	return nil
}

// collectDiagnostics verifies identity and captures best-effort console evidence.
func (b *Backend) collectDiagnostics(ctx context.Context, runnerID string, instanceUUID string) error {
	if _, _, verifyErr := b.getVerifiedInstance(ctx, runnerID, instanceUUID, "diagnostics"); verifyErr != nil {
		return verifyErr
	}
	console, err := b.client.GetInstanceConsoleLog(ctx, runnerID)
	if err != nil {
		if !errors.Is(err, errNotFound) {
			b.options.Logger.WarnContext(
				ctx,
				"failed to collect runner diagnostics",
				"runner_id",
				runnerID,
				"error",
				err,
			)
		}
		return nil
	}
	if storeErr := b.options.Diagnostics.Store(
		ctx,
		Diagnostics{RunnerID: runnerID, Console: console},
	); storeErr != nil {
		b.options.Logger.WarnContext(
			ctx,
			"failed to store runner diagnostics",
			"runner_id",
			runnerID,
			"error",
			storeErr,
		)
	}

	return nil
}

// getVerifiedInstance re-fetches runnerID and confirms the original stable identity.
func (b *Backend) getVerifiedInstance(
	ctx context.Context,
	runnerID string,
	expectedUUID string,
	operation string,
) (*api.Instance, string, error) {
	instance, etag, err := b.client.GetInstance(ctx, runnerID)
	if err != nil {
		return nil, "", fmt.Errorf("get instance %q before %s: %w", runnerID, operation, err)
	}
	instanceUUID, err := b.verifyInstanceIdentity(runnerID, instance, operation)
	if err != nil {
		return nil, "", err
	}
	if instanceUUID != expectedUUID {
		return nil, "", fmt.Errorf("refusing to %s replacement Incus instance %q", operation, runnerID)
	}

	return instance, etag, nil
}

// verifyInstanceIdentity validates owner and the server-generated stable UUID.
func (b *Backend) verifyInstanceIdentity(runnerID string, instance *api.Instance, operation string) (string, error) {
	if instance == nil {
		return "", fmt.Errorf("refusing to %s Incus instance %q without identity", operation, runnerID)
	}
	if instance.Config[ownershipKey] != b.options.Owner {
		return "", fmt.Errorf("refusing to %s unowned Incus instance %q", operation, runnerID)
	}
	instanceUUID := instance.Config[instanceUUIDKey]
	if _, err := uuid.Parse(instanceUUID); err != nil {
		return "", fmt.Errorf("refusing to %s Incus instance %q without a valid stable UUID", operation, runnerID)
	}

	return instanceUUID, nil
}

// runnerState maps Incus and guest status into controller lifecycle state.
func (b *Backend) runnerState(ctx context.Context, instance api.Instance) (controller.RunnerState, error) {
	switch strings.ToLower(instance.Status) {
	case guestStatusStopped, "error":
		return controller.RunnerTerminal, nil
	case "running":
		status, err := b.client.GetInstanceFile(ctx, instance.Name, statusPath)
		if err != nil && !errors.Is(err, errInstanceFileNotFound) {
			return "", fmt.Errorf("read guest status: %w", err)
		}
		if err == nil {
			var observed struct {
				Version int    `json:"version"`
				State   string `json:"state"`
			}
			if err := json.Unmarshal(status, &observed); err != nil {
				return "", fmt.Errorf("decode guest status: %w", err)
			}
			if observed.Version != 1 {
				return "", fmt.Errorf("unsupported guest status version %d", observed.Version)
			}
			switch observed.State {
			case guestStatusStarting:
			case "running":
				return controller.RunnerReady, nil
			case "exited", "failed":
				return controller.RunnerTerminal, nil
			default:
				return "", fmt.Errorf("unknown guest status state %q", observed.State)
			}
		}
	}

	createdAt := instance.CreatedAt
	if raw := instance.Config[createdAtKey]; raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return "", fmt.Errorf("parse ownership creation time: %w", err)
		}
		createdAt = parsed
	}
	if !createdAt.IsZero() && !b.options.Now().Before(createdAt.Add(b.options.BootstrapTimeout)) {
		return controller.RunnerTerminal, nil
	}

	return controller.RunnerProvisioning, nil
}
