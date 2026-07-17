package config

import (
	"strings"

	"github.com/spf13/viper"

	"github.com/meigma/incus-gh-runner/internal/projectinfo"
)

// Config contains runtime settings used by the CLI.
type Config struct {
	// Message is printed when the root command runs without a subcommand.
	Message string
}

// Load reads runtime settings from Viper.
func Load(vp *viper.Viper) Config {
	message := strings.TrimSpace(vp.GetString("message"))
	if message == "" {
		message = projectinfo.Summary()
	}

	return Config{
		Message: message,
	}
}
