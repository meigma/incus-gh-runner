package liveacceptance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIPv6RunHextetIsStableAndBounded proves run-scoped probe addresses remain valid evidence identifiers.
func TestIPv6RunHextetIsStableAndBounded(t *testing.T) {
	t.Parallel()

	first := ipv6RunHextet("hostile-20260719120000-123")
	second := ipv6RunHextet("hostile-20260719120000-123")
	assert.Equal(t, first, second)
	assert.Regexp(t, `^[a-f0-9]{4}$`, first)
}

// TestIPv6ListenerUsesInterpreterOnNoExecRun proves the response helper is not executed directly from /run.
func TestIPv6ListenerUsesInterpreterOnNoExecRun(t *testing.T) {
	t.Parallel()

	script := ipv6ListenerScript()
	assert.Contains(t, script, "--accept --inetd -- /bin/sh /run/incus-gh-runner-ipv6-response")
	assert.NotContains(t, script, "--accept -- /run/incus-gh-runner-ipv6-response")
}

// TestClassifyGuestCurlDenialRejectsFalsePositiveFailures proves only explicit network denial closes the gate.
func TestClassifyGuestCurlDenialRejectsFalsePositiveFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    commandResult
		runErr    error
		blocked   bool
		errorText string
	}{
		{name: "reachable", result: commandResult{stdout: []byte("0\nipv6-isolation-probe")}},
		{name: "could not connect", result: commandResult{stdout: []byte("7\nconnection refused")}, blocked: true},
		{name: "timed out", result: commandResult{stdout: []byte("28\noperation timed out")}, blocked: true},
		{
			name:      "malformed URL",
			result:    commandResult{stdout: []byte("3\nbad URL")},
			errorText: "non-network curl exit code 3",
		},
		{
			name:      "source bind failed",
			result:    commandResult{stdout: []byte("45\nbind failed")},
			errorText: "non-network curl exit code 45",
		},
		{name: "missing status line", result: commandResult{stdout: []byte("missing newline")}, errorText: "omitted"},
		{name: "invalid status", result: commandResult{stdout: []byte("invalid\ndetail")}, errorText: "invalid"},
		{name: "host timeout", runErr: context.DeadlineExceeded, errorText: "deadline exceeded"},
		{
			name:      "Incus exec failed",
			result:    commandResult{exitCode: 1, stderr: []byte("agent unavailable")},
			errorText: "agent unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := classifyGuestCurlDenial(tt.result, tt.runErr)
			if tt.errorText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorText)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.blocked, result.blocked)
		})
	}
}
