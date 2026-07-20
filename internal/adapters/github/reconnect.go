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
	mu        sync.Mutex
	available bool
	open      sessionOpener
	wait      retryWaiter
	options   DemandSourceOptions
}

// NewResilientDemandSource prepares startup and reconnecting message-session acquisition.
func NewResilientDemandSource(
	_ context.Context,
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

	return newResilientDemandSource(open, options, waitForRetry)
}

// newResilientDemandSource constructs a reconnecting source around testable session seams.
func newResilientDemandSource(
	open sessionOpener,
	options DemandSourceOptions,
	wait retryWaiter,
) (*ResilientDemandSource, error) {
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
	if options.MinRunners < 0 {
		return nil, errors.New("minimum runners must not be negative")
	}
	if options.MaxRunners < options.MinRunners {
		return nil, errors.New("maximum runners must be at least minimum runners")
	}

	return &ResilientDemandSource{
		available: true,
		open:      open,
		wait:      wait,
		options:   options,
	}, nil
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
	if !s.claim() {
		return errors.New("resilient demand source is already running or closed")
	}

	backoff := newReconnectBackoff(s.options.ReconnectInitial, s.options.ReconnectMaximum)
	session, disconnectErr := s.openInitial(ctx)
	var generation uint64
	for {
		if session != nil {
			generation++
			disconnectErr = s.runSession(ctx, session, publish, backoff.Reset, generation)
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

// openInitial attempts the first message session without delaying application startup.
func (s *ResilientDemandSource) openInitial(ctx context.Context) (messageSession, error) {
	session, err := s.open(ctx)
	if err != nil {
		return nil, fmt.Errorf("create initial GitHub message session: %w", err)
	}
	return session, nil
}

// runSession polls one GitHub message session and reports why it disconnected.
func (s *ResilientDemandSource) runSession(
	ctx context.Context,
	session messageSession,
	publish func(controller.Demand),
	onContact func(),
	generation uint64,
) error {
	source, err := NewDemandSource(session, s.options)
	if err != nil {
		return err
	}
	source.onContact = onContact
	if err := source.Run(ctx, func(demand controller.Demand) {
		demand.Generation = generation
		publish(demand)
	}); err != nil {
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

// Close prevents a prepared demand source from starting when application construction fails.
func (s *ResilientDemandSource) Close(context.Context) error {
	s.claim()
	return nil
}

// claim transfers ownership of the demand source exactly once.
func (s *ResilientDemandSource) claim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.available {
		return false
	}
	s.available = false
	return true
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
