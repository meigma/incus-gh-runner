package incus

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/provenance"
)

// proofWrite records one attempted proof-sink file mutation.
type proofWrite struct {
	path    string
	content []byte
	mode    int
}

// proofFakeClient provides deterministic identity and guest-file behavior.
type proofFakeClient struct {
	instances  []*api.Instance
	instanceAt int
	files      map[string][]byte
	writes     []proofWrite
	events     []string
	writeError map[string]error
}

// newProofFakeClient creates a running owned target with an empty proof directory.
func newProofFakeClient() *proofFakeClient {
	instance := ownedInstance("runner", "Running", time.Now())
	return &proofFakeClient{
		instances:  []*api.Instance{&instance, &instance},
		files:      make(map[string][]byte),
		writes:     make([]proofWrite, 0),
		events:     make([]string, 0),
		writeError: make(map[string]error),
	}
}

// GetInstance returns the next configured identity observation.
func (f *proofFakeClient) GetInstance(
	_ context.Context,
	name string,
) (*api.Instance, string, error) {
	f.events = append(f.events, "get-instance")
	if f.instanceAt >= len(f.instances) || f.instances[f.instanceAt] == nil {
		f.instanceAt++
		return nil, "", errNotFound
	}
	instance := *f.instances[f.instanceAt]
	f.instanceAt++
	if instance.Name != name {
		return nil, "", fmt.Errorf("unexpected instance lookup %q", name)
	}

	return &instance, "etag", nil
}

// CreateInstanceFile records and materializes one guest-file write.
func (f *proofFakeClient) CreateInstanceFile(
	_ context.Context,
	_ string,
	path string,
	content []byte,
	mode int,
) error {
	f.events = append(f.events, "write "+path)
	if err := f.writeError[path]; err != nil {
		return err
	}
	f.files[path] = bytes.Clone(content)
	f.writes = append(f.writes, proofWrite{path: path, content: bytes.Clone(content), mode: mode})

	return nil
}

// GetInstanceFile returns a committed fake guest file when present.
func (f *proofFakeClient) GetInstanceFile(_ context.Context, _ string, path string) ([]byte, error) {
	f.events = append(f.events, "read "+path)
	content, ok := f.files[path]
	if !ok {
		return nil, errInstanceFileNotFound
	}

	return bytes.Clone(content), nil
}

// TestProofSinkCommitsAfterTwoIdentityChecks proves proof-before-marker ordering and permissions.
func TestProofSinkCommitsAfterTwoIdentityChecks(t *testing.T) {
	t.Parallel()

	client := newProofFakeClient()
	sink, signer, target := newTestProofSink(t, client)
	envelope := signTestProof(t, signer, target, "job-1")

	err := sink.Deliver(context.Background(), target, envelope)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"get-instance",
		"read " + proofReadyPath,
		"write " + proofPath,
		"get-instance",
		"write " + proofReadyPath,
	}, client.events)
	require.Len(t, client.writes, 2)
	assert.Equal(t, proofPath, client.writes[0].path)
	assert.Equal(t, envelope, client.writes[0].content)
	assert.Equal(t, proofReadyPath, client.writes[1].path)
	assert.Empty(t, client.writes[1].content)
	assert.Equal(t, proofFileMode, client.writes[0].mode)
	assert.Equal(t, proofFileMode, client.writes[1].mode)
}

// TestProofSinkFailsClosedAroundWrites proves identity and file errors never commit uncertain proof state.
func TestProofSinkFailsClosedAroundWrites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		prepare     func(*proofFakeClient)
		wantError   string
		wantProof   bool
		wantMarker  bool
		wantLookups int
	}{
		{
			name: "unowned before proof",
			prepare: func(client *proofFakeClient) {
				client.instances[0].Config[ownershipKey] = "another-owner"
			},
			wantError:   "unowned Incus instance",
			wantLookups: 1,
		},
		{
			name: "stopped before proof",
			prepare: func(client *proofFakeClient) {
				client.instances[0].Status = "Stopped"
			},
			wantError:   "non-running Incus instance",
			wantLookups: 1,
		},
		{
			name: "proof write failure",
			prepare: func(client *proofFakeClient) {
				client.writeError[proofPath] = errors.New("agent rejected proof")
			},
			wantError:   "agent rejected proof",
			wantLookups: 1,
		},
		{
			name: "replacement before marker",
			prepare: func(client *proofFakeClient) {
				replacement := *client.instances[1]
				replacement.Config = map[string]string{
					ownershipKey:    "test-owner",
					instanceUUIDKey: stableTestUUID("replacement"),
				}
				client.instances[1] = &replacement
			},
			wantError:   "replacement Incus instance",
			wantProof:   true,
			wantLookups: 2,
		},
		{
			name: "instance disappears before marker",
			prepare: func(client *proofFakeClient) {
				client.instances[1] = nil
			},
			wantError:   "incus resource not found",
			wantProof:   true,
			wantLookups: 2,
		},
		{
			name: "marker write failure",
			prepare: func(client *proofFakeClient) {
				client.writeError[proofReadyPath] = errors.New("agent rejected marker")
			},
			wantError:   "agent rejected marker",
			wantProof:   true,
			wantLookups: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newProofFakeClient()
			tt.prepare(client)
			sink, signer, target := newTestProofSink(t, client)
			envelope := signTestProof(t, signer, target, "job-1")

			err := sink.Deliver(context.Background(), target, envelope)

			require.ErrorContains(t, err, tt.wantError)
			_, proofExists := client.files[proofPath]
			_, markerExists := client.files[proofReadyPath]
			assert.Equal(t, tt.wantProof, proofExists)
			assert.Equal(t, tt.wantMarker, markerExists)
			assert.Equal(t, tt.wantLookups, client.instanceAt)
		})
	}
}

