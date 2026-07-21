// Package provenance creates and verifies job-bound machine provenance receipts.
package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Version is the only job machine proof schema version currently supported.
	Version = 1
	// Claim identifies the exact statement made by a version 1 receipt.
	Claim = "github_job_started_for_jit_runner_provisioned_to_incus_vm"
	// PayloadType binds DSSE signatures to version 1 job machine proof payloads.
	PayloadType = "application/vnd.meigma.incus-gh-runner.job-provenance.v1+json"
	// MaximumEnvelopeBytes bounds one encoded DSSE envelope.
	MaximumEnvelopeBytes = 64 * 1024
	// MaximumPayloadBytes bounds one decoded job machine proof payload.
	MaximumPayloadBytes = 64 * 1024
	// MaximumIdentityBytes bounds every mandatory string identity field.
	MaximumIdentityBytes = 4 * 1024

	// LaunchInstanceType is the fixed Incus instance type covered by version 1.
	LaunchInstanceType = "virtual-machine"
	digestHexLength    = sha256.Size * 2
)

// Payload is the signed version 1 job-bound machine provenance statement.
type Payload struct {
	// Version selects the receipt schema.
	Version int `json:"version"`
	// Claim identifies the statement made by the receipt.
	Claim string `json:"claim"`
	// IssuedAt is the controller host time at signing.
	IssuedAt time.Time `json:"issued_at"`
	// Host identifies the enrolled controller installation.
	Host Host `json:"host"`
	// GitHub identifies the authenticated job-start event and runner registration.
	GitHub GitHub `json:"github"`
	// Machine identifies the owned Incus VM and pinned launch inputs.
	Machine Machine `json:"machine"`
}

// Host identifies the controller that issued a receipt.
type Host struct {
	// ID is the stable operator-enrolled host identity.
	ID string `json:"id"`
	// ControllerVersion is the running controller release version.
	ControllerVersion string `json:"controller_version"`
	// ControllerCommit is the running controller source revision.
	ControllerCommit string `json:"controller_commit"`
}

// GitHub identifies one authenticated GitHub job-start event.
type GitHub struct {
	// Owner is the repository owner reported by GitHub.
	Owner string `json:"owner"`
	// Repository is the repository name reported by GitHub.
	Repository string `json:"repository"`
	// WorkflowRef is the opaque workflow reference reported by GitHub.
	WorkflowRef string `json:"workflow_ref"`
	// WorkflowRunID is the positive workflow run identifier reported by GitHub.
	WorkflowRunID int64 `json:"workflow_run_id"`
	// JobID is the opaque job identifier reported by GitHub.
	JobID string `json:"job_id"`
	// RunnerRequestID is the runner request identifier reported by GitHub.
	// GitHub may report zero for JobStarted messages in live scale-set sessions.
	RunnerRequestID int64 `json:"runner_request_id"`
	// RunnerID is the positive JIT runner registration identifier.
	RunnerID int64 `json:"runner_id"`
	// RunnerName is the JIT runner name assigned to the VM.
	RunnerName string `json:"runner_name"`
	// EventName is the workflow event reported by GitHub.
	EventName string `json:"event_name"`
	// ScaleSetID is the positive resolved runner scale-set identifier.
	ScaleSetID int64 `json:"scale_set_id"`
	// ScaleSetName is the resolved runner scale-set name.
	ScaleSetName string `json:"scale_set_name"`
}

// Machine identifies the Incus VM and launch inputs bound to a receipt.
type Machine struct {
	// IncusProject is the configured Incus project containing the VM.
	IncusProject string `json:"incus_project"`
	// InstanceName is the requested and observed Incus instance name.
	InstanceName string `json:"instance_name"`
	// InstanceUUID is the server-generated Incus instance UUID.
	InstanceUUID string `json:"instance_uuid"`
	// ImageFingerprint is the full pinned image SHA-256 fingerprint.
	ImageFingerprint string `json:"image_fingerprint"`
	// LaunchConfigurationSHA256 identifies the exact version 1 launch input bytes.
	LaunchConfigurationSHA256 string `json:"launch_configuration_sha256"`
	// Profiles is the ordered set of pinned profile identities.
	Profiles []Profile `json:"profiles"`
}

// Profile identifies one ordered Incus profile input.
type Profile struct {
	// Name is the operator-facing profile name.
	Name string `json:"name"`
	// SHA256 identifies the profile configuration and devices.
	SHA256 string `json:"sha256"`
}

// LaunchInput is the exact JSON object hashed for a version 1 VM launch identity.
type LaunchInput struct {
	// Version selects the launch-input schema.
	Version int `json:"version"`
	// InstanceType is fixed to virtual-machine for version 1.
	InstanceType string `json:"instance_type"`
	// ImageFingerprint is the full resolved image SHA-256 fingerprint.
	ImageFingerprint string `json:"image_fingerprint"`
	// Profiles is the ordered set of pinned profile identities.
	Profiles []Profile `json:"profiles"`
	// Config is the effective pre-metadata instance configuration.
	Config map[string]string `json:"config"`
	// Devices is the effective pre-metadata instance device configuration.
	Devices map[string]map[string]string `json:"devices"`
}

