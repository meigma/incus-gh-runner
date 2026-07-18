package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"

	"github.com/meigma/incus-gh-runner/internal/controller"
)

// messageSession is one closeable GitHub scale-set polling session.
type messageSession interface {
	listener.Client
	Close(ctx context.Context) error
}

// sessionOpener creates a fresh GitHub message session after a disconnect.
type sessionOpener func(ctx context.Context) (messageSession, error)

// retryWaiter blocks for one reconnect delay or until its context ends.
type retryWaiter func(ctx context.Context, delay time.Duration) error

// ResilientDemandSource recreates failed GitHub message sessions with capped backoff.
type ResilientDemandSource struct {
	mu      sync.Mutex
	initial messageSession
	open    sessionOpener
	wait    retryWaiter
	options DemandSourceOptions
}

// NewResilientDemandSource opens the startup message session and prepares transient recovery.
func NewResilientDemandSource(
	ctx context.Context,
	client *scaleset.Client,
	owner string,
	options DemandSourceOptions,
) (*ResilientDemandSource, error) {
	if client == nil {
		return nil, errors.New("scale-set client is required")
	}
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("message-session owner is required")
	}
	if err := validateReconnectOptions(options); err != nil {
		return nil, err
	}

	open := func(ctx context.Context) (messageSession, error) {
		session, err := client.MessageSessionClient(ctx, options.ScaleSetID, owner)
		if err != nil {
			return nil, err
		}
		return session, nil
	}
	initial, err := open(ctx)
	if err != nil {
		return nil, fmt.Errorf("create initial GitHub message session: %w", err)
	}

	source, err := newResilientDemandSource(initial, open, options, waitForRetry)
	if err != nil {
		closeContext, cancelClose := context.WithTimeout(context.Background(), options.SessionCloseTimeout)
		defer cancelClose()
		_ = initial.Close(closeContext)
		return nil, err
	}
	return source, nil
}

// newResilientDemandSource constructs a reconnecting source around testable session seams.
func newResilientDemandSource(
	initial messageSession,
	open sessionOpener,
	options DemandSourceOptions,
	wait retryWaiter,
) (*ResilientDemandSource, error) {
	if initial == nil {
		return nil, errors.New("initial message session is required")
	}
	if open == nil {
		return nil, errors.New("message-session opener is required")
	}
	if wait == nil {
		return nil, errors.New("reconnect waiter is required")
	}
	if err := validateReconnectOptions(options); err != nil {
		return nil, err
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	if _, err := NewDemandSource(initial, options); err != nil {
		return nil, err
	}

	return &ResilientDemandSource{initial: initial, open: open, wait: wait, options: options}, nil
}

// validateReconnectOptions checks bounded reconnect and cleanup timing.
func validateReconnectOptions(options DemandSourceOptions) error {
	if options.ReconnectInitial <= 0 {
		return errors.New("initial reconnect delay must be positive")
	}
	if options.ReconnectMaximum < options.ReconnectInitial {
		return errors.New("maximum reconnect delay must be at least the initial delay")
	}
	if options.SessionCloseTimeout <= 0 {
		return errors.New("message-session close timeout must be positive")
	}
	return nil
}

// Run publishes demand while recreating failed GitHub message sessions until cancellation.
func (s *ResilientDemandSource) Run(ctx context.Context, publish func(controller.Demand)) error {
	if publish == nil {
		return errors.New("demand publisher is required")
	}
	session := s.takeInitial()
	if session == nil {
		return errors.New("resilient demand source is already running or closed")
	}

	backoff := newReconnectBackoff(s.options.ReconnectInitial, s.options.ReconnectMaximum)
	var disconnectErr error
	for {
		if session != nil {
			disconnectErr = s.runSession(ctx, session, publish, backoff.Reset)
			s.closeSession(session)
			session = nil
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		reopened, err := s.reconnect(ctx, disconnectErr, backoff.Next())
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			disconnectErr = err
			continue
		}
		session = reopened
	}
}

// runSession polls one GitHub message session and reports why it disconnected.
func (s *ResilientDemandSource) runSession(
	ctx context.Context,
	session messageSession,
	publish func(controller.Demand),
	onContact func(),
) error {
	source, err := NewDemandSource(session, s.options)
	if err != nil {
		return err
	}
	source.onContact = onContact
	if err := source.Run(ctx, publish); err != nil {
		return err
	}
	return errors.New("scale-set listener stopped unexpectedly")
}

// reconnect waits for the current backoff and opens a replacement message session.
func (s *ResilientDemandSource) reconnect(
	ctx context.Context,
	disconnectErr error,
	delay time.Duration,
) (messageSession, error) {
	s.options.Logger.WarnContext(
		ctx,
		"GitHub message session disconnected; reconnecting",
		"error", disconnectErr,
		"retry_after", delay,
	)
	if err := s.wait(ctx, delay); err != nil {
		return nil, fmt.Errorf("wait to recreate GitHub message session: %w", err)
	}

	session, err := s.open(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GitHub message session: %w", err)
	}
	return session, nil
}

// Close releases the startup session when the prepared application cannot start.
func (s *ResilientDemandSource) Close(ctx context.Context) error {
	session := s.takeInitial()
	if session == nil {
		return nil
	}
	return session.Close(ctx)
}

// takeInitial transfers ownership of the startup session exactly once.
func (s *ResilientDemandSource) takeInitial() messageSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.initial
	s.initial = nil
	return session
}

// closeSession releases one replaced message session within a fresh bounded context.
func (s *ResilientDemandSource) closeSession(session messageSession) {
	closeContext, cancelClose := context.WithTimeout(context.Background(), s.options.SessionCloseTimeout)
	defer cancelClose()
	if err := session.Close(closeContext); err != nil {
		s.options.Logger.Warn("failed to close GitHub message session", "error", err)
	}
}

// waitForRetry sleeps for delay while remaining responsive to cancellation.
func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// reconnectBackoff tracks the next capped exponential reconnect delay.
type reconnectBackoff struct {
	initial time.Duration
	maximum time.Duration
	current time.Duration
}

// newReconnectBackoff creates a backoff at its initial delay.
func newReconnectBackoff(initial, maximum time.Duration) *reconnectBackoff {
	return &reconnectBackoff{initial: initial, maximum: maximum, current: initial}
}

// Next returns the current delay and advances the backoff toward its cap.
func (b *reconnectBackoff) Next() time.Duration {
	delay := b.current
	if b.current >= b.maximum-b.current {
		b.current = b.maximum
	} else {
		b.current *= 2
	}
	return delay
}

// Reset returns the next reconnect delay to its initial value after healthy contact.
func (b *reconnectBackoff) Reset() {
	b.current = b.initial
}
