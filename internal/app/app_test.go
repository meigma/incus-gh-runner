package app_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/app"
	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/controller"
)

func TestApplicationRunsFakeDemandToConvergence(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	application := newApplication(t, app.Options{
		DemandSource: demandSourceFunc(func(ctx context.Context, publish func(controller.Demand)) error {
			publish(controller.Demand{AssignedJobs: 3})
			<-ctx.Done()
			return ctx.Err()
		}),
		RunnerBackend: backend,
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- application.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		return backend.runnerCount() == 3
	}, time.Second, time.Millisecond, "expected fake demand to converge")
	cancel()

	require.NoError(t, receiveError(t, errCh))
}

func TestApplicationPropagatesDemandSourceFailure(t *testing.T) {
	t.Parallel()

	application := newApplication(t, app.Options{
		DemandSource: demandSourceFunc(func(context.Context, func(controller.Demand)) error {
			return errors.New("poll failed")
		}),
		RunnerBackend: newFakeBackend(),
	})

	err := application.Run(context.Background())

	assert.EqualError(t, err, "demand source: poll failed")
}

func newApplication(t *testing.T, options app.Options) *app.Application {
	t.Helper()

	options.Config = config.Config{
		Capacity: config.Capacity{MinRunners: 0, MaxRunners: 4},
		Concurrency: config.Concurrency{
			IncusOperations: 2,
		},
		ReconcileInterval: time.Millisecond,
		Timeouts: config.Timeouts{
			IncusOperation: time.Second,
			Shutdown:       50 * time.Millisecond,
		},
	}
	application, err := app.New(options)
	require.NoError(t, err)
	return application
}

func receiveError(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("application did not stop")
		return nil
	}
}

type demandSourceFunc func(context.Context, func(controller.Demand)) error

func (f demandSourceFunc) Run(ctx context.Context, publish func(controller.Demand)) error {
	return f(ctx, publish)
}

type fakeBackend struct {
	mu       sync.Mutex
	runners  map[string]controller.Runner
	sequence int
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{runners: make(map[string]controller.Runner)}
}

func (f *fakeBackend) ListOwned(context.Context) ([]controller.Runner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	runners := make([]controller.Runner, 0, len(f.runners))
	for _, runner := range f.runners {
		runners = append(runners, runner)
	}
	return runners, nil
}

func (f *fakeBackend) Create(context.Context) (controller.Runner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sequence++
	runner := controller.Runner{
		ID:    fmt.Sprintf("runner-%d", f.sequence),
		State: controller.RunnerIdle,
	}
	f.runners[runner.ID] = runner
	return runner, nil
}

func (f *fakeBackend) Delete(_ context.Context, runnerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.runners, runnerID)
	return nil
}

func (f *fakeBackend) runnerCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runners)
}
