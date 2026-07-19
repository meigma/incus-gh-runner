// Package config loads and validates immutable controller configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	defaultMaxRunners        = 1
	defaultIncusOperations   = 2
	defaultReconcileInterval = time.Second
	defaultOperationTimeout  = 5 * time.Minute
	defaultShutdownTimeout   = 30 * time.Second
	defaultBootstrapTimeout  = 5 * time.Minute
	defaultRetryInitial      = time.Second
	defaultRetryMaximum      = 30 * time.Second
	defaultRunnerGroup       = "default"

	// KeyMinRunners identifies the configured idle capacity floor.
	KeyMinRunners = "capacity.min_runners"
	// KeyMaxRunners identifies the configured capacity ceiling.
	KeyMaxRunners = "capacity.max_runners"
	// KeyIncusOperations identifies the backend worker limit.
	KeyIncusOperations = "concurrency.incus_operations"
	// KeyReconcileInterval identifies the periodic reconciliation interval.
	KeyReconcileInterval = "reconcile_interval"
	// KeyOperationTimeout identifies the lifecycle operation timeout.
	KeyOperationTimeout = "timeouts.incus_operation"
	// KeyShutdownTimeout identifies the graceful shutdown timeout.
	KeyShutdownTimeout = "timeouts.shutdown"
	// KeyRetryInitial identifies the first transient-failure retry delay.
	KeyRetryInitial = "retry.initial"
	// KeyRetryMaximum identifies the capped transient-failure retry delay.
	KeyRetryMaximum = "retry.maximum"
	// KeyGitHubConfigURL identifies the repository or organization registration URL.
	KeyGitHubConfigURL = "github.config_url"
	// KeyGitHubScaleSet identifies the persistent runner scale-set name.
	KeyGitHubScaleSet = "github.scale_set"
	// KeyGitHubRunnerGroup identifies the runner group containing the scale set.
	KeyGitHubRunnerGroup = "github.runner_group"
	// KeyGitHubAppClientID identifies the GitHub App client ID.
	KeyGitHubAppClientID = "github.app.client_id"
	// KeyGitHubAppInstallationID identifies the GitHub App installation ID.
	KeyGitHubAppInstallationID = "github.app.installation_id"
	// KeyGitHubAppPrivateKeyFile identifies the protected GitHub App private-key path.
	KeyGitHubAppPrivateKeyFile = "github.app.private_key_file"
	// KeyGitHubTokenFile identifies the protected personal access token path.
	KeyGitHubTokenFile = "github.token_file" //nolint:gosec // This is a configuration key, not a credential.
	// KeyIncusSocket identifies an optional Incus Unix socket path.
	KeyIncusSocket = "incus.socket"
	// KeyIncusProject identifies the preconfigured Incus project.
	KeyIncusProject = "incus.project"
	// KeyIncusImage identifies the existing runner image alias or fingerprint.
	KeyIncusImage = "incus.image"
	// KeyIncusOwner identifies the exact durable instance ownership marker.
	KeyIncusOwner = "incus.owner"
	// KeyIncusBootstrapTimeout identifies the guest readiness deadline.
	KeyIncusBootstrapTimeout = "incus.bootstrap_timeout"
	// KeyIncusDiagnosticsDir identifies the directory for terminal runner evidence.
	KeyIncusDiagnosticsDir = "incus.diagnostics_dir"

	// EnvGitHubToken is the environment-only PAT credential.
	EnvGitHubToken = "INCUS_GH_RUNNER_GITHUB_TOKEN" //nolint:gosec // This is an environment variable name, not a credential.
	// EnvGitHubTokenFile identifies the protected PAT path supplied through the environment.
	EnvGitHubTokenFile = "INCUS_GH_RUNNER_GITHUB_TOKEN_FILE" //nolint:gosec // This is an environment variable name, not a credential.
)

