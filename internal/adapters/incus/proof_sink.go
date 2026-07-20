package incus

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	incusclient "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"

	"github.com/meigma/incus-gh-runner/internal/provenance"
)

const (
	proofPath      = "/run/incus-gh-runner-proof/job-proof.dsse.json"
	proofReadyPath = "/run/incus-gh-runner-proof/job-proof.ready"
	proofFileMode  = 0o444
)

// proofClient is the Incus file and identity surface required by ProofSink.
type proofClient interface {
	GetInstance(ctx context.Context, name string) (*api.Instance, string, error)
	CreateInstanceFile(ctx context.Context, name string, path string, content []byte, mode int) error
	GetInstanceFile(ctx context.Context, name string, path string) ([]byte, error)
}

// ProofSink commits verified job machine proofs through the Incus guest agent.
type ProofSink struct {
	client   proofClient
	owner    string
	verifier provenance.ProofVerifier
}

// NewProofSink constructs an ownership-scoped proof sink from an Incus server.
func NewProofSink(
	server incusclient.InstanceServer,
	owner string,
	verifier provenance.ProofVerifier,
) (*ProofSink, error) {
	client, err := newServerClient(server)
	if err != nil {
		return nil, err
	}

	return newProofSink(client, owner, verifier)
}

// newProofSink constructs a proof sink around its narrow testable boundaries.
func newProofSink(client proofClient, owner string, verifier provenance.ProofVerifier) (*ProofSink, error) {
	if client == nil {
		return nil, errors.New("incus proof client is required")
	}
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("incus proof ownership identity is required")
	}
	if verifier == nil {
		return nil, errors.New("job proof verifier is required")
	}

	return &ProofSink{client: client, owner: owner, verifier: verifier}, nil
}

// Deliver commits one verified envelope to an exact running owned VM.
func (s *ProofSink) Deliver(
	ctx context.Context,
	target provenance.MachineTarget,
	envelope []byte,
) error {
	if strings.TrimSpace(target.InstanceName) == "" {
		return errors.New("proof target instance name is required")
	}
	if _, parseErr := uuid.Parse(target.InstanceUUID); parseErr != nil {
		return errors.New("proof target instance UUID is invalid")
	}
	incoming, verifyErr := s.verifier.Verify(ctx, envelope)
	if verifyErr != nil {
		return fmt.Errorf("verify incoming job proof: %w", verifyErr)
	}
	if incoming.Machine.InstanceName != target.InstanceName || incoming.Machine.InstanceUUID != target.InstanceUUID {
		return errors.New("incoming job proof does not identify its delivery target")
	}

	if targetErr := s.verifyTarget(ctx, target, "proof write"); targetErr != nil {
		return targetErr
	}
	_, markerErr := s.client.GetInstanceFile(ctx, target.InstanceName, proofReadyPath)
	if markerErr == nil {
		return s.acceptCommittedDuplicate(ctx, target.InstanceName, incoming)
	}
	if !errors.Is(markerErr, errInstanceFileNotFound) {
		return fmt.Errorf("check proof commit marker on %q: %w", target.InstanceName, markerErr)
	}

	if writeErr := s.client.CreateInstanceFile(
		ctx,
		target.InstanceName,
		proofPath,
		envelope,
		proofFileMode,
	); writeErr != nil {
		return fmt.Errorf("write job proof to %q: %w", target.InstanceName, writeErr)
	}
	if targetErr := s.verifyTarget(ctx, target, "proof marker write"); targetErr != nil {
		return targetErr
	}
	if writeErr := s.client.CreateInstanceFile(
		ctx,
		target.InstanceName,
		proofReadyPath,
		nil,
		proofFileMode,
	); writeErr != nil {
		return fmt.Errorf("write job proof commit marker to %q: %w", target.InstanceName, writeErr)
	}

	return nil
}

// acceptCommittedDuplicate validates the immutable proof selected by an existing marker.
func (s *ProofSink) acceptCommittedDuplicate(
	ctx context.Context,
	instanceName string,
	incoming provenance.Payload,
) error {
	committedEnvelope, err := s.client.GetInstanceFile(ctx, instanceName, proofPath)
	if err != nil {
		return fmt.Errorf("read committed job proof from %q: %w", instanceName, err)
	}
	committed, err := s.verifier.Verify(ctx, committedEnvelope)
	if err != nil {
		return fmt.Errorf("verify committed job proof from %q: %w", instanceName, err)
	}
	if proofTupleFor(committed) != proofTupleFor(incoming) {
		return fmt.Errorf("refusing to replace committed job proof on %q", instanceName)
	}

	return nil
}

// verifyTarget fences each write with the exact owner, running state, and UUID.
func (s *ProofSink) verifyTarget(
	ctx context.Context,
	target provenance.MachineTarget,
	operation string,
) error {
	instance, _, err := s.client.GetInstance(ctx, target.InstanceName)
	if err != nil {
		return fmt.Errorf("get instance %q before %s: %w", target.InstanceName, operation, err)
	}
	if instance == nil {
		return fmt.Errorf("refusing %s for Incus instance %q without identity", operation, target.InstanceName)
	}
	if instance.Config[ownershipKey] != s.owner {
		return fmt.Errorf("refusing %s for unowned Incus instance %q", operation, target.InstanceName)
	}
	if !strings.EqualFold(instance.Status, "running") {
		return fmt.Errorf("refusing %s for non-running Incus instance %q", operation, target.InstanceName)
	}
	instanceUUID := instance.Config[instanceUUIDKey]
	if _, err := uuid.Parse(instanceUUID); err != nil {
		return fmt.Errorf(
			"refusing %s for Incus instance %q without a valid stable UUID",
			operation,
			target.InstanceName,
		)
	}
	if instanceUUID != target.InstanceUUID {
		return fmt.Errorf("refusing %s for replacement Incus instance %q", operation, target.InstanceName)
	}

	return nil
}

// proofTuple is the immutable idempotency identity for one committed proof.
type proofTuple struct {
	jobID        string
	runnerID     int64
	instanceUUID string
}

// proofTupleFor selects the signed fields that define an identical delivery.
func proofTupleFor(payload provenance.Payload) proofTuple {
	return proofTuple{
		jobID:        payload.GitHub.JobID,
		runnerID:     payload.GitHub.RunnerID,
		instanceUUID: payload.Machine.InstanceUUID,
	}
}

var _ provenance.ProofSink = (*ProofSink)(nil)
