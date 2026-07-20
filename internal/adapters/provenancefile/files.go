// Package provenancefile loads bounded provenance inputs from the filesystem.
package provenancefile

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"os"

	"github.com/meigma/incus-gh-runner/internal/provenance"
)

const maximumKeyBytes = 16 * 1024

// LoadPrivateKey reads exactly one bounded, regular PKCS#8 Ed25519 private-key file.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := readRegularFile(path, maximumKeyBytes, "signing key")
	if err != nil {
		return nil, err
	}
	key, err := provenance.ParsePrivateKeyPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse signing key: %w", err)
	}

	return key, nil
}

// LoadPublicKey reads exactly one bounded, regular SubjectPublicKeyInfo Ed25519 public-key file.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readRegularFile(path, maximumKeyBytes, "public key")
	if err != nil {
		return nil, err
	}
	key, err := provenance.ParsePublicKeyPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	return key, nil
}

// ReadEnvelope reads one bounded, regular DSSE envelope file.
func ReadEnvelope(path string) ([]byte, error) {
	return readRegularFile(path, provenance.MaximumEnvelopeBytes, "proof envelope")
}

// readRegularFile reads a stable-size regular file without including its content in errors.
func readRegularFile(path string, maximum int, label string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s %q: %w", label, path, err)
	}
	defer func() {
		_ = file.Close()
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s %q: %w", label, path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, path)
	}
	if info.Size() > int64(maximum) {
		return nil, fmt.Errorf("%s %q exceeds %d bytes", label, path, maximum)
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(maximum)+1))
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", label, path, err)
	}
	if len(data) > maximum {
		return nil, fmt.Errorf("%s %q exceeds %d bytes", label, path, maximum)
	}

	return data, nil
}
