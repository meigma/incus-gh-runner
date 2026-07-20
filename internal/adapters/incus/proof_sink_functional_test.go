package incus

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	incusclient "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/provenance"
)

// TestProofSinkFunctional delivers and retrieves a signed envelope through a real reference VM.
func TestProofSinkFunctional(t *testing.T) {
	project := os.Getenv("INCUS_GH_RUNNER_TEST_PROJECT")
	image := os.Getenv("INCUS_GH_RUNNER_TEST_IMAGE")
	if project == "" || image == "" {
		t.Skip("set INCUS_GH_RUNNER_TEST_PROJECT and INCUS_GH_RUNNER_TEST_IMAGE to run")
	}
	require.NotEqual(t, "default", project, "functional proof delivery must use a disposable non-default project")

	testContext, cancelTest := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelTest()
	server, err := ConnectUnix(testContext, os.Getenv("INCUS_GH_RUNNER_TEST_SOCKET"), project)
	require.NoError(t, err)
	client, err := newServerClient(server)
	require.NoError(t, err)

	imageIdentity, err := client.ResolveImage(testContext, image)
	require.NoError(t, err)
	owner := "functional-proof-" + uuid.NewString()
	name := "proof-" + uuid.NewString()
	profiles := splitProfiles(os.Getenv("INCUS_GH_RUNNER_TEST_PROFILES"))
	if len(profiles) == 0 {
		profiles = []string{defaultProfileName}
	}
	require.NoError(t, client.CreateInstance(testContext, api.InstancesPost{
		Name: name,
		Type: api.InstanceTypeVM,
		InstancePut: api.InstancePut{
			Config:   api.ConfigMap{ownershipKey: owner},
			Profiles: profiles,
		},
		Source: api.InstanceSource{Type: "image", Fingerprint: imageIdentity.Fingerprint},
	}))
	t.Cleanup(func() { cleanupProofFunctionalInstance(t, server, name) })
	require.NoError(t, client.StartInstance(testContext, name))

	var target provenance.MachineTarget
	require.Eventually(t, func() bool {
		instance, _, lookupErr := client.GetInstance(testContext, name)
		if lookupErr != nil || !strings.EqualFold(instance.Status, "running") {
			return false
		}
		instanceUUID := instance.Config[instanceUUIDKey]
		if _, parseErr := uuid.Parse(instanceUUID); parseErr != nil {
			return false
		}
		_, fileErr := client.GetInstanceFile(testContext, name, proofReadyPath)
		if !errors.Is(fileErr, errInstanceFileNotFound) {
			return false
		}
		target = provenance.MachineTarget{InstanceName: name, InstanceUUID: instanceUUID}
		return true
	}, 5*time.Minute, time.Second, "reference VM agent must become available")

	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize))
	signer, err := provenance.NewSigner(privateKey)
	require.NoError(t, err)
	verifier, err := provenance.NewEnvelopeVerifier(privateKey.Public().(ed25519.PublicKey), "functional-builder")
	require.NoError(t, err)
	sink, err := NewProofSink(server, owner, verifier)
	require.NoError(t, err)
	envelope := signFunctionalProof(t, signer, target, project, imageIdentity.Fingerprint)

	deliveryStarted := time.Now()
	require.NoError(t, sink.Deliver(testContext, target, envelope))
	deliveryDuration := time.Since(deliveryStarted)
	assert.Less(t, deliveryDuration, 5*time.Minute, "delivery must fit the existing bounded Incus operation window")
	t.Logf("Incus guest-agent proof delivery completed in %s", deliveryDuration)

	outputPath := "/tmp/job-proof-" + uuid.NewString() + ".json"
	_, stderr, err := execProofFunctionalInstance(
		testContext,
		server,
		name,
		[]string{
			"/usr/sbin/runuser", "-u", "actions-runner", "--",
			"/usr/local/bin/incus-gh-runner-proof", "--output", outputPath, "--timeout", "10s",
		},
	)
	require.NoError(t, err, "unprivileged proof helper failed: %s", stderr)
	retrieved, err := client.GetInstanceFile(testContext, name, outputPath)
	require.NoError(t, err)
	assert.Equal(t, envelope, retrieved)

	stdout, stderr, err := execProofFunctionalInstance(
		testContext,
		server,
		name,
		[]string{
			"/usr/bin/stat", "-c", "%a %U %G %n",
			"/run/incus-gh-runner",
			"/run/incus-gh-runner-proof",
			proofPath,
			proofReadyPath,
			outputPath,
		},
	)
	require.NoError(t, err, "guest permission inspection failed: %s", stderr)
	assert.Contains(t, stdout, "700 root root /run/incus-gh-runner\n")
	assert.Contains(t, stdout, "755 root root /run/incus-gh-runner-proof\n")
	assert.Contains(t, stdout, "444 root root "+proofPath+"\n")
	assert.Contains(t, stdout, "444 root root "+proofReadyPath+"\n")
	assert.Contains(t, stdout, "600 actions-runner actions-runner "+outputPath+"\n")
}