// TestProofSinkTreatsCommittedProofAsImmutable proves exact duplicates are no-ops and other jobs are refused.
func TestProofSinkTreatsCommittedProofAsImmutable(t *testing.T) {
	t.Parallel()

	client := newProofFakeClient()
	client.instances = append(client.instances, client.instances[0], client.instances[0])
	sink, signer, target := newTestProofSink(t, client)
	firstEnvelope := signTestProof(t, signer, target, "job-1")
	require.NoError(t, sink.Deliver(context.Background(), target, firstEnvelope))
	committedWrites := len(client.writes)

	require.NoError(t, sink.Deliver(context.Background(), target, firstEnvelope))
	assert.Len(t, client.writes, committedWrites, "an identical duplicate must not rewrite committed files")

	differentEnvelope := signTestProof(t, signer, target, "job-2")
	err := sink.Deliver(context.Background(), target, differentEnvelope)
	require.ErrorContains(t, err, "refusing to replace committed job proof")
	assert.Len(t, client.writes, committedWrites)
}

// TestProofSinkRejectsMalformedCommittedProof proves readiness never makes an invalid envelope acceptable.
func TestProofSinkRejectsMalformedCommittedProof(t *testing.T) {
	t.Parallel()

	client := newProofFakeClient()
	client.files[proofReadyPath] = nil
	client.files[proofPath] = []byte(`{"not":"a proof"}`)
	sink, signer, target := newTestProofSink(t, client)
	incoming := signTestProof(t, signer, target, "job-1")

	err := sink.Deliver(context.Background(), target, incoming)

	require.ErrorContains(t, err, "verify committed job proof")
	assert.Empty(t, client.writes)
}

// TestProofSinkRejectsEnvelopeForAnotherTarget proves signed machine identity selects no alternate VM.
func TestProofSinkRejectsEnvelopeForAnotherTarget(t *testing.T) {
	t.Parallel()

	client := newProofFakeClient()
	sink, signer, target := newTestProofSink(t, client)
	other := provenance.MachineTarget{
		InstanceName: "other-runner",
		InstanceUUID: stableTestUUID("other-runner"),
	}
	envelope := signTestProof(t, signer, other, "job-1")

	err := sink.Deliver(context.Background(), target, envelope)

	require.ErrorContains(t, err, "does not identify its delivery target")
	assert.Empty(t, client.events, "target mismatch must fail before Incus I/O")
}

// newTestProofSink constructs a sink, signer, and exact fake target.
func newTestProofSink(
	t *testing.T,
	client *proofFakeClient,
) (*ProofSink, *provenance.Signer, provenance.MachineTarget) {
	t.Helper()

	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	signer, err := provenance.NewSigner(privateKey)
	require.NoError(t, err)
	verifier, err := provenance.NewEnvelopeVerifier(privateKey.Public().(ed25519.PublicKey), "builder-host-01")
	require.NoError(t, err)
	sink, err := newProofSink(client, "test-owner", verifier)
	require.NoError(t, err)
	target := provenance.MachineTarget{
		InstanceName: "runner",
		InstanceUUID: stableTestUUID("runner"),
	}

	return sink, signer, target
}

// signTestProof creates one valid deterministic envelope for sink behavior tests.
func signTestProof(
	t *testing.T,
	signer *provenance.Signer,
	target provenance.MachineTarget,
	jobID string,
) []byte {
	t.Helper()

	payload := provenance.Payload{
		Version:  provenance.Version,
		Claim:    provenance.Claim,
		IssuedAt: time.Date(2026, 7, 20, 20, 15, 32, 0, time.UTC),
		Host: provenance.Host{
			ID:                "builder-host-01",
			ControllerVersion: "1.1.0",
			ControllerCommit:  "0123456789abcdef",
		},
		GitHub: provenance.GitHub{
			Owner:           "meigma",
			Repository:      "incus-gh-runner",
			WorkflowRef:     "meigma/incus-gh-runner/.github/workflows/ci.yml@refs/heads/master",
			WorkflowRunID:   100,
			JobID:           jobID,
			RunnerRequestID: 200,
			RunnerID:        101,
			RunnerName:      target.InstanceName,
			EventName:       "workflow_dispatch",
			ScaleSetID:      42,
			ScaleSetName:    "incus-linux-x64",
		},
		Machine: provenance.Machine{
			IncusProject:              "github-runners",
			InstanceName:              target.InstanceName,
			InstanceUUID:              target.InstanceUUID,
			ImageFingerprint:          strings.Repeat("1", 64),
			LaunchConfigurationSHA256: strings.Repeat("2", 64),
			Profiles:                  []provenance.Profile{},
		},
	}
	envelope, err := signer.Sign(context.Background(), payload)
	require.NoError(t, err)

	return envelope
}
