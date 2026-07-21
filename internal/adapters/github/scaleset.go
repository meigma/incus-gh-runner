package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"

	"github.com/meigma/incus-gh-runner/internal/controller"
	"github.com/meigma/incus-gh-runner/internal/provenance"
)

const (
	defaultRunnerGroupID = 1
	defaultWorkFolder    = "_work"
)

// ScaleSetOptions configures persistent scale-set resolution.
type ScaleSetOptions struct {
	// Name is the persistent runner scale-set name and default runner label.
	Name string
	// RunnerGroup is the existing runner group containing the scale set.
	RunnerGroup string
	// SystemInfo identifies this controller in upstream client requests.
	SystemInfo scaleset.SystemInfo
	// Logger receives secret-safe scale-set lifecycle events.
	Logger *slog.Logger
}

// ScaleSet represents one resolved persistent GitHub runner scale set.
type ScaleSet struct {
	client scaleSetClient
	id     int
}

// JITConfig contains one validated runner registration and its opaque guest payload.
type JITConfig struct {
	// Encoded is the opaque one-runner configuration consumed by the GitHub Actions runner.
	Encoded string
	// RunnerID is the positive GitHub runner registration identifier.
	RunnerID int
	// RunnerName is the exact controller-requested runner name.
	RunnerName string
	// ScaleSetID is the resolved runner scale-set identifier.
	ScaleSetID int
}

// ResolveScaleSet resolves an existing scale set or creates it when absent.
func ResolveScaleSet(
	ctx context.Context,
	client *scaleset.Client,
	options ScaleSetOptions,
) (*ScaleSet, error) {
	return resolveScaleSet(ctx, client, options)
}

// resolveScaleSet performs persistent scale-set resolution through the narrow test seam.
func resolveScaleSet(
	ctx context.Context,
	client scaleSetClient,
	options ScaleSetOptions,
) (*ScaleSet, error) {
	if client == nil {
		return nil, errors.New("scale-set client is required")
	}
	if strings.TrimSpace(options.Name) == "" {
		return nil, errors.New("scale-set name is required")
	}
	if strings.TrimSpace(options.RunnerGroup) == "" {
		return nil, errors.New("runner group is required")
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}

	runnerGroupID, err := resolveRunnerGroupID(ctx, client, options.RunnerGroup)
	if err != nil {
		return nil, err
	}

	resolved, err := client.GetRunnerScaleSet(ctx, runnerGroupID, options.Name)
	if err != nil {
		return nil, fmt.Errorf("resolve runner scale set %q: %w", options.Name, err)
	}
	created := false
	if resolved == nil {
		resolved, err = client.CreateRunnerScaleSet(ctx, &scaleset.RunnerScaleSet{
			Name:          options.Name,
			RunnerGroupID: runnerGroupID,
			Labels:        []scaleset.Label{{Name: options.Name}},
			RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		})
		if err != nil {
			return nil, fmt.Errorf("create runner scale set %q: %w", options.Name, err)
		}
		created = true
	}
	if err := validateResolvedScaleSet(resolved, runnerGroupID, options.RunnerGroup, options.Name); err != nil {
		return nil, err
	}
	if created {
		options.Logger.InfoContext(ctx, "GitHub runner scale set created", "scale_set", options.Name)
	} else {
		options.Logger.InfoContext(ctx, "GitHub runner scale set resolved", "scale_set", options.Name)
	}

	options.SystemInfo.ScaleSetID = resolved.ID
	client.SetSystemInfo(options.SystemInfo)
	return &ScaleSet{client: client, id: resolved.ID}, nil
}

