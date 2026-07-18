package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNextFailureDelay proves initial, exponential, and capped retry delays.
func TestNextFailureDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		previous time.Duration
		want     time.Duration
	}{
		{name: "starts at initial", want: time.Second},
		{name: "doubles previous", previous: time.Second, want: 2 * time.Second},
		{name: "caps growth", previous: 8 * time.Second, want: 10 * time.Second},
		{name: "stays capped", previous: 10 * time.Second, want: 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, nextFailureDelay(tt.previous, time.Second, 10*time.Second))
		})
	}
}
