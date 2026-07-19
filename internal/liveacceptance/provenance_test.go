package liveacceptance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseBuildProvenanceRequiresExplicitIdentity proves implicit or malformed VCS claims cannot enter evidence.
func TestParseBuildProvenanceRequiresExplicitIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		revision  string
		modified  string
		wantDirty bool
		wantError bool
	}{
		{name: "clean SHA-1", revision: strings.Repeat("a", 40), modified: "false"},
		{name: "clean SHA-256", revision: strings.Repeat("b", 64), modified: "false"},
		{name: "dirty source", revision: strings.Repeat("b", 40), modified: "true", wantError: true},
		{name: "missing revision", modified: "false", wantError: true},
		{name: "uppercase revision", revision: strings.Repeat("A", 40), modified: "false", wantError: true},
		{name: "missing modified state", revision: strings.Repeat("a", 40), wantError: true},
		{name: "ambiguous modified state", revision: strings.Repeat("a", 40), modified: "1", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			revision, dirty, err := parseBuildProvenance(tt.revision, tt.modified)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.revision, revision)
			assert.Equal(t, tt.wantDirty, dirty)
		})
	}
}