// resolveRunnerGroupID resolves a named non-default group without trusting a redirected response.
func resolveRunnerGroupID(ctx context.Context, client scaleSetClient, name string) (int, error) {
	if name == scaleset.DefaultRunnerGroup {
		return defaultRunnerGroupID, nil
	}
	runnerGroup, err := client.GetRunnerGroupByName(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("resolve runner group %q: %w", name, err)
	}
	if runnerGroup == nil || runnerGroup.ID <= 0 {
		return 0, fmt.Errorf("resolve runner group %q: response has no ID", name)
	}
	if runnerGroup.ID == defaultRunnerGroupID || runnerGroup.IsDefault {
		return 0, fmt.Errorf("resolve runner group %q: response identifies the default group", name)
	}
	if runnerGroup.Name != name {
		return 0, fmt.Errorf("resolve runner group %q: response name does not match", name)
	}
	return runnerGroup.ID, nil
}

// validateResolvedScaleSet checks the required identity and any optional group identity in the API response.
func validateResolvedScaleSet(
	resolved *scaleset.RunnerScaleSet,
	runnerGroupID int,
	runnerGroupName string,
	name string,
) error {
	if resolved == nil || resolved.ID <= 0 {
		return fmt.Errorf("runner scale set %q response has no ID", name)
	}
	if resolved.Name != name {
		return fmt.Errorf("runner scale set %q response name does not match", name)
	}
	if resolved.RunnerGroupID != 0 && resolved.RunnerGroupID != runnerGroupID {
		return fmt.Errorf("runner scale set %q response runner group does not match", name)
	}
	if resolved.RunnerGroupName != "" && !runnerGroupNameMatches(runnerGroupName, resolved.RunnerGroupName) {
		return fmt.Errorf("runner scale set %q response runner group name does not match", name)
	}
	if len(resolved.Labels) != 1 || resolved.Labels[0].Name != name {
		return fmt.Errorf("runner scale set %q response labels do not match", name)
	}
	if !resolved.RunnerSetting.DisableUpdate {
		return fmt.Errorf("runner scale set %q does not disable runner self-update", name)
	}
	return nil
}

// runnerGroupNameMatches permits GitHub's display capitalization only for the built-in default group.
func runnerGroupNameMatches(expected string, actual string) bool {
	if expected == scaleset.DefaultRunnerGroup {
		return strings.EqualFold(expected, actual)
	}
	return expected == actual
}

// ID returns the resolved runner scale-set ID.
func (s *ScaleSet) ID() int {
	return s.id
}

// JITConfig generates and validates one fresh runner registration configuration.
func (s *ScaleSet) JITConfig(ctx context.Context, runnerName string) (JITConfig, error) {
	if strings.TrimSpace(runnerName) == "" {
		return JITConfig{}, errors.New("runner name is required")
	}
	jit, err := s.client.GenerateJitRunnerConfig(ctx, &scaleset.RunnerScaleSetJitRunnerSetting{
		Name:       runnerName,
		WorkFolder: defaultWorkFolder,
	}, s.id)
	if err != nil {
		return JITConfig{}, fmt.Errorf("generate JIT configuration for %q: %w", runnerName, err)
	}
	if jit == nil || strings.TrimSpace(jit.EncodedJITConfig) == "" || jit.Runner == nil {
		return JITConfig{}, fmt.Errorf("generate JIT configuration for %q: response is incomplete", runnerName)
	}
	if jit.Runner.ID <= 0 {
		return JITConfig{}, fmt.Errorf("generate JIT configuration for %q: runner response has no ID", runnerName)
	}
	if jit.Runner.Name != runnerName {
		return JITConfig{}, fmt.Errorf(
			"generate JIT configuration for %q: runner response name does not match",
			runnerName,
		)
	}
	if jit.Runner.RunnerScaleSetID != s.id {
		return JITConfig{}, fmt.Errorf(
			"generate JIT configuration for %q: runner response scale set does not match",
			runnerName,
		)
	}

	return JITConfig{
		Encoded:    jit.EncodedJITConfig,
		RunnerID:   jit.Runner.ID,
		RunnerName: jit.Runner.Name,
		ScaleSetID: jit.Runner.RunnerScaleSetID,
	}, nil
}