// signFunctionalProof creates the phase 1 envelope injected by the live harness.
func signFunctionalProof(
	t *testing.T,
	signer *provenance.Signer,
	target provenance.MachineTarget,
	project string,
	imageFingerprint string,
) []byte {
	t.Helper()

	payload := provenance.Payload{
		Version:  provenance.Version,
		Claim:    provenance.Claim,
		IssuedAt: time.Now().UTC(),
		Host: provenance.Host{
			ID:                "functional-builder",
			ControllerVersion: "functional",
			ControllerCommit:  "functional",
		},
		GitHub: provenance.GitHub{
			Owner:           "meigma",
			Repository:      "incus-gh-runner",
			WorkflowRef:     "meigma/incus-gh-runner/.github/workflows/ci.yml@refs/heads/master",
			WorkflowRunID:   1,
			JobID:           "phase-2-functional",
			RunnerRequestID: 2,
			RunnerID:        3,
			RunnerName:      target.InstanceName,
			EventName:       "workflow_dispatch",
			ScaleSetID:      4,
			ScaleSetName:    "phase-2-functional",
		},
		Machine: provenance.Machine{
			IncusProject:              project,
			InstanceName:              target.InstanceName,
			InstanceUUID:              target.InstanceUUID,
			ImageFingerprint:          imageFingerprint,
			LaunchConfigurationSHA256: strings.Repeat("a", 64),
			Profiles:                  []provenance.Profile{},
		},
	}
	envelope, err := signer.Sign(context.Background(), payload)
	require.NoError(t, err)

	return envelope
}

// execProofFunctionalInstance runs one non-interactive guest command and captures its streams.
func execProofFunctionalInstance(
	ctx context.Context,
	server incusclient.InstanceServer,
	name string,
	command []string,
) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dataDone := make(chan bool)
	contextual, ok := server.(interface {
		WithContext(context.Context) incusclient.InstanceServer
	})
	if !ok {
		return "", "", errors.New("Incus server does not support request contexts")
	}
	operation, err := contextual.WithContext(ctx).ExecInstance(name, api.InstanceExecPost{
		Command:   command,
		WaitForWS: true,
	}, &incusclient.InstanceExecArgs{
		Stdout:   &stdout,
		Stderr:   &stderr,
		DataDone: dataDone,
	})
	if err != nil {
		return "", "", err
	}
	if err := operation.WaitContext(ctx); err != nil {
		return stdout.String(), stderr.String(), err
	}
	select {
	case <-dataDone:
	case <-ctx.Done():
		return stdout.String(), stderr.String(), ctx.Err()
	}
	if rawStatus, ok := operation.Get().Metadata["return"].(float64); ok && rawStatus != 0 {
		return stdout.String(), stderr.String(), fmt.Errorf("guest command exited with status %d", int(rawStatus))
	}

	return stdout.String(), stderr.String(), nil
}

// cleanupProofFunctionalInstance removes only the instance allocated by this test.
func cleanupProofFunctionalInstance(t *testing.T, server incusclient.InstanceServer, name string) {
	t.Helper()

	cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelCleanup()
	client, err := newServerClient(server)
	if err != nil {
		t.Errorf("construct cleanup client: %v", err)
		return
	}
	instance, etag, err := client.GetInstance(cleanupContext, name)
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		t.Errorf("get functional proof instance for cleanup: %v", err)
		return
	}
	if !strings.EqualFold(instance.Status, "stopped") {
		if err := client.StopInstance(cleanupContext, name, etag); err != nil && !errors.Is(err, errNotFound) {
			t.Errorf("stop functional proof instance: %v", err)
			return
		}
	}
	if err := client.DeleteInstance(cleanupContext, name); err != nil && !errors.Is(err, errNotFound) {
		t.Errorf("delete functional proof instance: %v", err)
	}
}
