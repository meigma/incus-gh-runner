// Package app composes controller ports and supervises their lifetimes.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/controller"
)

const (
	componentCount      = 2
	shutdownWindowCount = 2
)

// ErrShutdownTimeout reports that an application component ignored cancellation.
var ErrShutdownTimeout = errors.New("application shutdown timed out")

// DemandSource publishes current scale-set demand until its context ends.
type DemandSource interface {
	// Run blocks while publishing demand and returns when polling stops.
	Run(ctx context.Context, publish func(controller.Demand)) error
}

// Options contains the application ports and immutable configuration.
type Options struct {
	// Config contains validated runtime settings.
	Config config.Config
	// DemandSource supplies current scale-set demand.
	DemandSource DemandSource
	// RunnerBackend performs owned runner lifecycle operations.
	RunnerBackend controller.Backend
	// Logger receives structured, secret-safe lifecycle events.
	Logger *slog.Logger
}

// Application supervises the demand source and controller core.
type Application struct {
	demandSource   demandSource
	controller     *controller.Controller
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

	return &Application{
		demandSource:   demandSource{source: options.DemandSource, publish: mailbox.Publish},
		controller:     ctrl,
		shutdownBudget: shutdownWindowCount * options.Config.Timeouts.Shutdown,
	}, nil
}

// Run supervises application components until ctx is canceled or one fails.
func (a *Application) Run(ctx context.Context) error {
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan componentResult, componentCount)
	go func() {
		results <- componentResult{name: "demand source", err: a.demandSource.Run(runContext)}
	}()
	go func() {
		results <- componentResult{name: "controller", err: a.controller.Run(runContext)}
	}()

	first := <-results
	cancel()
	second, waitErr := a.waitForPeer(results)
	if waitErr != nil {
		if ctx.Err() != nil {
			return waitErr
		}
		return errors.Join(componentError(first), waitErr)
	}

	if ctx.Err() != nil {
		if err := meaningfulError(first); err != nil {
			return err
		}
		return meaningfulError(second)
	}
	if err := componentError(first); err != nil {
		return err
	}

	return meaningfulError(second)
}

// waitForPeer bounds shutdown across the controller's graceful and cancellation windows.
func (a *Application) waitForPeer(results <-chan componentResult) (componentResult, error) {
	timer := time.NewTimer(a.shutdownBudget)
	defer timer.Stop()

	select {
	case result := <-results:
		return result, nil
	case <-timer.C:
		return componentResult{}, fmt.Errorf("after %s: %w", a.shutdownBudget, ErrShutdownTimeout)
	}
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
