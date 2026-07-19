package liveacceptance

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// evidenceWriter creates private, deterministic files inside one new evidence directory.
type evidenceWriter struct {
	directory string
}

// newEvidenceWriter creates a private evidence directory without accepting an existing path.
func newEvidenceWriter(directory string) (*evidenceWriter, error) {
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create evidence directory: %w", err)
	}
	return &evidenceWriter{directory: directory}, nil
}

// path resolves one fixed evidence filename beneath the writer directory.
func (w *evidenceWriter) path(name string) (string, error) {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", fmt.Errorf("invalid evidence filename %q", name)
	}
	return filepath.Join(w.directory, name), nil
}

// write writes one private evidence file without following a final symlink.
func (w *evidenceWriter) write(name string, data []byte) error {
	path, err := w.path(name)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create evidence file %q: %w", name, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write evidence file %q: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close evidence file %q: %w", name, err)
	}
	return nil
}

// writeJSON serializes one indented JSON evidence document.
func (w *evidenceWriter) writeJSON(name string, value any) error {
	data, err := evidenceJSON(value)
	if err != nil {
		return fmt.Errorf("encode evidence file %q: %w", name, err)
	}
	return w.write(name, data)
}

// evidenceJSON serializes one indented JSON document with a trailing newline.
func evidenceJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// openJSONL creates one private line-oriented JSON evidence stream.
func (w *evidenceWriter) openJSONL(name string) (*jsonlWriter, error) {
	path, err := w.path(name)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create evidence stream %q: %w", name, err)
	}
	return &jsonlWriter{file: file, writer: bufio.NewWriter(file)}, nil
}

// scanFor rejects a synthetic canary that leaked into retained evidence.
func (w *evidenceWriter) scanFor(canary []byte) error {
	if len(canary) == 0 {
		return errors.New("evidence canary must not be empty")
	}
	root, err := os.OpenRoot(w.directory)
	if err != nil {
		return fmt.Errorf("open evidence root: %w", err)
	}
	defer root.Close()
	return fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, canary) {
			return fmt.Errorf("synthetic agent canary leaked into evidence file %q", filepath.Base(path))
		}
		return nil
	})
}

// writeChecksums writes a stable manifest including one pending file not yet exposed to readers.
func (w *evidenceWriter) writeChecksums(pendingName string, pendingData []byte) error {
	if _, err := w.path(pendingName); err != nil {
		return err
	}
	entries, err := os.ReadDir(w.directory)
	if err != nil {
		return fmt.Errorf("list evidence directory: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "checksums.sha256" {
			continue
		}
		names = append(names, entry.Name())
	}
	names = append(names, pendingName)
	sort.Strings(names)

	var manifest strings.Builder
	for _, name := range names {
		data := pendingData
		if name != pendingName {
			data, err = os.ReadFile(filepath.Join(w.directory, name))
			if err != nil {
				return fmt.Errorf("checksum evidence file %q: %w", name, err)
			}
		}
		digest := sha256.Sum256(data)
		_, _ = fmt.Fprintf(&manifest, "%s  %s\n", hex.EncodeToString(digest[:]), name)
	}
	return w.write("checksums.sha256", []byte(manifest.String()))
}

// jsonlWriter writes and closes one JSON object per evidence line.
type jsonlWriter struct {
	file   *os.File
	writer *bufio.Writer
}

// write serializes one value followed by a newline.
func (w *jsonlWriter) write(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode JSONL evidence: %w", err)
	}
	if _, err := w.writer.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write JSONL evidence: %w", err)
	}
	return nil
}

// close flushes and closes the evidence stream.
func (w *jsonlWriter) close() error {
	if err := w.writer.Flush(); err != nil {
		_ = w.file.Close()
		return fmt.Errorf("flush JSONL evidence: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close JSONL evidence: %w", err)
	}
	return nil
}
