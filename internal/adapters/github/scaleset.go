package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"

	"github.com/meigma/incus-gh-runner/internal/controller"
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

	runnerGroupID := defaultRunnerGroupID
	if options.RunnerGroup != scaleset.DefaultRunnerGroup {
		runnerGroup, err := client.GetRunnerGroupByName(ctx, options.RunnerGroup)
		if err != nil {
			return nil, fmt.Errorf("resolve runner group %q: %w", options.RunnerGroup, err)
		}
		if runnerGroup == nil || runnerGroup.ID == 0 {
			return nil, fmt.Errorf("resolve runner group %q: response has no ID", options.RunnerGroup)
		}
		runnerGroupID = runnerGroup.ID
	}

	resolved, err := client.GetRunnerScaleSet(ctx, runnerGroupID, options.Name)
	if err != nil {
		return nil, fmt.Errorf("resolve runner scale set %q: %w", options.Name, err)
	}
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
		options.Logger.InfoContext(ctx, "GitHub runner scale set created", "scale_set", options.Name)
	} else {
		options.Logger.InfoContext(ctx, "GitHub runner scale set resolved", "scale_set", options.Name)
	}
	if resolved == nil || resolved.ID == 0 {
		return nil, fmt.Errorf("runner scale set %q response has no ID", options.Name)
	}

	options.SystemInfo.ScaleSetID = resolved.ID
	client.SetSystemInfo(options.SystemInfo)
	return &ScaleSet{client: client, id: resolved.ID}, nil
}

// ID returns the resolved runner scale-set ID.
func (s *ScaleSet) ID() int {
	return s.id
}

// JITConfig generates a fresh opaque one-runner registration configuration.
func (s *ScaleSet) JITConfig(ctx context.Context, runnerName string) (string, error) {
	if strings.TrimSpace(runnerName) == "" {
		return "", errors.New("runner name is required")
	}
	jit, err := s.client.GenerateJitRunnerConfig(ctx, &scaleset.RunnerScaleSetJitRunnerSetting{
		Name:       runnerName,
		WorkFolder: defaultWorkFolder,
	}, s.id)
	if err != nil {
		return "", fmt.Errorf("generate JIT configuration for %q: %w", runnerName, err)
	}
	if jit == nil || strings.TrimSpace(jit.EncodedJITConfig) == "" {
		return "", fmt.Errorf("generate JIT configuration for %q: response is empty", runnerName)
	}

	return jit.EncodedJITConfig, nil
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
	SetSystemInfo(info scaleset.SystemInfo)
}

// DemandSourceOptions configures message polling and demand publication.
type DemandSourceOptions struct {
	// ScaleSetID is the resolved runner scale-set ID.
	ScaleSetID int
	// MinRunners is the idle runner floor used for desired-count reporting.
	MinRunners int
	// MaxRunners is the maximum capacity reported while polling.
	MaxRunners int
	// Logger receives secret-safe message and job lifecycle events.
	Logger *slog.Logger
}

// DemandSource publishes coalescible demand from one GitHub message session.
type DemandSource struct {
	listener demandListener
	options  DemandSourceOptions
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
	handler := demandHandler{options: s.options, publish: publish}
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
	options DemandSourceOptions
	publish func(controller.Demand)
}

// HandleDesiredRunnerCount publishes current assigned jobs and reports the bounded target.
func (h *demandHandler) HandleDesiredRunnerCount(_ context.Context, count int) (int, error) {
	assignedJobs := max(count, 0)
	h.publish(controller.Demand{AssignedJobs: assignedJobs})
	return min(h.options.MaxRunners, h.options.MinRunners+assignedJobs), nil
}

// HandleJobStarted records a secret-safe lifecycle event.
func (h *demandHandler) HandleJobStarted(ctx context.Context, job *scaleset.JobStarted) error {
	if job != nil {
		h.options.Logger.InfoContext(
			ctx,
			"GitHub Actions job started",
			"runner_name",
			job.RunnerName,
			"job_id",
			job.JobID,
		)
	}
	return nil
}

// HandleJobCompleted records a secret-safe lifecycle event.
func (h *demandHandler) HandleJobCompleted(ctx context.Context, job *scaleset.JobCompleted) error {
	if job != nil {
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
