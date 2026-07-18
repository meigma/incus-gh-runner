package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/meigma/incus-gh-runner/internal/cli"
	"github.com/meigma/incus-gh-runner/internal/config"
	runnerruntime "github.com/meigma/incus-gh-runner/internal/runtime"
)

//nolint:gochecknoglobals // GoReleaser injects these values with ldflags during releases.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// main executes the command and exits with its status code.
func main() {
	os.Exit(run())
}

// run builds and executes the signal-aware root command.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	root := cli.NewRootCommand(cli.Options{
		In: os.Stdin,
		Build: cli.BuildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
		},
		Out: os.Stdout,
		Err: os.Stderr,
		Run: func(ctx context.Context, cfg config.Config) error {
			return runnerruntime.Run(ctx, cfg, runnerruntime.BuildInfo{
				Version: version,
				Commit:  commit,
			}, logger)
		},
	})
	if err := root.ExecuteContext(ctx); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			return 1
		}

		return 1
	}

	return 0
}
