// Package runtime composes production adapters around the controller application.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"

	githubadapter "github.com/meigma/incus-gh-runner/internal/adapters/github"
	incusadapter "github.com/meigma/incus-gh-runner/internal/adapters/incus"
	"github.com/meigma/incus-gh-runner/internal/app"
	"github.com/meigma/incus-gh-runner/internal/config"
)

// BuildInfo identifies the executable in GitHub scale-set client requests.
type BuildInfo struct {
	// Version is the release version or development marker.
	Version string
	// Commit is the source commit used to build the executable.
	Commit string
}

// components contains the prepared production application and resolved scale-set identity.
type components struct {
	application *app.Application
	scaleSetID  int
}

// jitPayloadSource binds Incus allocation identities to fresh GitHub JIT configurations.
type jitPayloadSource struct {
	scaleSet *githubadapter.ScaleSet
}

// Payload returns the versioned guest input for one allocated runner.
func (s *jitPayloadSource) Payload(ctx context.Context, runnerID string) (incusadapter.Payload, error) {
	if s.scaleSet == nil {
		return incusadapter.Payload{}, errors.New("GitHub runner scale set is not resolved")
	}
	jitConfig, jitErr := s.scaleSet.JITConfig(ctx, runnerID)
	if jitErr != nil {
		return incusadapter.Payload{}, jitErr
	}

	return incusadapter.Payload{Version: 1, JITConfig: jitConfig}, nil
}

