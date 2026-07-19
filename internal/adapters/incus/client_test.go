package incus

import (
	"errors"
	"net/http"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
