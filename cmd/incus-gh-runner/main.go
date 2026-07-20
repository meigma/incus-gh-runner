package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	incuspolicy "github.com/meigma/incus-gh-runner/deploy/incus"
	incusadapter "github.com/meigma/incus-gh-runner/internal/adapters/incus"
	"github.com/meigma/incus-gh-runner/internal/adapters/provenancefile"
	"github.com/meigma/incus-gh-runner/internal/cli"
	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/incusvalidate"
	"github.com/meigma/incus-gh-runner/internal/provenance"
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
		Validate:    validateIncusBaseline,
		VerifyProof: verifyJobProof,
	})
	if err := root.ExecuteContext(ctx); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			return 1
		}

		return 1
	}

	return 0
}

// verifyJobProof authenticates one proof without loading controller configuration or credentials.
func verifyJobProof(
	ctx context.Context,
	proofPath string,
	publicKeyPath string,
	expectedHostID string,
) ([]byte, error) {
	publicKey, err := provenancefile.LoadPublicKey(publicKeyPath)
	if err != nil {
		return nil, err
	}
	envelope, err := provenancefile.ReadEnvelope(proofPath)
	if err != nil {
		return nil, err
	}
	payload, err := provenance.Verify(ctx, envelope, publicKey, expectedHostID)
	if err != nil {
		return nil, fmt.Errorf("verify job proof: %w", err)
	}

	return payload, nil
}

// validateIncusBaseline checks one rendered baseline without initializing controller runtime state.
func validateIncusBaseline(
	ctx context.Context,
	baselinePath string,
	socketPath string,
) (cli.ValidationResult, error) {
	data, err := readIncusBaseline(baselinePath)
	if err != nil {
		return cli.ValidationResult{}, err
	}
	baseline, err := incusvalidate.ParseBaseline(baselinePath, data, incuspolicy.ValidateBaseline)
	if err != nil {
		return cli.ValidationResult{}, err
	}

	reader, err := incusadapter.ConnectValidationReader(ctx, socketPath)
	if err != nil {
		return cli.ValidationResult{}, err
	}
	defer reader.Close()

	result, err := incusvalidate.Validate(ctx, baseline, reader)
	if err != nil {
		return cli.ValidationResult{}, err
	}

	return cli.ValidationResult{Notices: result.Notices}, nil
}

// readIncusBaseline bounds manifest reads before policy parsing and socket access.
func readIncusBaseline(baselinePath string) ([]byte, error) {
	info, err := os.Stat(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("stat baseline %q: %w", baselinePath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("baseline %q must be a regular file", baselinePath)
	}

	file, err := os.Open(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("open baseline %q: %w", baselinePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(file, incuspolicy.MaximumBaselineBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read baseline %q: %w", baselinePath, err)
	}

	return data, nil
}
