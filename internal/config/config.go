// Package config loads and validates immutable controller configuration.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

const (
	defaultMaxRunners        = 1
	defaultIncusOperations   = 2
	defaultReconcileInterval = time.Second
	defaultOperationTimeout  = 5 * time.Minute
	defaultShutdownTimeout   = 30 * time.Second

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
)

// Config contains immutable phase 1 controller settings.
type Config struct {
	// Capacity controls the minimum and maximum owned runner counts.
	Capacity Capacity `mapstructure:"capacity"`
	// Concurrency bounds external lifecycle operations.
	Concurrency Concurrency `mapstructure:"concurrency"`
	// ReconcileInterval controls the periodic safety reconciliation tick.
	ReconcileInterval time.Duration `mapstructure:"reconcile_interval"`
	// Timeouts bounds lifecycle operations and shutdown.
	Timeouts Timeouts `mapstructure:"timeouts"`
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

// ConfigureViper installs defaults and explicit environment bindings.
func ConfigureViper(vp *viper.Viper) error {
	defaultConfig := Defaults()
	defaults := map[string]any{
		KeyMinRunners:        defaultConfig.Capacity.MinRunners,
		KeyMaxRunners:        defaultConfig.Capacity.MaxRunners,
		KeyIncusOperations:   defaultConfig.Concurrency.IncusOperations,
		KeyReconcileInterval: defaultConfig.ReconcileInterval,
		KeyOperationTimeout:  defaultConfig.Timeouts.IncusOperation,
		KeyShutdownTimeout:   defaultConfig.Timeouts.Shutdown,
	}
	for key, value := range defaults {
		vp.SetDefault(key, value)
	}

	environment := map[string]string{
		KeyMinRunners:        "INCUS_GH_RUNNER_CAPACITY_MIN_RUNNERS",
		KeyMaxRunners:        "INCUS_GH_RUNNER_CAPACITY_MAX_RUNNERS",
		KeyIncusOperations:   "INCUS_GH_RUNNER_CONCURRENCY_INCUS_OPERATIONS",
		KeyReconcileInterval: "INCUS_GH_RUNNER_RECONCILE_INTERVAL",
		KeyOperationTimeout:  "INCUS_GH_RUNNER_TIMEOUTS_INCUS_OPERATION",
		KeyShutdownTimeout:   "INCUS_GH_RUNNER_TIMEOUTS_SHUTDOWN",
	}
	for key, name := range environment {
		if err := vp.BindEnv(key, name); err != nil {
			return fmt.Errorf("bind environment variable %s: %w", name, err)
		}
	}

	return nil
}

// Defaults returns the phase 1 controller defaults.
func Defaults() Config {
	return Config{
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
	}
}

// Load decodes and validates runtime settings from Viper.
func Load(vp *viper.Viper) (Config, error) {
	var cfg Config
	if err := vp.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
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

	return nil
}
