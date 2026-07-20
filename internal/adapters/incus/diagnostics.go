package incus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	diagnosticsDirectoryMode = 0o700
	diagnosticsFileMode      = 0o600
	maximumDiagnosticsFiles  = 256
)

// DirectoryDiagnosticsSink stores terminal serial-console evidence in a protected directory.
type DirectoryDiagnosticsSink struct {
	directory string
	mu        sync.Mutex
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
	if len(diagnostics.Console) > maximumConsoleLogBytes {
		return fmt.Errorf("runner diagnostics exceed %d-byte limit", maximumConsoleLogBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.directory, diagnosticsDirectoryMode); err != nil {
		return fmt.Errorf("create diagnostics directory: %w", err)
	}
	directoryInfo, err := os.Lstat(s.directory)
	if err != nil {
		return fmt.Errorf("inspect diagnostics directory: %w", err)
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("diagnostics path is not a directory")
	}
	if directoryInfo.Mode().Perm() != diagnosticsDirectoryMode {
		return fmt.Errorf("diagnostics directory permissions must be 0700, got %04o", directoryInfo.Mode().Perm())
	}
	path := filepath.Join(s.directory, diagnostics.RunnerID+".console.log")
	if _, err = os.Lstat(path); err == nil {
		return fmt.Errorf("write runner diagnostics: destination already exists: %w", os.ErrExist)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect runner diagnostics destination: %w", err)
	}
	if pruneErr := pruneDiagnostics(s.directory, maximumDiagnosticsFiles-1); pruneErr != nil {
		return fmt.Errorf("prune runner diagnostics: %w", pruneErr)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, diagnosticsFileMode)
	if err != nil {
		return fmt.Errorf("write runner diagnostics: %w", err)
	}
	if _, err = file.Write(diagnostics.Console); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write runner diagnostics: %w", err)
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close runner diagnostics: %w", err)
	}

	return nil
}

type diagnosticFile struct {
	path    string
	modTime time.Time
}

// pruneDiagnostics bounds retained controller-created files by removing the oldest first.
func pruneDiagnostics(directory string, retain int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}

	files := make([]diagnosticFile, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".console.log") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if info.Size() > maximumConsoleLogBytes {
			if removeErr := os.Remove(path); removeErr != nil {
				return removeErr
			}
			continue
		}
		files = append(files, diagnosticFile{path: path, modTime: info.ModTime()})
	}

	sort.Slice(files, func(i int, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, file := range files[:max(0, len(files)-retain)] {
		if err := os.Remove(file.path); err != nil {
			return err
		}
	}

	return nil
}
