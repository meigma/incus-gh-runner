package liveacceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	admissionTestProject  = "runtime-acceptance"
	admissionTestInstance = "limit-probe"
	admissionTestRunID    = "run-123"
)

// admissionCommandCall describes one expected Incus command and its scripted result.
type admissionCommandCall struct {
	project   string
	arguments []string
	result    commandResult
	err       error
}

// admissionCommandStub is a strict ordered command boundary for admission cleanup tests.
type admissionCommandStub struct {
	t     *testing.T
	calls []admissionCommandCall
}

// run rejects unexpected host-command calls from admission cleanup behavior.
func (s *admissionCommandStub) run(
	_ context.Context,
	name string,
	arguments ...string,
) (commandResult, error) {
	s.t.Helper()
	require.FailNow(s.t, "unexpected host command", "%s %v", name, arguments)
	return commandResult{}, errors.New("unexpected host command")
}

// incus returns the next strictly matched Incus command result.
func (s *admissionCommandStub) incus(
	_ context.Context,
	project string,
	arguments ...string,
) (commandResult, error) {
	s.t.Helper()
	require.NotEmpty(s.t, s.calls, "unexpected Incus command: %s %v", project, arguments)
	call := s.calls[0]
	s.calls = s.calls[1:]
	require.Equal(s.t, call.project, project)
	require.Equal(s.t, call.arguments, arguments)
	return call.result, call.err
}

// requireExhausted proves that cleanup issued every expected command and no more.
func (s *admissionCommandStub) requireExhausted() {
	s.t.Helper()
	require.Empty(s.t, s.calls)
}

// TestMatchesProjectInstanceLimitRecognizesOnlyExpectedIncusErrors proves unrelated failures cannot close the gate.
func TestMatchesProjectInstanceLimitRecognizesOnlyExpectedIncusErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reason  string
		project string
		want    bool
	}{
		{
			name:    "total project limit",
			reason:  `Reached maximum number of instances in project "runtime-acceptance"`,
			project: "runtime-acceptance",
			want:    true,
		},
		{
			name: "wrapped virtual machine limit",
			reason: "Error: Failed instance creation: " +
				`Reached maximum number of instances of type "virtual-machine" in project "runtime-acceptance"`,
			project: "runtime-acceptance",
			want:    true,
		},
		{
			name:    "different project",
			reason:  `Reached maximum number of instances in project "other-project"`,
			project: "runtime-acceptance",
		},
		{
			name:    "project prefix is not exact",
			reason:  `Reached maximum number of instances in project "runtime-acceptance-other"`,
			project: "runtime-acceptance",
		},
		{
			name: "container type is not the probed ceiling",
			reason: `Reached maximum number of instances of type "container" in project ` +
				`"runtime-acceptance"`,
			project: "runtime-acceptance",
		},
		{
			name:    "trailing unrelated failure",
			reason:  `Reached maximum number of instances in project "runtime-acceptance": image not found`,
			project: "runtime-acceptance",
		},
		{name: "generic limit", reason: "Reached project limit for number of instances", project: "runtime-acceptance"},
		{name: "image missing", reason: "Image not found", project: "runtime-acceptance"},
		{name: "socket failure", reason: "connection refused", project: "runtime-acceptance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, matchesProjectInstanceLimit(tt.reason, tt.project))
		})
	}
}

// TestCleanupAdmissionProbeEventuallyWaitsForStableAbsence proves delayed removal is observed before success.
func TestCleanupAdmissionProbeEventuallyWaitsForStableAbsence(t *testing.T) {
	t.Parallel()

	commands := &admissionCommandStub{
		t: t,
		calls: []admissionCommandCall{
			admissionListCall(admissionTestInstance),
			admissionConfigCall(acceptanceIDKey, admissionTestRunID),
			admissionConfigCall(disposableVMKey, trueValue),
			admissionDeleteCall(),
			admissionListCall(admissionTestInstance),
			admissionListCall(),
			admissionListCall(),
		},
	}

	err := cleanupAdmissionProbeEventually(
		context.Background(),
		commands,
		admissionTestProject,
		admissionTestInstance,
		admissionTestRunID,
		0,
		2,
	)
	require.NoError(t, err)
	commands.requireExhausted()
}

// TestCleanupAdmissionProbeEventuallyFailsClosed proves uncertain post-delete inventory cannot pass cleanup.
func TestCleanupAdmissionProbeEventuallyFailsClosed(t *testing.T) {
	t.Parallel()

	commands := &admissionCommandStub{
		t: t,
		calls: []admissionCommandCall{
			admissionListCall(admissionTestInstance),
			admissionConfigCall(acceptanceIDKey, admissionTestRunID),
			admissionConfigCall(disposableVMKey, trueValue),
			admissionDeleteCall(),
			{
				project:   admissionTestProject,
				arguments: []string{"list", "--format=json"},
				result: commandResult{
					exitCode: 1,
					stderr:   []byte("daemon unavailable"),
				},
			},
		},
	}

	err := cleanupAdmissionProbeEventually(
		context.Background(),
		commands,
		admissionTestProject,
		admissionTestInstance,
		admissionTestRunID,
		0,
		2,
	)
	require.ErrorContains(t, err, "verify admission-probe cleanup")
	require.ErrorContains(t, err, "daemon unavailable")
	commands.requireExhausted()
}

// admissionListCall builds one exact project inventory response for the supplied instance names.
func admissionListCall(instances ...string) admissionCommandCall {
	entries := make([]string, 0, len(instances))
	for _, instance := range instances {
		entries = append(entries, `{"name":"`+instance+`"}`)
	}
	return admissionCommandCall{
		project:   admissionTestProject,
		arguments: []string{"list", "--format=json"},
		result: commandResult{
			stdout: []byte("[" + strings.Join(entries, ",") + "]"),
		},
	}
}

// admissionConfigCall builds one exact marker-read response.
func admissionConfigCall(key string, value string) admissionCommandCall {
	return admissionCommandCall{
		project:   admissionTestProject,
		arguments: []string{"config", "get", admissionTestInstance, key},
		result: commandResult{
			stdout: []byte(value + "\n"),
		},
	}
}

// admissionDeleteCall builds one successful exact-instance deletion response.
func admissionDeleteCall() admissionCommandCall {
	return admissionCommandCall{
		project:   admissionTestProject,
		arguments: []string{"delete", "--force", admissionTestInstance},
	}
}

// TestAdmissionProbeNameStaysWithinIncusLimit proves long safe run IDs remain bounded.
func TestAdmissionProbeNameStaysWithinIncusLimit(t *testing.T) {
	t.Parallel()

	name := admissionProbeName(strings.Repeat("a", 63))
	assert.LessOrEqual(t, len(name), 63)
	assert.True(t, localNamePattern.MatchString(name))
}
