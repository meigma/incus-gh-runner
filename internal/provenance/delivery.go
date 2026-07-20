package provenance

import "context"

// MachineTarget identifies the exact owned VM that may receive a proof.
type MachineTarget struct {
	// InstanceName is the controller-selected Incus instance name.
	InstanceName string
	// InstanceUUID is the server-generated stable Incus instance identity.
	InstanceUUID string
}

// ProofVerifier authenticates an envelope and returns its validated payload.
type ProofVerifier interface {
	// Verify authenticates envelope against one enrolled host identity.
	Verify(ctx context.Context, envelope []byte) (Payload, error)
}

// ProofSink commits an already signed envelope to one exact machine target.
type ProofSink interface {
	// Deliver writes envelope and its commit marker without changing the target identity.
	Deliver(ctx context.Context, target MachineTarget, envelope []byte) error
}
