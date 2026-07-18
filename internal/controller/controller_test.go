package controller_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/controller"
)

func TestMailboxCoalescesDemandWithoutBlocking(t *testing.T) {
	t.Parallel()

	mailbox := controller.NewMailbox()
	for assigned := range 1_000 {
		mailbox.Publish(controller.Demand{AssignedJobs: assigned})
	}

	assert.Equal(t, controller.Demand{AssignedJobs: 999}, <-mailbox.Updates())
}

func TestControllerConvergesAfterDemandChangesDuringSlowOperations(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	gate := make(chan struct{})
	backend.createGate = gate
	mailbox := controller.NewMailbox()
	ctrl := newController(t, backend, mailbox, controller.Options{Workers: 2})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runController(ctx, ctrl)

	mailbox.Publish(controller.Demand{AssignedJobs: 5})
	require.Eventually(t, func() bool {
		return backend.concurrentCreates() == 2
	}, time.Second, time.Millisecond, "expected both workers to enter slow creates")

	mailbox.Publish(controller.Demand{AssignedJobs: 1})
	close(gate)
	require.Eventually(t, func() bool {
		return backend.runnerCount() == 1
	}, time.Second, time.Millisecond, "expected latest demand to win after slow creates")
	assert.LessOrEqual(t, backend.maximumConcurrentCreates(), 2)

	cancel()
	require.NoError(t, receiveError(t, errCh))
}

func TestControllerRetriesWorkerFailure(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	backend.failCreates = 1
	mailbox := controller.NewMailbox()
	ctrl := newController(t, backend, mailbox, controller.Options{Workers: 2})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runController(ctx, ctrl)

	mailbox.Publish(controller.Demand{AssignedJobs: 2})
	require.Eventually(t, func() bool {
		return backend.runnerCount() == 2
	}, time.Second, time.Millisecond, "expected periodic reconciliation to retry the failed create")
	assert.GreaterOrEqual(t, backend.createAttempts(), 3)

	cancel()
	require.NoError(t, receiveError(t, errCh))
}

func TestControllerPreservesBusyCapacityWhileScalingDown(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend(
		controller.Runner{ID: "busy", State: controller.RunnerBusy},
		controller.Runner{ID: "idle", State: controller.RunnerIdle},
	)
	mailbox := controller.NewMailbox()
	ctrl := newController(t, backend, mailbox, controller.Options{Workers: 1})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runController(ctx, ctrl)

	require.Eventually(t, func() bool {
		return backend.hasRunner("busy") && !backend.hasRunner("idle")
	}, time.Second, time.Millisecond, "expected only idle excess capacity to be removed")

	cancel()
	require.NoError(t, receiveError(t, errCh))
}

func TestControllerBoundsShutdownOfSlowOperation(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	backend.createGate = make(chan struct{})
	mailbox := controller.NewMailbox()
	ctrl := newController(t, backend, mailbox, controller.Options{
		Workers:         1,
		ShutdownTimeout: 20 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runController(ctx, ctrl)

	mailbox.Publish(controller.Demand{AssignedJobs: 1})
	require.Eventually(t, func() bool {
		return backend.concurrentCreates() == 1
	}, time.Second, time.Millisecond, "expected a create to be in flight")

	started := time.Now()
	cancel()
	require.NoError(t, receiveError(t, errCh))
	assert.Less(t, time.Since(started), 250*time.Millisecond)
	assert.Equal(t, 1, backend.canceledCreates())
}

func newController(
	t *testing.T,
	backend controller.Backend,
	mailbox *controller.Mailbox,
	overrides controller.Options,
) *controller.Controller {
	t.Helper()

	options := controller.Options{
		Backend:           backend,
		Demand:            mailbox.Updates(),
		MinRunners:        0,
		MaxRunners:        10,
		Workers:           2,
		ReconcileInterval: 5 * time.Millisecond,
		OperationTimeout:  time.Second,
		ShutdownTimeout:   50 * time.Millisecond,
	}
	if overrides.Workers != 0 {
		options.Workers = overrides.Workers
	}
	if overrides.ShutdownTimeout != 0 {
		options.ShutdownTimeout = overrides.ShutdownTimeout
	}

	ctrl, err := controller.New(options)
	require.NoError(t, err)
	return ctrl
}

func runController(ctx context.Context, ctrl *controller.Controller) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Run(ctx)
	}()
	return errCh
}

func receiveError(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("controller did not stop")
		return nil
	}
}

type fakeBackend struct {
	mu                    sync.Mutex
	runners               map[string]controller.Runner
	createGate            <-chan struct{}
	failCreates           int
	createSequence        int
	createAttemptCount    int
	concurrentCreateCount int
	maximumCreateCount    int
	canceledCreateCount   int
}

func newFakeBackend(runners ...controller.Runner) *fakeBackend {
	backend := &fakeBackend{runners: make(map[string]controller.Runner, len(runners))}
	for _, runner := range runners {
		backend.runners[runner.ID] = runner
	}
	return backend
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

func (f *fakeBackend) Create(ctx context.Context) (controller.Runner, error) {
	f.mu.Lock()
	f.createAttemptCount++
	if f.failCreates > 0 {
		f.failCreates--
		f.mu.Unlock()
		return controller.Runner{}, errors.New("injected create failure")
	}
	f.concurrentCreateCount++
	f.maximumCreateCount = max(f.maximumCreateCount, f.concurrentCreateCount)
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.concurrentCreateCount--
		f.mu.Unlock()
	}()

	if f.createGate != nil {
		select {
		case <-ctx.Done():
			f.mu.Lock()
			f.canceledCreateCount++
			f.mu.Unlock()
			return controller.Runner{}, ctx.Err()
		case <-f.createGate:
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.createSequence++
	runner := controller.Runner{
		ID:    fmt.Sprintf("runner-%d", f.createSequence),
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

func (f *fakeBackend) hasRunner(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, exists := f.runners[id]
	return exists
}

func (f *fakeBackend) concurrentCreates() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.concurrentCreateCount
}

func (f *fakeBackend) maximumConcurrentCreates() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maximumCreateCount
}

func (f *fakeBackend) createAttempts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createAttemptCount
}

func (f *fakeBackend) canceledCreates() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.canceledCreateCount
}
