package github

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/actions/scaleset"
)

var errDiagnosticMessagePollTimeout = errors.New("diagnostic message poll timeout")

// observedMessageSession records secret-safe message timing and can bound polls for diagnostics.
type observedMessageSession struct {
	messageSession

	logger      *slog.Logger
	pollTimeout time.Duration
}

// newObservedMessageSession decorates one session without changing its default polling behavior.
func newObservedMessageSession(
	session messageSession,
	logger *slog.Logger,
	pollTimeout time.Duration,
) *observedMessageSession {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &observedMessageSession{
		messageSession: session,
		logger:         logger,
		pollTimeout:    pollTimeout,
	}
}

// GetMessage records the client-visible poll boundary without logging response bodies or transport details.
func (s *observedMessageSession) GetMessage(
	ctx context.Context,
	lastMessageID int,
	maxCapacity int,
) (*scaleset.RunnerScaleSetMessage, error) {
	startedAt := time.Now().UTC()
	s.logger.InfoContext(
		ctx,
		"GitHub message poll started",
		"started_at", startedAt,
		"last_message_id", lastMessageID,
		"max_capacity", maxCapacity,
		"diagnostic_timeout", s.pollTimeout,
	)

	pollContext := ctx
	cancel := func() {}
	if s.pollTimeout > 0 {
		pollContext, cancel = context.WithTimeoutCause(ctx, s.pollTimeout, errDiagnosticMessagePollTimeout)
	}
	defer cancel()

	message, err := s.messageSession.GetMessage(pollContext, lastMessageID, maxCapacity)
	completedAt := time.Now().UTC()
	duration := completedAt.Sub(startedAt)
	if err != nil {
		if ctx.Err() == nil &&
			errors.Is(context.Cause(pollContext), errDiagnosticMessagePollTimeout) &&
			errors.Is(err, context.DeadlineExceeded) {
			s.logger.InfoContext(
				ctx,
				"GitHub message poll completed",
				"completed_at", completedAt,
				"duration", duration,
				"outcome", "diagnostic_timeout",
			)
			return nil, nil //nolint:nilnil // The listener treats an expired empty long poll as healthy contact.
		}
		s.logger.InfoContext(
			ctx,
			"GitHub message poll completed",
			"completed_at", completedAt,
			"duration", duration,
			"outcome", "error",
			"error_kind", messagePollErrorKind(err),
		)
		return nil, err
	}
	if message == nil {
		s.logger.InfoContext(
			ctx,
			"GitHub message poll completed",
			"completed_at", completedAt,
			"duration", duration,
			"outcome", "empty",
		)
		return nil, nil //nolint:nilnil // This is the upstream client's documented empty response.
	}

	s.logger.InfoContext(
		ctx,
		"GitHub message poll completed",
		"completed_at", completedAt,
		"duration", duration,
		"outcome", "message",
		"message_id", message.MessageID,
		"job_available_count", len(message.JobAvailableMessages),
		"job_assigned_count", len(message.JobAssignedMessages),
		"job_started_count", len(message.JobStartedMessages),
		"job_completed_count", len(message.JobCompletedMessages),
	)
	return message, nil
}

// messagePollErrorKind keeps diagnostic logs useful without rendering upstream error text.
func messagePollErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "upstream"
	}
}