// Validate checks all version 1 payload invariants before signing or after verification.
func (p Payload) Validate() error {
	if p.Version != Version {
		return fmt.Errorf("proof version must be %d", Version)
	}
	if p.Claim != Claim {
		return errors.New("proof claim is not supported")
	}
	if p.IssuedAt.IsZero() {
		return errors.New("proof issued_at is required")
	}
	stringsToValidate := []struct {
		name  string
		value string
	}{
		{name: "host.id", value: p.Host.ID},
		{name: "host.controller_version", value: p.Host.ControllerVersion},
		{name: "host.controller_commit", value: p.Host.ControllerCommit},
		{name: "github.owner", value: p.GitHub.Owner},
		{name: "github.repository", value: p.GitHub.Repository},
		{name: "github.workflow_ref", value: p.GitHub.WorkflowRef},
		{name: "github.job_id", value: p.GitHub.JobID},
		{name: "github.runner_name", value: p.GitHub.RunnerName},
		{name: "github.event_name", value: p.GitHub.EventName},
		{name: "github.scale_set_name", value: p.GitHub.ScaleSetName},
		{name: "machine.incus_project", value: p.Machine.IncusProject},
		{name: "machine.instance_name", value: p.Machine.InstanceName},
		{name: "machine.instance_uuid", value: p.Machine.InstanceUUID},
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
		{name: "github.workflow_run_id", value: p.GitHub.WorkflowRunID},
		{name: "github.runner_id", value: p.GitHub.RunnerID},
		{name: "github.scale_set_id", value: p.GitHub.ScaleSetID},
	}
	if p.GitHub.RunnerRequestID < 0 {
		return errors.New("github.runner_request_id must not be negative")
	}
	for _, field := range positiveIDs {
		if field.value <= 0 {
			return fmt.Errorf("%s must be positive", field.name)
		}
	}
	if p.GitHub.RunnerName != p.Machine.InstanceName {
		return errors.New("github.runner_name must equal machine.instance_name")
	}
	if err := validateDigest("machine.image_fingerprint", p.Machine.ImageFingerprint); err != nil {
		return err
	}
	if err := validateDigest("machine.launch_configuration_sha256", p.Machine.LaunchConfigurationSHA256); err != nil {
		return err
	}
	if p.Machine.Profiles == nil {
		return errors.New("machine.profiles must be an array")
	}
	for index, profile := range p.Machine.Profiles {
		if err := validateProfile(fmt.Sprintf("machine.profiles[%d]", index), profile); err != nil {
			return err
		}
	}

	return nil
}

// LaunchBytes returns the normative JSON encoding of one validated launch input.
func LaunchBytes(input LaunchInput) ([]byte, error) {
	if err := validateLaunchInput(input); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode launch input: %w", err)
	}

	return encoded, nil
}

// LaunchDigest returns the lowercase SHA-256 digest of the normative launch JSON.
func LaunchDigest(input LaunchInput) (string, error) {
	encoded, err := LaunchBytes(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:]), nil
}

// ProfileDigest preserves the existing JSON and SHA-256 profile identity format.
func ProfileDigest(config map[string]string, devices map[string]map[string]string) (string, error) {
	encoded, err := json.Marshal(struct {
		Config  map[string]string            `json:"config"`
		Devices map[string]map[string]string `json:"devices"`
	}{Config: config, Devices: devices})
	if err != nil {
		return "", fmt.Errorf("encode profile input: %w", err)
	}
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:]), nil
}

// validateLaunchInput checks the fixed schema before producing digest bytes.
func validateLaunchInput(input LaunchInput) error {
	if input.Version != Version {
		return fmt.Errorf("launch input version must be %d", Version)
	}
	if input.InstanceType != LaunchInstanceType {
		return fmt.Errorf("launch instance_type must be %q", LaunchInstanceType)
	}
	if err := validateDigest("launch image_fingerprint", input.ImageFingerprint); err != nil {
		return err
	}
	if input.Profiles == nil {
		return errors.New("launch profiles must be an array")
	}
	for index, profile := range input.Profiles {
		if err := validateProfile(fmt.Sprintf("launch profiles[%d]", index), profile); err != nil {
			return err
		}
	}
	if input.Config == nil {
		return errors.New("launch config must be an object")
	}
	if input.Devices == nil {
		return errors.New("launch devices must be an object")
	}

	return nil
}

// validateProfile checks one named profile digest.
func validateProfile(path string, profile Profile) error {
	if err := validateIdentity(path+".name", profile.Name); err != nil {
		return err
	}
	return validateDigest(path+".sha256", profile.SHA256)
}

// validateIdentity checks one bounded, non-blank identity without echoing its value.
func validateIdentity(path string, value string) error {
	if value == "" || strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", path)
	}
	if len(value) > MaximumIdentityBytes {
		return fmt.Errorf("%s exceeds %d bytes", path, MaximumIdentityBytes)
	}

	return nil
}

// validateDigest checks one lowercase SHA-256 hexadecimal identity.
func validateDigest(path string, value string) error {
	if len(value) != digestHexLength {
		return fmt.Errorf("%s must contain %d lowercase hexadecimal characters", path, digestHexLength)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || strings.ToLower(value) != value || len(decoded) != sha256.Size {
		return fmt.Errorf("%s must contain %d lowercase hexadecimal characters", path, digestHexLength)
	}

	return nil
}
