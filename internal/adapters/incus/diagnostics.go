package incus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	diagnosticsDirectoryMode = 0o700
	diagnosticsFileMode      = 0o600
)

// DirectoryDiagnosticsSink stores terminal serial-console evidence in a protected directory.
type DirectoryDiagnosticsSink struct {
	directory string
}

// NewDirectoryDiagnosticsSink constructs a protected filesystem diagnostics sink.
func NewDirectoryDiagnosticsSink(directory string) (*DirectoryDiagnosticsSink, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("diagnostics directory is required")
	}

	return &DirectoryDiagnosticsSink{directory: filepath.Clean(directory)}, nil
}

// Store writes one runner's terminal console without broadening its filename authority.
func (s *DirectoryDiagnosticsSink) Store(_ context.Context, diagnostics Diagnostics) error {
	if diagnostics.RunnerID == "" || filepath.Base(diagnostics.RunnerID) != diagnostics.RunnerID {
		return errors.New("diagnostics runner ID is not a safe filename")
	}
	if err := os.MkdirAll(s.directory, diagnosticsDirectoryMode); err != nil {
		return fmt.Errorf("create diagnostics directory: %w", err)
	}
	path := filepath.Join(s.directory, diagnostics.RunnerID+".console.log")
	if err := os.WriteFile(path, diagnostics.Console, diagnosticsFileMode); err != nil {
		return fmt.Errorf("write runner diagnostics: %w", err)
	}

	return nil
}
