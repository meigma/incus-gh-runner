package github

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObservedMessageSessionLogsOnlySecretSafeMetadata(t *testing.T) {
	t.Parallel()

	const secret = "must-not-appear-in-message-poll-log"
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	session := newFakeMessageSession(
		func(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
			return &scaleset.RunnerScaleSetMessage{
				MessageID: 29,
				JobStartedMessages: []*scaleset.JobStarted{{
					JobMessageBase: scaleset.JobMessageBase{
						JobDisplayName: secret,
						RequestLabels:  []string{secret},
					},
				}},
			}, nil
		},
	)
	observed := newObservedMessageSession(session, logger, 0)

	message, err := observed.GetMessage(context.Background(), 28, 4)

	require.NoError(t, err)
	require.NotNil(t, message)
	assert.Equal(t, 29, message.MessageID)
	assert.Contains(t, logs.String(), "outcome=message")
	assert.Contains(t, logs.String(), "message_id=29")
	assert.Contains(t, logs.String(), "job_started_count=1")
	assert.NotContains(t, logs.String(), secret)
}

func TestObservedMessageSessionTranslatesOnlyItsDiagnosticTimeout(t *testing.T) {
	t.Parallel()

	session := newFakeMessageSession(
		func(ctx context.Context, _ int, _ int) (*scaleset.RunnerScaleSetMessage, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)
	observed := newObservedMessageSession(session, nil, 10*time.Millisecond)

	started := time.Now()
	message, err := observed.GetMessage(context.Background(), 0, 4)

	require.NoError(t, err)
	assert.Nil(t, message)
	assert.Less(t, time.Since(started), time.Second)
}

func TestObservedMessageSessionPreservesParentCancellation(t *testing.T) {
	t.Parallel()

	session := newFakeMessageSession(
		func(ctx context.Context, _ int, _ int) (*scaleset.RunnerScaleSetMessage, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)
	observed := newObservedMessageSession(session, nil, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	message, err := observed.GetMessage(ctx, 0, 4)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, message)
}

func TestObservedMessageSessionPreservesUpstreamErrors(t *testing.T) {
	t.Parallel()

	upstreamErr := errors.New("unavailable")
	session := newFakeMessageSession(
		func(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
			return nil, upstreamErr
		},
	)
	observed := newObservedMessageSession(session, nil, 0)

	message, err := observed.GetMessage(context.Background(), 0, 4)

	require.ErrorIs(t, err, upstreamErr)
	assert.Nil(t, message)
}

func TestObservedMessageSessionDoesNotHideUpstreamErrorAtDiagnosticDeadline(t *testing.T) {
	t.Parallel()

	upstreamErr := errors.New("unavailable")
	session := newFakeMessageSession(
		func(ctx context.Context, _ int, _ int) (*scaleset.RunnerScaleSetMessage, error) {
			<-ctx.Done()
			return nil, upstreamErr
		},
	)
	observed := newObservedMessageSession(session, nil, 10*time.Millisecond)

	message, err := observed.GetMessage(context.Background(), 0, 4)

	require.ErrorIs(t, err, upstreamErr)
	assert.Nil(t, message)
}
