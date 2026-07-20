package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/projectinfo"
)

const (
	defaultConfigPath           = "/etc/incus-gh-runner/config.yaml"
	defaultValidationSocketPath = "/var/lib/incus/unix.socket"
	proofCommandName            = "proof"
)

// BuildInfo describes linker-injected build metadata printed by --version.
type BuildInfo struct {
	// Version is the release version.
	Version string
	// Commit is the source commit used to build the binary.
	Commit string
	// Date is the build timestamp.
	Date string
}

// RunFunc starts the configured controller application.
type RunFunc func(ctx context.Context, cfg config.Config) error

// ValidateFunc validates one rendered baseline against an Incus Unix socket.
type ValidateFunc func(ctx context.Context, baselinePath string, socketPath string) (ValidationResult, error)

// VerifyProofFunc verifies one proof against an enrolled public key and host identity.
type VerifyProofFunc func(
	ctx context.Context,
	proofPath string,
	publicKeyPath string,
	expectedHostID string,
) ([]byte, error)

// ValidationResult describes successful host validation output.
type ValidationResult struct {
	// Notices contains non-fatal security residuals printed to stderr.
	Notices []string
}

// Options customizes root command construction.
type Options struct {
	// In receives interactive command input.
	In io.Reader
	// Out receives machine-readable command output.
	Out io.Writer
	// Err receives diagnostics and human-readable status.
	Err io.Writer
	// Build controls the root command version output.
	Build BuildInfo
	// Viper is the configuration instance used by the command tree.
	Viper *viper.Viper
	// Run starts the controller after configuration is loaded.
	Run RunFunc
	// Validate compares one rendered baseline with live Incus state.
	Validate ValidateFunc
	// VerifyProof authenticates one job machine proof without controller configuration.
	VerifyProof VerifyProofFunc
	// DefaultConfigPath overrides the optional system configuration path.
	DefaultConfigPath string
}

// NewRootCommand creates the incus-gh-runner Cobra command tree.
func NewRootCommand(options Options) *cobra.Command {
	options = options.withDefaults()
	root := &cobra.Command{
		Use:           "incus-gh-runner",
		Short:         projectinfo.Summary(),
		Version:       options.Build.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return initializeConfig(cmd, options.Viper, options.DefaultConfigPath)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(options.Viper)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}

			return options.Run(cmd.Context(), cfg)
		},
	}
	root.SetVersionTemplate(
		fmt.Sprintf(
			"incus-gh-runner %s (%s) built %s\n",
			options.Build.Version,
			options.Build.Commit,
			options.Build.Date,
		),
	)
	root.SetIn(options.In)
	root.SetOut(options.Out)
	root.SetErr(options.Err)
	addConfigFlags(root.Flags())
	root.AddCommand(newValidateCommand(options))
	root.AddCommand(newProofCommand(options))
	return root
}

// withDefaults fills missing command dependencies and build metadata.
func (o Options) withDefaults() Options {
	if o.In == nil {
		o.In = strings.NewReader("")
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Err == nil {
		o.Err = io.Discard
	}
	if o.Viper == nil {
		o.Viper = viper.New()
	}
	if o.Run == nil {
		o.Run = func(context.Context, config.Config) error {
			return errors.New("controller runtime adapters are not implemented")
		}
	}
	if o.Validate == nil {
		o.Validate = func(context.Context, string, string) (ValidationResult, error) {
			return ValidationResult{}, errors.New("incus validator runtime adapter is not implemented")
		}
	}
	if o.VerifyProof == nil {
		o.VerifyProof = func(context.Context, string, string, string) ([]byte, error) {
			return nil, errors.New("job proof verifier runtime adapter is not implemented")
		}
	}
	if o.DefaultConfigPath == "" {
		o.DefaultConfigPath = defaultConfigPath
	}
	o.Build = o.Build.withDefaults()
	return o
}

// newProofCommand creates the job machine proof command group.
func newProofCommand(options Options) *cobra.Command {
	command := &cobra.Command{
		Use:   proofCommandName,
		Short: "Work with job machine provenance proofs",
		Args:  cobra.NoArgs,
	}
	command.AddCommand(newProofVerifyCommand(options))

	return command
}

// newProofVerifyCommand creates the policy-neutral proof verification command.
func newProofVerifyCommand(options Options) *cobra.Command {
	var publicKeyPath string
	var expectedHostID string
	command := &cobra.Command{
		Use:   "verify --public-key <path> --expected-host-id <id> <proof>",
		Short: "Verify a job machine proof",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(publicKeyPath) == "" {
				return errors.New("--public-key is required")
			}
			if strings.TrimSpace(expectedHostID) == "" {
				return errors.New("--expected-host-id is required")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := options.VerifyProof(cmd.Context(), args[0], publicKeyPath, expectedHostID)
			if err != nil {
				return err
			}
			if len(payload) == 0 {
				return errors.New("verified proof payload is empty")
			}
			if _, err := cmd.OutOrStdout().Write(payload); err != nil {
				return fmt.Errorf("write verified proof payload: %w", err)
			}
			if payload[len(payload)-1] != '\n' {
				if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("terminate verified proof payload: %w", err)
				}
			}

			return nil
		},
	}
	command.Flags().StringVar(&publicKeyPath, "public-key", "", "enrolled Ed25519 public key path")
	command.Flags().StringVar(&expectedHostID, "expected-host-id", "", "enrolled controller host identity")

	return command
}

