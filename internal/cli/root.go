package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/projectinfo"
)

const defaultConfigPath = "/etc/incus-gh-runner/config.yaml"

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
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
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
	return root
}

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
	if o.DefaultConfigPath == "" {
		o.DefaultConfigPath = defaultConfigPath
	}
	o.Build = o.Build.withDefaults()
	return o
}

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
	if err := vp.ReadInConfig(); err != nil {
		if explicitConfigPath == "" && isConfigNotFound(err) {
			return nil
		}
		return fmt.Errorf("read configuration %q: %w", configPath, err)
	}

	return nil
}

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

func isConfigNotFound(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	return errors.As(err, &notFound) || errors.Is(err, fs.ErrNotExist)
}
