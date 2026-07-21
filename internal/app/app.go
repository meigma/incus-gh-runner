// Package app composes controller ports and supervises their lifetimes.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/controller"
)

const shutdownWindowCount = 2

// ErrShutdownTimeout reports that an application component ignored cancellation.
var ErrShutdownTimeout = errors.New("application shutdown timed out")

// DemandSource publishes current scale-set demand until its context ends.
type DemandSource interface {
	// Run blocks while publishing demand and returns when polling stops.
	Run(ctx context.Context, publish func(controller.Demand)) error
}

// Runnable is one independently supervised application component.
type Runnable interface {
	// Run blocks until cancellation or component failure.
	Run(ctx context.Context) error
}

// Component names one additional supervised runtime component.
type Component struct {
	// Name identifies component failures in operator-visible errors.
	Name string
	// Runner owns the component's blocking lifecycle.
	Runner Runnable
}

// Options contains the application ports and immutable configuration.
type Options struct {
	// Config contains validated runtime settings.
	Config config.Config
	// DemandSource supplies current scale-set demand.
	DemandSource DemandSource
	// RunnerBackend performs owned runner lifecycle operations.
	RunnerBackend controller.Backend
	// RunnerFencer removes GitHub registration before idle scale-down.
	RunnerFencer controller.Fencer
	// Components are additional independently supervised runtime services.
	Components []Component
	// Logger receives structured, secret-safe lifecycle events.
	Logger *slog.Logger
}

// Application supervises the demand source and controller core.
type Application struct {
	components     []Component
	shutdownBudget time.Duration
}

// New constructs an Application from its ports.
func New(options Options) (*Application, error) {
	if err := options.Config.Validate(); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}
	if options.DemandSource == nil {
		return nil, errors.New("demand source is required")
	}

	mailbox := controller.NewMailbox()
	ctrl, err := controller.New(controller.Options{
		Backend:           options.RunnerBackend,
		Fencer:            options.RunnerFencer,
		Demand:            mailbox.Updates(),
		Logger:            options.Logger,
		MinRunners:        options.Config.Capacity.MinRunners,
		MaxRunners:        options.Config.Capacity.MaxRunners,
		Workers:           options.Config.Concurrency.IncusOperations,
		ReconcileInterval: options.Config.ReconcileInterval,
		OperationTimeout:  options.Config.Timeouts.IncusOperation,
		RetryInitial:      options.Config.Retry.Initial,
		RetryMaximum:      options.Config.Retry.Maximum,
		ShutdownTimeout:   options.Config.Timeouts.Shutdown,
	})
	if err != nil {
		return nil, fmt.Errorf("construct controller: %w", err)
	}

	components := []Component{
		{Name: "demand source", Runner: demandSource{source: options.DemandSource, publish: mailbox.Publish}},
		{Name: "controller", Runner: ctrl},
	}
	names := map[string]struct{}{"demand source": {}, "controller": {}}
	for _, component := range options.Components {
		if strings.TrimSpace(component.Name) == "" {
			return nil, errors.New("component name is required")
		}
		if component.Runner == nil {
			return nil, fmt.Errorf("component %q runner is required", component.Name)
		}
		if _, exists := names[component.Name]; exists {
			return nil, fmt.Errorf("component name %q is duplicated", component.Name)
		}
		names[component.Name] = struct{}{}
		components = append(components, component)
	}

	return &Application{
		components:     components,
		shutdownBudget: shutdownWindowCount * options.Config.Timeouts.Shutdown,
	}, nil
}

// Run supervises application components until ctx is canceled or one fails.
func (a *Application) Run(ctx context.Context) error {
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan componentResult, len(a.components))
	for _, component := range a.components {
		go func(component Component) {
			results <- componentResult{name: component.Name, err: component.Runner.Run(runContext)}
		}(component)
	}

	first := <-results
	cancel()
	peers, waitErr := a.waitForPeers(results, len(a.components)-1)
	if waitErr != nil {
		if ctx.Err() != nil {
			return waitErr
		}
		return errors.Join(componentError(first), waitErr)
	}

	if ctx.Err() != nil {
		return joinMeaningful(first, peers)
	}
	joined := []error{componentError(first)}
	for _, peer := range peers {
		joined = append(joined, meaningfulError(peer))
	}

	return errors.Join(joined...)
}

// waitForPeers bounds shutdown across all remaining supervised components.
func (a *Application) waitForPeers(results <-chan componentResult, count int) ([]componentResult, error) {
	timer := time.NewTimer(a.shutdownBudget)
	defer timer.Stop()

	peers := make([]componentResult, 0, count)
	for range count {
		select {
		case result := <-results:
			peers = append(peers, result)
		case <-timer.C:
			return nil, fmt.Errorf("after %s: %w", a.shutdownBudget, ErrShutdownTimeout)
		}
	}

	return peers, nil
}

// demandSource binds a source port to the controller's publish callback.
type demandSource struct {
	source  DemandSource
	publish func(controller.Demand)
}

// Run polls the source and publishes its updates to the bound callback.
func (s demandSource) Run(ctx context.Context) error {
	return s.source.Run(ctx, s.publish)
}

// componentResult identifies the outcome of one supervised component.
type componentResult struct {
	name string
	err  error
}

// componentError treats an unsolicited component stop as an application failure.
func componentError(result componentResult) error {
	if result.err == nil {
		return fmt.Errorf("%s stopped unexpectedly", result.name)
	}
	if errors.Is(result.err, context.Canceled) {
		return fmt.Errorf("%s stopped unexpectedly: %w", result.name, result.err)
	}

	return fmt.Errorf("%s: %w", result.name, result.err)
}

// meaningfulError filters normal cancellation from a component result.
func meaningfulError(result componentResult) error {
	if result.err == nil || errors.Is(result.err, context.Canceled) {
		return nil
	}

	return fmt.Errorf("%s: %w", result.name, result.err)
}

// joinMeaningful returns only non-cancellation component failures during requested shutdown.
func joinMeaningful(first componentResult, peers []componentResult) error {
	errs := []error{meaningfulError(first)}
	for _, peer := range peers {
		errs = append(errs, meaningfulError(peer))
	}

	return errors.Join(errs...)
}