// newValidateCommand creates the read-only Incus baseline validation command.
func newValidateCommand(options Options) *cobra.Command {
	socketPath := defaultValidationSocketPath
	command := &cobra.Command{
		Use:   "validate <baseline>",
		Short: "Validate a rendered baseline against local Incus state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := options.Validate(cmd.Context(), args[0], socketPath)
			if err != nil {
				return err
			}
			for _, notice := range result.Notices {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "NOTICE: %s\n", notice); err != nil {
					return fmt.Errorf("write validation notice: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Incus isolation baseline matches %s\n", args[0]); err != nil {
				return fmt.Errorf("write validation result: %w", err)
			}

			return nil
		},
	}
	command.Flags().StringVar(&socketPath, "socket", socketPath, "local Incus Unix socket path")
	return command
}

// withDefaults fills missing build metadata with development values.
func (b BuildInfo) withDefaults() BuildInfo {
	if strings.TrimSpace(b.Version) == "" {
		b.Version = "dev"
	}
	if strings.TrimSpace(b.Commit) == "" {
		b.Commit = "none"
	}
	if strings.TrimSpace(b.Date) == "" {
		b.Date = "unknown"
	}
	return b
}

// initializeConfig binds all sources and reads the selected configuration file.
func initializeConfig(cmd *cobra.Command, vp *viper.Viper, optionalConfigPath string) error {
	if err := config.ConfigureViper(vp); err != nil {
		return err
	}
	if err := bindConfigFlags(cmd.Flags(), vp); err != nil {
		return err
	}

	explicitConfigPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("read config flag: %w", err)
	}
	configPath := optionalConfigPath
	if explicitConfigPath != "" {
		configPath = explicitConfigPath
	}
	vp.SetConfigFile(configPath)
	contents, err := os.ReadFile(configPath)
	if err != nil {
		if explicitConfigPath == "" && isConfigNotFound(err) {
			return nil
		}
		return fmt.Errorf("read configuration %q: %w", configPath, err)
	}
	if err := config.ValidateYAML(contents); err != nil {
		return fmt.Errorf("validate configuration %q: %w", configPath, err)
	}
	if err := vp.ReadConfig(bytes.NewReader(contents)); err != nil {
		return fmt.Errorf("read configuration %q: %w", configPath, err)
	}

	return nil
}

// addConfigFlags declares the flags that participate in configuration precedence.
func addConfigFlags(flags *pflag.FlagSet) {
	defaults := config.Defaults()
	flags.String("config", "", "configuration file path")
	flags.Int("min-runners", defaults.Capacity.MinRunners, "idle runner floor")
	flags.Int("max-runners", defaults.Capacity.MaxRunners, "runner capacity ceiling")
	flags.Int("incus-operations", defaults.Concurrency.IncusOperations, "concurrent runner operations")
	flags.Duration("reconcile-interval", defaults.ReconcileInterval, "periodic reconciliation interval")
	flags.Duration("operation-timeout", defaults.Timeouts.IncusOperation, "runner operation timeout")
	flags.Duration("shutdown-timeout", defaults.Timeouts.Shutdown, "graceful shutdown timeout")
}

// bindConfigFlags maps CLI flags to their nested configuration keys.
func bindConfigFlags(flags *pflag.FlagSet, vp *viper.Viper) error {
	bindings := map[string]string{
		config.KeyMinRunners:        "min-runners",
		config.KeyMaxRunners:        "max-runners",
		config.KeyIncusOperations:   "incus-operations",
		config.KeyReconcileInterval: "reconcile-interval",
		config.KeyOperationTimeout:  "operation-timeout",
		config.KeyShutdownTimeout:   "shutdown-timeout",
	}
	for key, name := range bindings {
		if err := vp.BindPFlag(key, flags.Lookup(name)); err != nil {
			return fmt.Errorf("bind flag %s: %w", name, err)
		}
	}

	return nil
}

// isConfigNotFound reports whether an optional configuration path is absent.
func isConfigNotFound(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	return errors.As(err, &notFound) || errors.Is(err, fs.ErrNotExist)
}
