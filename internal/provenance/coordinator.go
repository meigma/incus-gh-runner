package provenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrJobStartedQueueFull reports that a synchronous callback could not enqueue without blocking.
var ErrJobStartedQueueFull = errors.New("job proof event queue is full")

// JobStarted records the authenticated GitHub fields required by one proof.
type JobStarted struct {
	// Owner is the repository owner reported by GitHub.
	Owner string
	// Repository is the repository name reported by GitHub.
	Repository string
	// WorkflowRef is the opaque workflow reference reported by GitHub.
	WorkflowRef string
	// WorkflowRunID is the positive workflow run identifier.
	WorkflowRunID int64
	// JobID is the opaque job identifier.
	JobID string
	// RunnerRequestID is the positive runner request identifier.
	RunnerRequestID int64
	// RunnerID is the positive JIT runner registration identifier.
	RunnerID int64
	// RunnerName is the exact JIT runner and Incus instance name.
	RunnerName string
	// EventName is the workflow event reported by GitHub.
	EventName string
	// ScaleSetID is the controller-resolved scale-set identifier.
	ScaleSetID int64
	// ScaleSetName is the controller-resolved scale-set name.
	ScaleSetName string
}

// Validate checks authenticated event field shape before asynchronous processing.
func (e JobStarted) Validate() error {
	stringsToValidate := []struct {
		name  string
		value string
	}{
		{name: "github.owner", value: e.Owner},
		{name: "github.repository", value: e.Repository},
		{name: "github.workflow_ref", value: e.WorkflowRef},
		{name: "github.job_id", value: e.JobID},
		{name: "github.runner_name", value: e.RunnerName},
		{name: "github.event_name", value: e.EventName},
		{name: "github.scale_set_name", value: e.ScaleSetName},
	}
	for _, field := range stringsToValidate {
		if err := validateIdentity(field.name, field.value); err != nil {
			return err
		}
	}
	positiveIDs := []struct {
		name  string
		value int64
	}{
		{name: "github.workflow_run_id", value: e.WorkflowRunID},
		{name: "github.runner_request_id", value: e.RunnerRequestID},
		{name: "github.runner_id", value: e.RunnerID},
		{name: "github.scale_set_id", value: e.ScaleSetID},
	}
	for _, field := range positiveIDs {
		if field.value <= 0 {
			return fmt.Errorf("%s must be positive", field.name)
		}
	}

	return nil
}

// JobStartedSink accepts authenticated job events without blocking their callback.
type JobStartedSink interface {
	// TryEnqueue validates event and returns ErrJobStartedQueueFull instead of blocking.
	TryEnqueue(event JobStarted) error
}

// JobStartedQueue is a bounded single-consumer proof-event queue.
type JobStartedQueue struct {
	events chan JobStarted
}

// NewJobStartedQueue constructs a queue with fixed positive capacity.
func NewJobStartedQueue(capacity int) (*JobStartedQueue, error) {
	if capacity < 1 {
		return nil, errors.New("job proof event queue capacity must be positive")
	}

	return &JobStartedQueue{events: make(chan JobStarted, capacity)}, nil
}

// TryEnqueue validates and copies event into the queue only when space is immediately available.
func (q *JobStartedQueue) TryEnqueue(event JobStarted) error {
	if q == nil || q.events == nil {
		return errors.New("job proof event queue is not configured")
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate job proof event: %w", err)
	}
	select {
	case q.events <- event:
		return nil
	default:
		return ErrJobStartedQueueFull
	}
}

// Events returns the queue's single-consumer stream.
func (q *JobStartedQueue) Events() <-chan JobStarted {
	if q == nil {
		return nil
	}

	return q.events
}

// MachineSource verifies and projects one exact owned VM for an authenticated event.
type MachineSource interface {
	// Snapshot returns signed machine fields only after exact event and launch checks.
	Snapshot(ctx context.Context, event JobStarted) (Machine, error)
}

// ProofSigner creates one authenticated envelope from a complete payload.
type ProofSigner interface {
	// Sign validates and signs payload.
	Sign(ctx context.Context, payload Payload) ([]byte, error)
}

// CoordinatorOptions configures asynchronous proof signing and delivery.
type CoordinatorOptions struct {
	// Events is the bounded authenticated event stream.
	Events <-chan JobStarted
	// Machines resolves trusted VM state for each event.
	Machines MachineSource
	// Signer creates the DSSE envelope.
	Signer ProofSigner
	// Sink commits the signed envelope to the exact VM.
	Sink ProofSink
	// Host identifies the enrolled running controller.
	Host Host
	// OperationTimeout bounds snapshot, signing, and delivery for one event.
	OperationTimeout time.Duration
	// Now supplies issued_at for deterministic tests.
	Now func() time.Time
	// Logger receives secret-safe per-event failures.
	Logger *slog.Logger
}

// Coordinator serially verifies, signs, and delivers queued job proofs.
type Coordinator struct {
	options CoordinatorOptions
}

// NewCoordinator constructs one single-owner proof coordinator.
func NewCoordinator(options CoordinatorOptions) (*Coordinator, error) {
	if options.Events == nil {
		return nil, errors.New("job proof event stream is required")
	}
	if options.Machines == nil {
		return nil, errors.New("job proof machine source is required")
	}
	if options.Signer == nil {
		return nil, errors.New("job proof signer is required")
	}
	if options.Sink == nil {
		return nil, errors.New("job proof sink is required")
	}
	if err := validateHost(options.Host); err != nil {
		return nil, err
	}
	if options.OperationTimeout <= 0 {
		return nil, errors.New("job proof operation timeout must be positive")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}

	return &Coordinator{options: options}, nil
}

// Run processes events until cancellation and keeps per-event failures isolated.
func (c *Coordinator) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-c.options.Events:
			if !ok {
				return errors.New("job proof event stream closed unexpectedly")
			}
			operationContext, cancel := context.WithTimeout(ctx, c.options.OperationTimeout)
			err := c.process(operationContext, event)
			cancel()
			if err != nil {
				c.options.Logger.ErrorContext(
					ctx,
					"job proof delivery failed",
					"job_id", event.JobID,
					"runner_id", event.RunnerID,
					"runner_name", event.RunnerName,
					"error", err,
				)
			}
		}
	}
}

// process creates and commits one proof after the machine source accepts correlation.
func (c *Coordinator) process(ctx context.Context, event JobStarted) error {
	machine, err := c.options.Machines.Snapshot(ctx, event)
	if err != nil {
		return fmt.Errorf("verify job machine: %w", err)
	}
	payload := Payload{
		Version:  Version,
		Claim:    Claim,
		IssuedAt: c.options.Now().UTC(),
		Host:     c.options.Host,
		GitHub:   GitHub(event),
		Machine:  machine,
	}
	envelope, err := c.options.Signer.Sign(ctx, payload)
	if err != nil {
		return fmt.Errorf("sign job proof: %w", err)
	}
	target := MachineTarget{InstanceName: machine.InstanceName, InstanceUUID: machine.InstanceUUID}
	if err := c.options.Sink.Deliver(ctx, target, envelope); err != nil {
		return fmt.Errorf("deliver job proof: %w", err)
	}

	return nil
}

// validateHost checks the controller-owned identity before the coordinator starts.
func validateHost(host Host) error {
	if err := validateIdentity("host.id", host.ID); err != nil {
		return err
	}
	if err := validateIdentity("host.controller_version", host.ControllerVersion); err != nil {
		return err
	}
	return validateIdentity("host.controller_commit", host.ControllerCommit)
}

var _ JobStartedSink = (*JobStartedQueue)(nil)