// Config contains immutable controller settings.
type Config struct {
	// GitHub configures one persistent runner scale set and its credentials.
	GitHub GitHub `mapstructure:"github"`
	// Incus configures the pre-existing environment used for runner VMs.
	Incus Incus `mapstructure:"incus"`
	// Capacity controls the minimum and maximum owned runner counts.
	Capacity Capacity `mapstructure:"capacity"`
	// Concurrency bounds external lifecycle operations.
	Concurrency Concurrency `mapstructure:"concurrency"`
	// ReconcileInterval controls the periodic safety reconciliation tick.
	ReconcileInterval time.Duration `mapstructure:"reconcile_interval"`
	// Timeouts bounds lifecycle operations and shutdown.
	Timeouts Timeouts `mapstructure:"timeouts"`
	// Retry controls transient GitHub and Incus operation backoff.
	Retry Retry `mapstructure:"retry"`
}

// GitHub contains the runner scale-set registration and credential settings.
type GitHub struct {
	// ConfigURL is the repository or organization URL that owns the scale set.
	ConfigURL string `mapstructure:"config_url"`
	// ScaleSet is the persistent runner scale-set name and default runner label.
	ScaleSet string `mapstructure:"scale_set"`
	// RunnerGroup is the existing GitHub runner group containing the scale set.
	RunnerGroup string `mapstructure:"runner_group"`
	// App contains preferred GitHub App credentials.
	App GitHubApp `mapstructure:"app"`
	// Token contains the environment-only PAT and is never decoded from YAML.
	Token string `mapstructure:"-"`
	// TokenFile is a protected PAT path read once during runtime startup.
	TokenFile string `mapstructure:"token_file"`
}

// GitHubApp identifies a GitHub App installation and protected private key.
type GitHubApp struct {
	// ClientID is the GitHub App client ID or application ID.
	ClientID string `mapstructure:"client_id"`
	// InstallationID is the GitHub App installation ID.
	InstallationID int64 `mapstructure:"installation_id"`
	// PrivateKeyFile is a protected PEM file read only during startup.
	PrivateKeyFile string `mapstructure:"private_key_file"`
}

// Incus contains references to the preconfigured runner environment.
type Incus struct {
	// Socket optionally selects a non-default local Incus Unix socket.
	Socket string `mapstructure:"socket"`
	// Project is the existing Incus project used for runner VMs.
	Project string `mapstructure:"project"`
	// Image is an existing local runner image alias or fingerprint.
	Image string `mapstructure:"image"`
	// Profiles are existing profiles applied to every runner VM.
	Profiles []string `mapstructure:"profiles"`
	// Owner is the exact durable marker authorizing runner instance mutation.
	Owner string `mapstructure:"owner"`
	// BootstrapTimeout bounds how long an unready VM counts as capacity.
	BootstrapTimeout time.Duration `mapstructure:"bootstrap_timeout"`
	// DiagnosticsDir optionally stores terminal serial-console evidence before deletion.
	DiagnosticsDir string `mapstructure:"diagnostics_dir"`
}

// Capacity contains desired runner capacity limits.
type Capacity struct {
	// MinRunners is the idle runner floor.
	MinRunners int `mapstructure:"min_runners"`
	// MaxRunners is the hard runner ceiling.
	MaxRunners int `mapstructure:"max_runners"`
}

// Concurrency contains external operation limits.
type Concurrency struct {
	// IncusOperations is the maximum number of concurrent backend operations.
	IncusOperations int `mapstructure:"incus_operations"`
}

// Timeouts contains bounded lifecycle durations.
type Timeouts struct {
	// IncusOperation bounds one backend lifecycle operation.
	IncusOperation time.Duration `mapstructure:"incus_operation"`
	// Shutdown allows in-flight operations to finish before cancellation.
	Shutdown time.Duration `mapstructure:"shutdown"`
}

// Retry contains shared bounded transient-failure retry settings.
type Retry struct {
	// Initial is the first delay after a transient external operation failure.
	Initial time.Duration `mapstructure:"initial"`
	// Maximum caps delay growth across consecutive external operation failures.
	Maximum time.Duration `mapstructure:"maximum"`
}

