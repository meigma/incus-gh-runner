package liveacceptance

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	mutationOptIn   = "I_UNDERSTAND_THIS_PROJECT_IS_DISPOSABLE"
	disposableKey   = "user.incus-gh-runner.disposable"
	acceptanceIDKey = "user.incus-gh-runner.acceptance-id"
	disposableVMKey = "user.incus-gh-runner.acceptance-disposable"
)

var (
	localNamePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Options configures one runtime acceptance probe against two already-running runner VMs.
type Options struct {
	// BaselinePath identifies the rendered CUE baseline validated before probing.
	BaselinePath string
	// Project is the disposable restricted project containing the runner VMs.
	Project string
	// Profile is the sole hardened profile attached to both runner VMs.
	Profile string
	// ImageFingerprint is the immutable local image fingerprint used by both runner VMs.
	ImageFingerprint string
	// VMA is the first marker-owned runner VM and resource aggressor.
	VMA string
	// VMB is the second marker-owned runner VM and availability victim.
	VMB string
	// RunID is the parent harness identity recorded on every disposable instance.
	RunID string
	// AllowedURL is one credential-free HTTP origin used for recovery probes.
	AllowedURL string
	// EgressProxy is the optional credential-free proxy origin used for recovery probes.
	EgressProxy string
	// EvidenceDirectory is a new directory populated with probe evidence.
	EvidenceDirectory string
	// StressDuration is the steady-state guest pressure and watchdog interval.
	StressDuration time.Duration
	// PollInterval controls API, peer, and host-memory watchdog sampling.
	PollInterval time.Duration
	// SelfPath is the acceptance executable copied through the Incus agent into the aggressor VM.
	SelfPath string
}

// withDefaults fills bounded runtime probe defaults.
func (o Options) withDefaults() Options {
	if o.StressDuration == 0 {
		o.StressDuration = defaultStressDuration
	}
	if o.PollInterval == 0 {
		o.PollInterval = defaultPollInterval
	}
	return o
}

// validate checks that the probe remains bound to the disposable baseline and safe local operands.
//
//nolint:gocognit // Fail-closed operand checks are intentionally linear.
func (o Options) validate(
	baseline RuntimeBaseline,
) error {
	if baseline.MaximumRunners != acceptanceRunnerCount {
		return fmt.Errorf(
			"runtime probe requires an acceptance baseline with exactly two runners; got %d",
			baseline.MaximumRunners,
		)
	}
	if o.Project != baseline.Manifest.Names.Project {
		return errors.New("project does not match the rendered baseline")
	}
	if o.Profile != baseline.Manifest.Names.Profile {
		return errors.New("profile does not match the rendered baseline")
	}
	if o.Project == "default" {
		return errors.New("runtime probe refuses the default project")
	}
	for label, value := range map[string]string{
		"project": o.Project,
		"profile": o.Profile,
		"VM A":    o.VMA,
		"VM B":    o.VMB,
		"run ID":  o.RunID,
	} {
		if !localNamePattern.MatchString(value) {
			return fmt.Errorf("%s must be a safe local name", label)
		}
	}
	if o.VMA == o.VMB {
		return errors.New("runtime probe requires two different runner VMs")
	}
	if !fingerprintPattern.MatchString(o.ImageFingerprint) {
		return errors.New("image fingerprint must be exactly 64 lowercase hexadecimal characters")
	}
	if err := validateOrigin("allowed URL", o.AllowedURL); err != nil {
		return err
	}
	if o.EgressProxy != "" {
		if err := validateOrigin("egress proxy", o.EgressProxy); err != nil {
			return err
		}
	}
	if o.StressDuration < minimumStressDuration || o.StressDuration > maximumStressDuration {
		return errors.New("stress duration must be between 10 and 15 minutes")
	}
	if o.PollInterval < minimumPollInterval || o.PollInterval > maximumPollInterval {
		return errors.New("poll interval must be between 100 milliseconds and 10 seconds")
	}
	if o.PollInterval > o.StressDuration/2 {
		return errors.New("poll interval must allow at least two stress samples")
	}
	if err := validateExecutable(o.SelfPath); err != nil {
		return err
	}
	if strings.TrimSpace(o.EvidenceDirectory) == "" {
		return errors.New("evidence directory is required")
	}
	if info, err := os.Lstat(o.EvidenceDirectory); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("evidence directory must not be a symbolic link")
		}
		return errors.New("evidence directory must not already exist")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect evidence directory: %w", err)
	}

	return nil
}

// validateOrigin accepts only credential-free HTTP origins safe to retain as evidence.
func validateOrigin(label string, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%s must be an HTTP(S) origin", label)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("%s must not contain user information, a path, query, or fragment", label)
	}
	if strings.ContainsAny(value, "\r\n\t ") {
		return fmt.Errorf("%s must not contain whitespace", label)
	}

	return nil
}

// validateExecutable requires one resolved, regular executable without following a final symlink.
func validateExecutable(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("acceptance executable path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve acceptance executable: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("inspect acceptance executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("acceptance executable must be a non-symlink regular executable")
	}

	return nil
}