// Fence removes runnerID from the resolved scale set and confirms its registration is absent.
func (s *ScaleSet) Fence(ctx context.Context, runnerID string) error {
	if strings.TrimSpace(runnerID) == "" {
		return errors.New("runner ID is required")
	}
	runner, err := s.client.GetRunnerByName(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("resolve runner %q before fence: %w", runnerID, err)
	}
	if runner == nil {
		return nil
	}
	if runner.ID <= 0 || runner.Name != runnerID || runner.RunnerScaleSetID != s.id {
		return fmt.Errorf("runner %q registration does not match the resolved scale set", runnerID)
	}
	if removeErr := s.client.RemoveRunner(ctx, int64(runner.ID)); removeErr != nil {
		return fmt.Errorf("remove runner %q registration: %w", runnerID, removeErr)
	}
	remaining, err := s.client.GetRunnerByName(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("confirm runner %q registration fence: %w", runnerID, err)
	}
	if remaining != nil {
		return fmt.Errorf("confirm runner %q registration fence: registration still exists", runnerID)
	}

	return nil
}

// scaleSetClient is the upstream client surface used by persistent resolution and JIT generation.
type scaleSetClient interface {
	GetRunnerGroupByName(ctx context.Context, runnerGroup string) (*scaleset.RunnerGroup, error)
	GetRunnerScaleSet(
		ctx context.Context,
		runnerGroupID int,
		runnerScaleSetName string,
	) (*scaleset.RunnerScaleSet, error)
	CreateRunnerScaleSet(ctx context.Context, runnerScaleSet *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error)
	GenerateJitRunnerConfig(
		ctx context.Context,
		jitRunnerSetting *scaleset.RunnerScaleSetJitRunnerSetting,
		scaleSetID int,
	) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
	GetRunnerByName(ctx context.Context, runnerName string) (*scaleset.RunnerReference, error)
	RemoveRunner(ctx context.Context, runnerID int64) error
	SetSystemInfo(info scaleset.SystemInfo)
}

// DemandSourceOptions configures message polling and demand publication.
type DemandSourceOptions struct {
	// ScaleSetID is the resolved runner scale-set ID.
	ScaleSetID int
	// ScaleSetName is the controller-resolved scale-set name used in proof events.
	ScaleSetName string
	// MinRunners is the idle runner floor used for desired-count reporting.
	MinRunners int
	// MaxRunners is the maximum capacity reported while polling.
	MaxRunners int
	// Logger receives secret-safe message and job lifecycle events.
	Logger *slog.Logger
	// ReconnectInitial is the first delay after a failed GitHub message session.
	ReconnectInitial time.Duration
	// ReconnectMaximum caps the delay between message-session recreation attempts.
	ReconnectMaximum time.Duration
	// SessionCloseTimeout bounds cleanup of each replaced GitHub message session.
	SessionCloseTimeout time.Duration
	// JobStartedSink receives validated proof events without blocking callbacks when configured.
	JobStartedSink provenance.JobStartedSink
}

// DemandSource publishes coalescible demand from one GitHub message session.
type DemandSource struct {
	listener  demandListener
	options   DemandSourceOptions
	onContact func()
}