// ConfigureViper installs defaults and explicit environment bindings.
func ConfigureViper(vp *viper.Viper) error {
	defaultConfig := Defaults()
	defaults := map[string]any{
		KeyMinRunners:            defaultConfig.Capacity.MinRunners,
		KeyMaxRunners:            defaultConfig.Capacity.MaxRunners,
		KeyIncusOperations:       defaultConfig.Concurrency.IncusOperations,
		KeyReconcileInterval:     defaultConfig.ReconcileInterval,
		KeyOperationTimeout:      defaultConfig.Timeouts.IncusOperation,
		KeyShutdownTimeout:       defaultConfig.Timeouts.Shutdown,
		KeyRetryInitial:          defaultConfig.Retry.Initial,
		KeyRetryMaximum:          defaultConfig.Retry.Maximum,
		KeyGitHubRunnerGroup:     defaultConfig.GitHub.RunnerGroup,
		KeyIncusBootstrapTimeout: defaultConfig.Incus.BootstrapTimeout,
	}
	for key, value := range defaults {
		vp.SetDefault(key, value)
	}

	environment := map[string]string{
		KeyMinRunners:              "INCUS_GH_RUNNER_CAPACITY_MIN_RUNNERS",
		KeyMaxRunners:              "INCUS_GH_RUNNER_CAPACITY_MAX_RUNNERS",
		KeyIncusOperations:         "INCUS_GH_RUNNER_CONCURRENCY_INCUS_OPERATIONS",
		KeyReconcileInterval:       "INCUS_GH_RUNNER_RECONCILE_INTERVAL",
		KeyOperationTimeout:        "INCUS_GH_RUNNER_TIMEOUTS_INCUS_OPERATION",
		KeyShutdownTimeout:         "INCUS_GH_RUNNER_TIMEOUTS_SHUTDOWN",
		KeyRetryInitial:            "INCUS_GH_RUNNER_RETRY_INITIAL",
		KeyRetryMaximum:            "INCUS_GH_RUNNER_RETRY_MAXIMUM",
		KeyGitHubConfigURL:         "INCUS_GH_RUNNER_GITHUB_CONFIG_URL",
		KeyGitHubScaleSet:          "INCUS_GH_RUNNER_GITHUB_SCALE_SET",
		KeyGitHubRunnerGroup:       "INCUS_GH_RUNNER_GITHUB_RUNNER_GROUP",
		KeyGitHubAppClientID:       "INCUS_GH_RUNNER_GITHUB_APP_CLIENT_ID",
		KeyGitHubAppInstallationID: "INCUS_GH_RUNNER_GITHUB_APP_INSTALLATION_ID",
		KeyGitHubAppPrivateKeyFile: "INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE",
		KeyGitHubTokenFile:         EnvGitHubTokenFile,
		KeyIncusSocket:             "INCUS_GH_RUNNER_INCUS_SOCKET",
		KeyIncusProject:            "INCUS_GH_RUNNER_INCUS_PROJECT",
		KeyIncusImage:              "INCUS_GH_RUNNER_INCUS_IMAGE",
		KeyIncusOwner:              "INCUS_GH_RUNNER_INCUS_OWNER",
		KeyIncusBootstrapTimeout:   "INCUS_GH_RUNNER_INCUS_BOOTSTRAP_TIMEOUT",
		KeyIncusDiagnosticsDir:     "INCUS_GH_RUNNER_INCUS_DIAGNOSTICS_DIR",
	}
	for key, name := range environment {
		if err := vp.BindEnv(key, name); err != nil {
			return fmt.Errorf("bind environment variable %s: %w", name, err)
		}
	}

	return nil
}

// Defaults returns the controller defaults.
func Defaults() Config {
	return Config{
		GitHub: GitHub{RunnerGroup: defaultRunnerGroup},
		Incus:  Incus{BootstrapTimeout: defaultBootstrapTimeout},
		Capacity: Capacity{
			MinRunners: 0,
			MaxRunners: defaultMaxRunners,
		},
		Concurrency:       Concurrency{IncusOperations: defaultIncusOperations},
		ReconcileInterval: defaultReconcileInterval,
		Timeouts: Timeouts{
			IncusOperation: defaultOperationTimeout,
			Shutdown:       defaultShutdownTimeout,
		},
		Retry: Retry{
			Initial: defaultRetryInitial,
			Maximum: defaultRetryMaximum,
		},
	}
}

