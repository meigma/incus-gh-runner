package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Controller reconciles desired capacity through a bounded runner backend.
type Controller struct {
	options Options
}

// New constructs a Controller from validated options.
func New(options Options) (*Controller, error) {
	if options.Backend == nil {
		return nil, errors.New("backend is required")
	}
	if options.Demand == nil {
		return nil, errors.New("demand channel is required")
	}
	if options.MinRunners < 0 {
		return nil, errors.New("minimum runners must not be negative")
	}
	if options.MaxRunners < options.MinRunners {
		return nil, errors.New("maximum runners must be at least minimum runners")
	}
	if options.Workers < 1 {
		return nil, errors.New("workers must be positive")
	}
	if options.ReconcileInterval <= 0 {
		return nil, errors.New("reconcile interval must be positive")
	}
	if options.OperationTimeout <= 0 {
		return nil, errors.New("operation timeout must be positive")
	}
	if options.ShutdownTimeout <= 0 {
		return nil, errors.New("shutdown timeout must be positive")
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &Controller{options: options}, nil
}

// Run inventories owned runners and reconciles until ctx is canceled.
func (c *Controller) Run(ctx context.Context) error {
	runners, err := c.listOwned(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}

		return err
	}

	state, err := newReconcileState(c.options, runners)
	if err != nil {
		return err
	}

	operationBase, cancelOperations := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelOperations()

	work := make(chan operation)
	results := make(chan operationResult, c.options.Workers)
	var workers sync.WaitGroup
	for range c.options.Workers {
		workers.Add(1)
		go c.runWorker(operationBase, &workers, work, results)
	}

	ticker := time.NewTicker(c.options.ReconcileInterval)
	defer ticker.Stop()
	demandUpdates := c.options.Demand

	state.reconcile(work)
	for {
		select {
		case <-ctx.Done():
			close(work)
			return c.waitForWorkers(&workers, cancelOperations)
		case demand, ok := <-demandUpdates:
			if !ok {
				demandUpdates = nil
				continue
			}
			state.setDemand(demand)
			state.reconcile(work)
		case result := <-results:
			if state.apply(result) {
				state.reconcile(work)
			}
		case <-ticker.C:
			state.reconcile(work)
		}
	}
}

func (c *Controller) listOwned(ctx context.Context) ([]Runner, error) {
	operationContext, cancel := context.WithTimeout(ctx, c.options.OperationTimeout)
	defer cancel()

	runners, err := c.options.Backend.ListOwned(operationContext)
	if err != nil {
		return nil, fmt.Errorf("list owned runners: %w", err)
	}

	return runners, nil
}

func (c *Controller) runWorker(
	ctx context.Context,
	workers *sync.WaitGroup,
	work <-chan operation,
	results chan<- operationResult,
) {
	defer workers.Done()

	for item := range work {
		operationContext, cancel := context.WithTimeout(ctx, c.options.OperationTimeout)
		result := operationResult{operation: item}
		switch item.kind {
		case operationCreate:
			result.runner, result.err = c.options.Backend.Create(operationContext)
		case operationDelete:
			result.err = c.options.Backend.Delete(operationContext, item.runnerID)
		}
		cancel()
		results <- result
	}
}

func (c *Controller) waitForWorkers(workers *sync.WaitGroup, cancelOperations context.CancelFunc) error {
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()

	timer := time.NewTimer(c.options.ShutdownTimeout)
	defer timer.Stop()

	select {
	case <-done:
		return nil
	case <-timer.C:
		c.options.Logger.Warn("canceling runner operations after shutdown grace period")
		cancelOperations()
	}

	hardTimer := time.NewTimer(c.options.ShutdownTimeout)
	defer hardTimer.Stop()
	select {
	case <-done:
		return nil
	case <-hardTimer.C:
		return ErrShutdownTimeout
	}
}

type operationKind string

const (
	operationCreate operationKind = "create"
	operationDelete operationKind = "delete"
)

type operation struct {
	id       uint64
	kind     operationKind
	runnerID string
}

type operationResult struct {
	operation operation
	runner    Runner
	err       error
}

type reconcileState struct {
	options    Options
	runners    map[string]Runner
	operations map[uint64]operation
	deleting   map[string]uint64
	target     int
	nextID     uint64
}

