package provenance

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// machineSourceFunc adapts a function to the trusted machine snapshot port.
type machineSourceFunc func(context.Context, JobStarted) (Machine, error)

// Snapshot invokes f for event.
func (f machineSourceFunc) Snapshot(ctx context.Context, event JobStarted) (Machine, error) {
	return f(ctx, event)
}

// proofSinkFunc adapts a function to the proof delivery port.
type proofSinkFunc func(context.Context, MachineTarget, []byte) error

// Deliver invokes f for target and envelope.
func (f proofSinkFunc) Deliver(ctx context.Context, target MachineTarget, envelope []byte) error {
	return f(ctx, target, envelope)
}

// TestJobStartedQueueValidatesAndNeverBlocks proves bounded callback behavior.
func TestJobStartedQueueValidatesAndNeverBlocks(t *testing.T) {
	t.Parallel()

	queue, err := NewJobStartedQueue(1)
	require.NoError(t, err)
	invalid := fixedJobStarted()
	invalid.RunnerID = 0
	require.ErrorContains(t, queue.TryEnqueue(invalid), "github.runner_id must be positive")

	first := fixedJobStarted()
	require.NoError(t, queue.TryEnqueue(first))
	second := first
	second.JobID = "job-2"
	require.ErrorIs(t, queue.TryEnqueue(second), ErrJobStartedQueueFull)
	assert.Equal(t, first, <-queue.Events(), "queue saturation must preserve the already accepted event")
}

// TestCoordinatorSignsAndDeliversVerifiedEvent proves the complete asynchronous core behavior.
func TestCoordinatorSignsAndDeliversVerifiedEvent(t *testing.T) {
	t.Parallel()

	privateKey := fixedPrivateKey(t, 0)
	signer, err := NewSigner(privateKey)
	require.NoError(t, err)
	verifier, err := NewEnvelopeVerifier(privateKey.Public().(ed25519.PublicKey), "builder-host-01")
	require.NoError(t, err)
	event := fixedJobStarted()
	machine := fixedPayload().Machine
	delivered := make(chan Payload, 1)
	queue, err := NewJobStartedQueue(1)
	require.NoError(t, err)
	coordinator, err := NewCoordinator(CoordinatorOptions{
		Events: queue.Events(),
		Machines: machineSourceFunc(func(_ context.Context, observed JobStarted) (Machine, error) {
			assert.Equal(t, event, observed)
			return machine, nil
		}),
		Signer: signer,
		Sink: proofSinkFunc(func(ctx context.Context, target MachineTarget, envelope []byte) error {
			assert.Equal(t, MachineTarget{
				InstanceName: machine.InstanceName,
				InstanceUUID: machine.InstanceUUID,
			}, target)
			payload, verifyErr := verifier.Verify(ctx, envelope)
			if verifyErr == nil {
				delivered <- payload
			}
			return verifyErr
		}),
		Host:             fixedPayload().Host,
		OperationTimeout: time.Second,
		Now:              func() time.Time { return fixedPayload().IssuedAt },
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- coordinator.Run(ctx) }()

	require.NoError(t, queue.TryEnqueue(event))
	var payload Payload
	select {
	case payload = <-delivered:
	case <-time.After(time.Second):
		t.Fatal("coordinator did not deliver the proof")
	}
	cancel()

	require.ErrorIs(t, <-errCh, context.Canceled)
	assert.Equal(t, fixedPayload(), payload)
}

// TestCoordinatorIsolatesEventFailures proves one failed delivery does not stop the queue owner.
func TestCoordinatorIsolatesEventFailures(t *testing.T) {
	t.Parallel()

	queue, err := NewJobStartedQueue(2)
	require.NoError(t, err)
	privateKey := fixedPrivateKey(t, 0)
	signer, err := NewSigner(privateKey)
	require.NoError(t, err)
	deliveries := 0
	succeeded := make(chan struct{})
	coordinator, err := NewCoordinator(CoordinatorOptions{
		Events: queue.Events(),
		Machines: machineSourceFunc(func(context.Context, JobStarted) (Machine, error) {
			return fixedPayload().Machine, nil
		}),
		Signer: signer,
		Sink: proofSinkFunc(func(context.Context, MachineTarget, []byte) error {
			deliveries++
			if deliveries == 1 {
				return errors.New("guest unavailable")
			}
			close(succeeded)
			return nil
		}),
		Host:             fixedPayload().Host,
		OperationTimeout: time.Second,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- coordinator.Run(ctx) }()

	first := fixedJobStarted()
	second := first
	second.JobID = "job-2"
	require.NoError(t, queue.TryEnqueue(first))
	require.NoError(t, queue.TryEnqueue(second))
	select {
	case <-succeeded:
	case <-time.After(time.Second):
		t.Fatal("coordinator stopped after an isolated delivery failure")
	}
	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
}

// fixedJobStarted projects the deterministic proof fixture into one authenticated event.
func fixedJobStarted() JobStarted {
	payload := fixedPayload()
	return JobStarted{
		Owner:           payload.GitHub.Owner,
		Repository:      payload.GitHub.Repository,
		WorkflowRef:     payload.GitHub.WorkflowRef,
		WorkflowRunID:   payload.GitHub.WorkflowRunID,
		JobID:           payload.GitHub.JobID,
		RunnerRequestID: payload.GitHub.RunnerRequestID,
		RunnerID:        payload.GitHub.RunnerID,
		RunnerName:      payload.GitHub.RunnerName,
		EventName:       payload.GitHub.EventName,
		ScaleSetID:      payload.GitHub.ScaleSetID,
		ScaleSetName:    payload.GitHub.ScaleSetName,
	}
}
