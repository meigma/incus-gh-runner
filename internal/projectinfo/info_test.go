package projectinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSummary(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Incus-backed GitHub Actions runner controller", Summary())
}
