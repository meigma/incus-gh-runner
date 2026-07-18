package controller

import (
	"context"
	"errors"
	"fmt"
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
	if options.RetryInitial <= 0 {
		return nil, errors.New("retry initial must be positive")
	}
	if options.RetryMaximum < options.RetryInitial {
		return nil, errors.New("retry maximum must be at least retry initial")
	}
	if options.ShutdownTimeout <= 0 {
		return nil, errors.New("shutdown timeout must be positive")
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
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
			state.refresh(work)
			state.reconcile(work)
		}
	}
}

// listOwned retrieves the initial owned-runner inventory within the operation timeout.
func (c *Controller) listOwned(ctx context.Context) ([]Runner, error) {
	operationContext, cancel := context.WithTimeout(ctx, c.options.OperationTimeout)
	defer cancel()

	runners, err := c.options.Backend.ListOwned(operationContext)
	if err != nil {
		return nil, fmt.Errorf("list owned runners: %w", err)
	}

	return runners, nil
}

// runWorker executes backend operations until the work channel closes.
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
		case operationList:
			result.runners, result.err = c.options.Backend.ListOwned(operationContext)
		case operationCreate:
			result.runner, result.err = c.options.Backend.Create(operationContext)
		case operationDelete:
			result.err = c.options.Backend.Delete(operationContext, item.runnerID)
		}
		cancel()
		results <- result
	}
}

// waitForWorkers gives active operations a grace period before canceling them.
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

// operationKind identifies a backend lifecycle action.
type operationKind string

const (
	operationList   operationKind = "list"
	operationCreate operationKind = "create"
	operationDelete operationKind = "delete"
)

// operation describes one scheduled backend lifecycle action.
type operation struct {
	id       uint64
	kind     operationKind
	runnerID string
}

// operationResult carries a backend lifecycle outcome to the reconciler.
type operationResult struct {
	operation operation
	runners   []Runner
	runner    Runner
	err       error
}

// failureKey identifies one independently retryable backend operation target.
type failureKey struct {
	kind     operationKind
	runnerID string
}

// failureBackoff records the current delay and earliest allowed retry time.
type failureBackoff struct {
	delay   time.Duration
	retryAt time.Time
}

// reconcileState is the single owner of desired and observed runner capacity.
type reconcileState struct {
	options        Options
	runners        map[string]Runner
	operations     map[uint64]operation
	deleting       map[string]uint64
	failures       map[failureKey]failureBackoff
	inventoryStale bool
	target         int
	nextID         uint64
}

// newReconcileState validates inventory and creates the initial reconciliation state.
func newReconcileState(options Options, runners []Runner) (*reconcileState, error) {
	state := &reconcileState{
		options:    options,
		runners:    make(map[string]Runner, len(runners)),
		operations: make(map[uint64]operation),
		deleting:   make(map[string]uint64),
		failures:   make(map[failureKey]failureBackoff),
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

// setDemand derives the bounded capacity target from the latest demand.
func (s *reconcileState) setDemand(demand Demand) {
	assignedJobs := max(demand.AssignedJobs, 0)
	s.target = min(s.options.MaxRunners, s.options.MinRunners+assignedJobs)
	s.options.Logger.Info("runner demand updated", "assigned_jobs", assignedJobs, "target", s.target)
}

// refresh schedules a new owned-runner inventory when no lifecycle operation is in flight.
func (s *reconcileState) refresh(work chan<- operation) {
	if len(s.operations) != 0 {
		return
	}

	s.trySchedule(work, operation{kind: operationList})
}

// reconcile schedules the operations needed to move current capacity toward the target.
func (s *reconcileState) reconcile(work chan<- operation) {
	if s.inventoryStale || s.inventorying() {
		return
	}

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

// trySchedule records item only when a worker can accept it immediately.
func (s *reconcileState) trySchedule(work chan<- operation, item operation) bool {
	if s.retryBlocked(item, time.Now()) {
		return false
	}
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

// apply consumes a result once and updates observed runner state.
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
		s.recordFailure(item, result.err)
		return false
	}

	switch item.kind {
	case operationList:
		inventory, err := validatedInventory(result.runners)
		if err != nil {
			s.recordFailure(item, err)
			return false
		}
		s.runners = inventory
		s.inventoryStale = false
	case operationCreate:
		if err := validateRunner(result.runner); err != nil {
			s.recordFailure(item, err)
			return false
		}
		s.runners[result.runner.ID] = result.runner
	case operationDelete:
		delete(s.runners, item.runnerID)
	}
	delete(s.failures, keyFor(item))

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

// retryBlocked reports whether item is still inside its failure cooldown.
func (s *reconcileState) retryBlocked(item operation, now time.Time) bool {
	failure, exists := s.failures[keyFor(item)]
	return exists && now.Before(failure.retryAt)
}

// recordFailure advances item cooldown and requires a fresh inventory before mutation.
func (s *reconcileState) recordFailure(item operation, err error) {
	key := keyFor(item)
	previous := s.failures[key]
	delay := nextFailureDelay(previous.delay, s.options.RetryInitial, s.options.RetryMaximum)
	s.failures[key] = failureBackoff{delay: delay, retryAt: time.Now().Add(delay)}
	s.inventoryStale = true
	s.options.Logger.Warn(
		"runner operation failed",
		"operation", item.kind,
		"operation_id", item.id,
		"runner_id", item.runnerID,
		"retry_after", delay,
		"error", err,
	)
}

// keyFor maps an operation to its independently retryable failure target.
func keyFor(item operation) failureKey {
	return failureKey{kind: item.kind, runnerID: item.runnerID}
}

// nextFailureDelay doubles previous up to maximum and starts at initial.
func nextFailureDelay(previous, initial, maximum time.Duration) time.Duration {
	if previous <= 0 {
		return initial
	}
	if previous >= maximum/2 {
		return maximum
	}
	return min(previous+previous, maximum)
}

// inventorying reports whether a refresh currently owns the observed-state boundary.
func (s *reconcileState) inventorying() bool {
	for _, item := range s.operations {
		if item.kind == operationList {
			return true
		}
	}

	return false
}

// validatedInventory converts an observed snapshot into reconciler-owned state.
func validatedInventory(runners []Runner) (map[string]Runner, error) {
	inventory := make(map[string]Runner, len(runners))
	for _, runner := range runners {
		if err := validateRunner(runner); err != nil {
			return nil, fmt.Errorf("inventory owned runner: %w", err)
		}
		inventory[runner.ID] = runner
	}

	return inventory, nil
}

// liveCapacity counts usable and in-flight capacity after scheduled deletions.
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

// idleRunner selects an idle runner that is not already being deleted.
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

// validateRunner checks the minimum identity and lifecycle invariants.
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