// NewDemandSource constructs a demand source from an active message session.
func NewDemandSource(session listener.Client, options DemandSourceOptions) (*DemandSource, error) {
	upstream, err := listener.New(session, listener.Config{
		ScaleSetID: options.ScaleSetID,
		MaxRunners: options.MaxRunners,
		Logger:     options.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("construct scale-set listener: %w", err)
	}

	return newDemandSource(upstream, options)
}

// newDemandSource constructs a demand source around the narrow listener test seam.
func newDemandSource(upstream demandListener, options DemandSourceOptions) (*DemandSource, error) {
	if upstream == nil {
		return nil, errors.New("scale-set listener is required")
	}
	if options.MinRunners < 0 {
		return nil, errors.New("minimum runners must not be negative")
	}
	if options.MaxRunners < options.MinRunners {
		return nil, errors.New("maximum runners must be at least minimum runners")
	}
	if options.JobStartedSink != nil {
		if options.ScaleSetID <= 0 {
			return nil, errors.New("proof-enabled demand source requires a positive scale-set ID")
		}
		if strings.TrimSpace(options.ScaleSetName) == "" {
			return nil, errors.New("proof-enabled demand source requires a scale-set name")
		}
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}

	return &DemandSource{listener: upstream, options: options}, nil
}

// Run polls GitHub and publishes the latest assigned-job count without blocking callbacks.
func (s *DemandSource) Run(ctx context.Context, publish func(controller.Demand)) error {
	if publish == nil {
		return errors.New("demand publisher is required")
	}
	handler := demandHandler{
		options:   s.options,
		publish:   publish,
		onContact: s.onContact,
		busy:      make(map[string]struct{}),
	}
	if err := s.listener.Run(ctx, &handler); err != nil {
		return fmt.Errorf("run scale-set listener: %w", err)
	}

	return nil
}

// demandListener runs the upstream message loop against its callback handler.
type demandListener interface {
	Run(ctx context.Context, scaler listener.Scaler) error
}

// demandHandler translates synchronous scale-set callbacks into non-blocking demand publication.
type demandHandler struct {
	options   DemandSourceOptions
	publish   func(controller.Demand)
	onContact func()
	busy      map[string]struct{}
}

// HandleDesiredRunnerCount publishes current assigned jobs and reports the bounded target.
func (h *demandHandler) HandleDesiredRunnerCount(_ context.Context, count int) (int, error) {
	if h.onContact != nil {
		h.onContact()
	}
	assignedJobs := max(count, 0)
	busyRunners := make([]string, 0, len(h.busy))
	for runnerID := range h.busy {
		busyRunners = append(busyRunners, runnerID)
	}
	sort.Strings(busyRunners)
	h.publish(controller.Demand{AssignedJobs: assignedJobs, BusyRunners: busyRunners})
	return min(h.options.MaxRunners, h.options.MinRunners+assignedJobs), nil
}

// HandleJobStarted records a secret-safe lifecycle event.
func (h *demandHandler) HandleJobStarted(ctx context.Context, job *scaleset.JobStarted) error {
	if job != nil {
		if job.RunnerName != "" {
			h.busy[job.RunnerName] = struct{}{}
		}
		h.options.Logger.InfoContext(
			ctx,
			"GitHub Actions job started",
			"runner_name",
			job.RunnerName,
			"job_id",
			job.JobID,
		)
		if h.options.JobStartedSink != nil {
			event := provenance.JobStarted{
				Owner:           job.OwnerName,
				Repository:      job.RepositoryName,
				WorkflowRef:     job.JobWorkflowRef,
				WorkflowRunID:   job.WorkflowRunID,
				JobID:           job.JobID,
				RunnerRequestID: job.RunnerRequestID,
				RunnerID:        int64(job.RunnerID),
				RunnerName:      job.RunnerName,
				EventName:       job.EventName,
				ScaleSetID:      int64(h.options.ScaleSetID),
				ScaleSetName:    h.options.ScaleSetName,
			}
			if err := h.options.JobStartedSink.TryEnqueue(event); err != nil {
				h.options.Logger.ErrorContext(
					ctx,
					"GitHub Actions job proof event dropped",
					"job_id", job.JobID,
					"runner_id", job.RunnerID,
					"runner_name", job.RunnerName,
					"error", err,
				)
			}
		}
	}
	return nil
}

// HandleJobCompleted records a secret-safe lifecycle event.
func (h *demandHandler) HandleJobCompleted(ctx context.Context, job *scaleset.JobCompleted) error {
	if job != nil {
		if job.RunnerName != "" {
			delete(h.busy, job.RunnerName)
		}
		h.options.Logger.InfoContext(
			ctx,
			"GitHub Actions job completed",
			"runner_name", job.RunnerName,
			"job_id", job.JobID,
			"result", job.Result,
		)
	}
	return nil
}