func newReconcileState(options Options, runners []Runner) (*reconcileState, error) {
	state := &reconcileState{
		options:    options,
		runners:    make(map[string]Runner, len(runners)),
		operations: make(map[uint64]operation),
		deleting:   make(map[string]uint64),
		target:     options.MinRunners,
	}

	for _, runner := range runners {
		if err := validateRunner(runner); err != nil {
			return nil, fmt.Errorf("inventory owned runner: %w", err)
		}
		state.runners[runner.ID] = runner
	}

	return state, nil
}

func (s *reconcileState) setDemand(demand Demand) {
	assignedJobs := max(demand.AssignedJobs, 0)
	s.target = min(s.options.MaxRunners, s.options.MinRunners+assignedJobs)
	s.options.Logger.Info("runner demand updated", "assigned_jobs", assignedJobs, "target", s.target)
}

func (s *reconcileState) reconcile(work chan<- operation) {
	for id, runner := range s.runners {
		if runner.State == RunnerTerminal {
			s.trySchedule(work, operation{kind: operationDelete, runnerID: id})
		}
	}

	live := s.liveCapacity()
	for live < s.target {
		if !s.trySchedule(work, operation{kind: operationCreate}) {
			break
		}
		live++
	}

	for live > s.target {
		id, ok := s.idleRunner()
		if !ok || !s.trySchedule(work, operation{kind: operationDelete, runnerID: id}) {
			break
		}
		live--
	}
}

func (s *reconcileState) trySchedule(work chan<- operation, item operation) bool {
	if item.runnerID != "" {
		if _, exists := s.deleting[item.runnerID]; exists {
			return false
		}
	}

	s.nextID++
	item.id = s.nextID
	select {
	case work <- item:
		s.operations[item.id] = item
		if item.runnerID != "" {
			s.deleting[item.runnerID] = item.id
		}
		s.options.Logger.Info(
			"runner operation scheduled",
			"operation", item.kind,
			"operation_id", item.id,
			"runner_id", item.runnerID,
		)
		return true
	default:
		return false
	}
}

func (s *reconcileState) apply(result operationResult) bool {
	item, exists := s.operations[result.operation.id]
	if !exists {
		return false
	}
	delete(s.operations, item.id)
	if item.runnerID != "" {
		delete(s.deleting, item.runnerID)
	}

	if result.err != nil {
		s.options.Logger.Warn(
			"runner operation failed",
			"operation", item.kind,
			"operation_id", item.id,
			"runner_id", item.runnerID,
			"error", result.err,
		)
		return false
	}

	switch item.kind {
	case operationCreate:
		if err := validateRunner(result.runner); err != nil {
			s.options.Logger.Warn(
				"runner create returned invalid state",
				"operation_id", item.id,
				"error", err,
			)
			return false
		}
		s.runners[result.runner.ID] = result.runner
	case operationDelete:
		delete(s.runners, item.runnerID)
	}

	runnerID := item.runnerID
	if item.kind == operationCreate {
		runnerID = result.runner.ID
	}
	s.options.Logger.Info(
		"runner operation completed",
		"operation", item.kind,
		"operation_id", item.id,
		"runner_id", runnerID,
	)
	return true
}

func (s *reconcileState) liveCapacity() int {
	live := 0
	for id, runner := range s.runners {
		if _, deleting := s.deleting[id]; deleting {
			continue
		}
		if runner.State != RunnerTerminal {
			live++
		}
	}
	for _, item := range s.operations {
		if item.kind == operationCreate {
			live++
		}
	}

	return live
}

func (s *reconcileState) idleRunner() (string, bool) {
	for id, runner := range s.runners {
		if runner.State != RunnerIdle {
			continue
		}
		if _, deleting := s.deleting[id]; !deleting {
			return id, true
		}
	}

	return "", false
}

func validateRunner(runner Runner) error {
	if runner.ID == "" {
		return errors.New("runner ID is required")
	}
	switch runner.State {
	case RunnerProvisioning, RunnerIdle, RunnerBusy, RunnerTerminal:
		return nil
	default:
		return fmt.Errorf("runner %q has unknown state %q", runner.ID, runner.State)
	}
}
