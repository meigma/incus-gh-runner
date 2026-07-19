package liveacceptance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBoundedCommandBufferConsumesWithoutRetainingOverflow proves hostile output remains memory bounded.
func TestBoundedCommandBufferConsumesWithoutRetainingOverflow(t *testing.T) {
	t.Parallel()

	buffer := boundedCommandBuffer{limit: 4}
	written, err := buffer.Write([]byte("abcdef"))
	require.NoError(t, err)
	assert.Equal(t, 6, written)
	assert.Equal(t, []byte("abcd"), buffer.bytes())
	assert.True(t, buffer.overflow)
}