// Load decodes and validates runtime settings from Viper.
func Load(vp *viper.Viper) (Config, error) {
	var cfg Config
	if err := vp.UnmarshalExact(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	cfg.GitHub.Token = strings.TrimSpace(os.Getenv(EnvGitHubToken))
	cfg.GitHub.TokenFile = strings.TrimSpace(cfg.GitHub.TokenFile)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// ValidateRuntime checks the external adapter settings needed by the executable.
func (c Config) ValidateRuntime() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := validateGitHub(c.GitHub); err != nil {
		return err
	}
	return validateIncus(c.Incus)
}

// validateGitHub checks scale-set identity and exactly one complete credential type.
func validateGitHub(settings GitHub) error {
	if err := validateGitHubScheduling(settings); err != nil {
		return err
	}

	appConfigured := settings.App.ClientID != "" ||
		settings.App.InstallationID != 0 ||
		settings.App.PrivateKeyFile != ""
	tokenValueConfigured := settings.Token != ""
	tokenFileConfigured := strings.TrimSpace(settings.TokenFile) != ""
	if tokenValueConfigured && tokenFileConfigured {
		return errors.New("configure either github.token_file or INCUS_GH_RUNNER_GITHUB_TOKEN, not both")
	}
	tokenConfigured := tokenValueConfigured || tokenFileConfigured
	if appConfigured && tokenConfigured {
		return errors.New("configure either github.app or a personal access token, not both")
	}
	if !appConfigured && !tokenConfigured {
		return errors.New("github credentials are required")
	}
	if !appConfigured {
		return nil
	}
	if strings.TrimSpace(settings.App.ClientID) == "" {
		return errors.New("github.app.client_id is required")
	}
	if settings.App.InstallationID <= 0 {
		return errors.New("github.app.installation_id must be positive")
	}
	if strings.TrimSpace(settings.App.PrivateKeyFile) == "" {
		return errors.New("github.app.private_key_file is required")
	}

	return nil
}

// validateGitHubScheduling checks the scope and identifiers passed to the upstream client.
func validateGitHubScheduling(settings GitHub) error {
	scope, err := validateConfigURL(settings.ConfigURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(settings.ScaleSet) == "" {
		return errors.New("github.scale_set is required")
	}
	if strings.TrimSpace(settings.ScaleSet) != settings.ScaleSet ||
		!githubQueryValueRoundTrips("name", settings.ScaleSet) {
		return errors.New("github.scale_set contains characters that are unsafe in GitHub API queries")
	}
	runnerGroup := strings.TrimSpace(settings.RunnerGroup)
	if runnerGroup == "" {
		return errors.New("github.runner_group is required")
	}
	if scope == configURLScopeRepository && settings.RunnerGroup != defaultRunnerGroup {
		return errors.New("github.runner_group must be default for repository scope")
	}
	if scope == configURLScopeOrganization && strings.EqualFold(runnerGroup, defaultRunnerGroup) {
		return errors.New(
			"github.runner_group must name a non-default runner group for organization scope",
		)
	}
	if runnerGroup != settings.RunnerGroup ||
		!githubQueryValueRoundTrips("groupName", settings.RunnerGroup) {
		return errors.New("github.runner_group contains characters that are unsafe in GitHub API queries")
	}
	return nil
}

// validateIncus checks preconfigured environment references and lifecycle settings.
func validateIncus(settings Incus) error {
	if strings.TrimSpace(settings.Project) == "" {
		return errors.New("incus.project is required")
	}
	if strings.TrimSpace(settings.Image) == "" {
		return errors.New("incus.image is required")
	}
	if strings.TrimSpace(settings.Owner) == "" {
		return errors.New("incus.owner is required")
	}
	if settings.BootstrapTimeout <= 0 {
		return errors.New("incus.bootstrap_timeout must be positive")
	}
	for _, profile := range settings.Profiles {
		if strings.TrimSpace(profile) == "" {
			return errors.New("incus.profiles must not contain empty names")
		}
	}

	return nil
}

// configURLScope identifies the scheduling boundary encoded in a GitHub configuration URL.
type configURLScope uint8

const (
	configURLScopeOrganization configURLScope = iota + 1
	configURLScopeRepository
)

// validateConfigURL checks and classifies an HTTPS GitHub registration target.
func validateConfigURL(raw string) (configURLScope, error) {
	const (
		message             = "github.config_url must be an absolute HTTPS GitHub organization or repository URL"
		repositoryPathParts = 2
	)

	trimmed := strings.TrimSpace(raw)
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || trimmed != raw || parsed.Opaque != "" || !strings.EqualFold(parsed.Scheme, "https") ||
		parsed.Hostname() == "" || strings.HasSuffix(parsed.Hostname(), ".") || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawPath != "" {
		return 0, errors.New(message)
	}
	hasPort, validPort := configURLPort(parsed.Host)
	if !validPort || (hasPort && isHostedGitHubHostname(parsed.Hostname())) {
		return 0, errors.New(message)
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	if !strings.HasPrefix(path, "/") {
		return 0, errors.New(message)
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return 0, errors.New(message)
		}
	}

	switch len(parts) {
	case 1:
		return configURLScopeOrganization, nil
	case repositoryPathParts:
		if strings.EqualFold(parts[0], "enterprises") {
			return 0, errors.New(message)
		}
		return configURLScopeRepository, nil
	default:
		return 0, errors.New(message)
	}
}

// isHostedGitHubHostname mirrors the pinned upstream client's hosted-domain classification without a port.
func isHostedGitHubHostname(hostname string) bool {
	normalized := strings.ToLower(hostname)
	return normalized == "github.com" || normalized == "www.github.com" ||
		normalized == "github.localhost" || strings.HasSuffix(normalized, ".ghe.com")
}

// githubQueryValueRoundTrips checks values interpolated into the pinned upstream client's raw query strings.
func githubQueryValueRoundTrips(key string, value string) bool {
	parsed, err := url.Parse("/?" + key + "=" + value)
	if err != nil || parsed.Fragment != "" {
		return false
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values) != 1 {
		return false
	}
	resolved, ok := values[key]
	return ok && len(resolved) == 1 && resolved[0] == value
}

// configURLPort reports and validates an explicitly configured TCP port.
func configURLPort(host string) (bool, bool) {
	if strings.HasPrefix(host, "[") {
		return bracketedConfigURLPort(host)
	}
	if strings.Count(host, ":") > 1 {
		return false, false
	}
	separator := strings.LastIndexByte(host, ':')
	if separator < 0 {
		return false, true
	}
	return validConfigURLPort(host[separator+1:])
}

// bracketedConfigURLPort extracts an optional port after an IPv6 host literal.
func bracketedConfigURLPort(host string) (bool, bool) {
	closingBracket := strings.LastIndexByte(host, ']')
	if closingBracket < 0 {
		return false, false
	}
	if closingBracket == len(host)-1 {
		return false, true
	}
	if host[closingBracket+1] != ':' {
		return false, false
	}
	return validConfigURLPort(host[closingBracket+2:])
}

// validConfigURLPort checks a required numeric TCP port.
func validConfigURLPort(port string) (bool, bool) {
	if port == "" {
		return true, false
	}
	for _, character := range port {
		if character < '0' || character > '9' {
			return true, false
		}
	}
	parsed, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsed == 0 {
		return true, false
	}
	return true, true
}

// Validate checks controller configuration invariants.
func (c Config) Validate() error {
	if c.Capacity.MinRunners < 0 {
		return errors.New("capacity.min_runners must not be negative")
	}
	if c.Capacity.MaxRunners < c.Capacity.MinRunners {
		return errors.New("capacity.max_runners must be at least capacity.min_runners")
	}
	if c.Concurrency.IncusOperations < 1 {
		return errors.New("concurrency.incus_operations must be positive")
	}
	if c.ReconcileInterval <= 0 {
		return errors.New("reconcile_interval must be positive")
	}
	if c.Timeouts.IncusOperation <= 0 {
		return errors.New("timeouts.incus_operation must be positive")
	}
	if c.Timeouts.Shutdown <= 0 {
		return errors.New("timeouts.shutdown must be positive")
	}
	if c.Retry.Initial <= 0 {
		return errors.New("retry.initial must be positive")
	}
	if c.Retry.Maximum < c.Retry.Initial {
		return errors.New("retry.maximum must be at least retry.initial")
	}

	return nil
}