// Run validates configuration, preflights adapters, and runs the controller application.
func Run(ctx context.Context, cfg config.Config, build BuildInfo, logger *slog.Logger) error {
	if validationErr := cfg.ValidateRuntime(); validationErr != nil {
		return fmt.Errorf("validate runtime configuration: %w", validationErr)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	prepared, prepareErr := prepare(ctx, cfg, build, logger)
	if prepareErr != nil {
		return prepareErr
	}
	logger.InfoContext(
		ctx,
		"incus-gh-runner started",
		"scale_set", cfg.GitHub.ScaleSet,
		"scale_set_id", prepared.scaleSetID,
		"incus_project", cfg.Incus.Project,
	)
	return prepared.application.Run(ctx)
}

// prepare preflights Incus before resolving GitHub state and opening a message session.
func prepare(ctx context.Context, cfg config.Config, build BuildInfo, logger *slog.Logger) (*components, error) {
	githubClient, clientErr := newGitHubClient(cfg.GitHub, build)
	if clientErr != nil {
		return nil, clientErr
	}
	payloads := &jitPayloadSource{}
	backend, backendErr := newIncusBackend(ctx, cfg, payloads, logger)
	if backendErr != nil {
		return nil, backendErr
	}

	resolved, resolveErr := githubadapter.ResolveScaleSet(ctx, githubClient, githubadapter.ScaleSetOptions{
		Name:        cfg.GitHub.ScaleSet,
		RunnerGroup: cfg.GitHub.RunnerGroup,
		SystemInfo:  systemInfo(build),
		Logger:      logger.WithGroup("github"),
	})
	if resolveErr != nil {
		return nil, resolveErr
	}
	payloads.scaleSet = resolved

	sessionOwner := resolveSessionOwner(ctx, logger)
	demandSource, sourceErr := githubadapter.NewResilientDemandSource(
		ctx,
		githubClient,
		sessionOwner,
		githubadapter.DemandSourceOptions{
			ScaleSetID:          resolved.ID(),
			MinRunners:          cfg.Capacity.MinRunners,
			MaxRunners:          cfg.Capacity.MaxRunners,
			Logger:              logger.WithGroup("github_listener"),
			ReconnectInitial:    cfg.Retry.Initial,
			ReconnectMaximum:    cfg.Retry.Maximum,
			SessionCloseTimeout: cfg.Timeouts.Shutdown,
		},
	)
	if sourceErr != nil {
		return nil, sourceErr
	}
	application, applicationErr := app.New(app.Options{
		Config:        cfg,
		DemandSource:  demandSource,
		RunnerBackend: backend,
		Logger:        logger.WithGroup("controller"),
	})
	if applicationErr != nil {
		closePreparedDemandSource(demandSource, cfg.Timeouts.Shutdown, logger)
		return nil, fmt.Errorf("construct application: %w", applicationErr)
	}

	return &components{application: application, scaleSetID: resolved.ID()}, nil
}

// newIncusBackend constructs and preflights the ownership-scoped VM lifecycle adapter.
func newIncusBackend(
	ctx context.Context,
	cfg config.Config,
	payloads incusadapter.PayloadSource,
	logger *slog.Logger,
) (*incusadapter.Backend, error) {
	incusServer, connectErr := incusadapter.ConnectUnix(ctx, cfg.Incus.Socket, cfg.Incus.Project)
	if connectErr != nil {
		return nil, fmt.Errorf("connect to Incus project %q: %w", cfg.Incus.Project, connectErr)
	}
	diagnostics, diagnosticsErr := newDiagnosticsSink(cfg.Incus.DiagnosticsDir)
	if diagnosticsErr != nil {
		return nil, diagnosticsErr
	}
	backend, backendErr := incusadapter.NewBackend(incusServer, incusadapter.Options{
		Image:            cfg.Incus.Image,
		Profiles:         cfg.Incus.Profiles,
		Owner:            cfg.Incus.Owner,
		BootstrapTimeout: cfg.Incus.BootstrapTimeout,
		Payloads:         payloads,
		Diagnostics:      diagnostics,
		Logger:           logger.WithGroup("incus"),
	})
	if backendErr != nil {
		return nil, fmt.Errorf("construct Incus backend: %w", backendErr)
	}
	preflightContext, cancelPreflight := context.WithTimeout(ctx, cfg.Timeouts.IncusOperation)
	preflightErr := backend.Preflight(preflightContext)
	cancelPreflight()
	if preflightErr != nil {
		return nil, fmt.Errorf("preflight Incus backend: %w", preflightErr)
	}

	return backend, nil
}

// newDiagnosticsSink constructs optional protected terminal-evidence storage.
func newDiagnosticsSink(directory string) (incusadapter.DiagnosticsSink, error) {
	if directory == "" {
		return incusadapter.DiagnosticsSinkFunc(
			func(context.Context, incusadapter.Diagnostics) error { return nil },
		), nil
	}
	diagnostics, diagnosticsErr := incusadapter.NewDirectoryDiagnosticsSink(directory)
	if diagnosticsErr != nil {
		return nil, fmt.Errorf("configure runner diagnostics: %w", diagnosticsErr)
	}

	return diagnostics, nil
}

// resolveSessionOwner returns a host identity or a generated fallback for message ownership.
func resolveSessionOwner(ctx context.Context, logger *slog.Logger) string {
	sessionOwner, hostnameErr := os.Hostname()
	if hostnameErr == nil && sessionOwner != "" {
		return sessionOwner
	}

	sessionOwner = uuid.NewString()
	logger.WarnContext(ctx, "hostname unavailable; using generated message-session owner", "error", hostnameErr)
	return sessionOwner
}

// systemInfo builds the upstream client identity before its scale-set ID is known.
func systemInfo(build BuildInfo) scaleset.SystemInfo {
	return scaleset.SystemInfo{
		System:    "incus-gh-runner",
		Version:   build.Version,
		CommitSHA: build.Commit,
		Subsystem: "controller",
	}
}

// newGitHubClient constructs the configured upstream scale-set client without logging credentials.
func newGitHubClient(settings config.GitHub, build BuildInfo) (*scaleset.Client, error) {
	systemInfo := systemInfo(build)
	if settings.Token != "" {
		client, err := githubadapter.NewClientWithPersonalAccessToken(
			scaleset.NewClientWithPersonalAccessTokenConfig{
				GitHubConfigURL:     settings.ConfigURL,
				PersonalAccessToken: settings.Token,
				SystemInfo:          systemInfo,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("construct GitHub PAT client: %w", err)
		}
		return client, nil
	}

	privateKey, err := os.ReadFile(settings.App.PrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read GitHub App private key: %w", err)
	}
	client, err := githubadapter.NewClientWithGitHubApp(scaleset.ClientWithGitHubAppConfig{
		GitHubConfigURL: settings.ConfigURL,
		GitHubAppAuth: scaleset.GitHubAppAuth{
			ClientID:       settings.App.ClientID,
			InstallationID: settings.App.InstallationID,
			PrivateKey:     string(privateKey),
		},
		SystemInfo: systemInfo,
	})
	if err != nil {
		return nil, fmt.Errorf("construct GitHub App client: %w", err)
	}

	return client, nil
}

// closePreparedDemandSource releases an unused startup session within a bounded fresh context.
func closePreparedDemandSource(
	source *githubadapter.ResilientDemandSource,
	timeout time.Duration,
	logger *slog.Logger,
) {
	closeContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := source.Close(closeContext); err != nil {
		logger.Warn("failed to close GitHub message session", "error", err)
	}
}
