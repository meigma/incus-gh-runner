package incus

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBounded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		limit     int64
		want      string
		truncated bool
	}{
		{name: "below limit", content: "abc", limit: 4, want: "abc"},
		{name: "at limit", content: "abcd", limit: 4, want: "abcd"},
		{name: "above limit", content: "abcde", limit: 4, want: "abcd", truncated: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, truncated, err := readBounded(bytes.NewBufferString(test.content), test.limit)

			require.NoError(t, err)
			assert.Equal(t, test.want, string(got))
			assert.Equal(t, test.truncated, truncated)
		})
	}
}

func TestReadGuestStatusRejectsOversizedDocument(t *testing.T) {
	t.Parallel()

	_, err := readGuestStatus(bytes.NewReader(make([]byte, maximumGuestStatusBytes+1)))

	assert.EqualError(t, err, "guest file exceeds 65536-byte limit")
}

func TestReadConsoleLogMarksTruncatedOutputWithinLimit(t *testing.T) {
	t.Parallel()

	content, err := readConsoleLog(bytes.NewReader(make([]byte, maximumConsoleLogBytes+1)))

	require.NoError(t, err)
	assert.Len(t, content, maximumConsoleLogBytes)
	assert.True(t, bytes.HasSuffix(content, []byte(consoleTruncationMarker)))
}

func TestClassifyInstanceFileError(t *testing.T) {
	t.Parallel()

	fileNotFound := api.StatusErrorf(http.StatusNotFound, "file not found")
	instanceNotFound := api.StatusErrorf(http.StatusNotFound, "instance not found")
	transportError := errors.New("connection reset")

	tests := []struct {
		name            string
		fileError       error
		confirmation    error
		want            error
		wantConfirmed   bool
		wantExactSource error
	}{
		{
			name:          "missing file on existing instance",
			fileError:     fileNotFound,
			want:          errInstanceFileNotFound,
			wantConfirmed: true,
		},
		{
			name:          "instance disappeared",
			fileError:     fileNotFound,
			confirmation:  instanceNotFound,
			want:          errNotFound,
			wantConfirmed: true,
		},
		{
			name:            "confirmation failed",
			fileError:       fileNotFound,
			confirmation:    transportError,
			want:            transportError,
			wantConfirmed:   true,
			wantExactSource: transportError,
		},
		{
			name:            "file transport failure",
			fileError:       transportError,
			want:            transportError,
			wantExactSource: transportError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			confirmed := false
			err := classifyInstanceFileError(test.fileError, func() error {
				confirmed = true
				return test.confirmation
			})

			require.Error(t, err)
			require.ErrorIs(t, err, test.want)
			assert.Equal(t, test.wantConfirmed, confirmed)
			if test.wantExactSource != nil {
				assert.ErrorIs(t, err, test.wantExactSource)
			}
		})
	}
}
