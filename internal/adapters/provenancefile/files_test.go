package provenancefile_test

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/adapters/provenancefile"
	"github.com/meigma/incus-gh-runner/internal/provenance"
)

// TestLoadKeysAndEnvelope proves the bounded file adapter's successful paths.
func TestLoadKeysAndEnvelope(t *testing.T) {
	t.Parallel()

	privateKey := testPrivateKey()
	directory := t.TempDir()
	privatePath := filepath.Join(directory, "private.pem")
	publicPath := filepath.Join(directory, "public.pem")
	proofPath := filepath.Join(directory, "proof.json")
	require.NoError(t, os.WriteFile(privatePath, encodePrivateKey(t, privateKey), 0o600))
	require.NoError(t, os.WriteFile(publicPath, encodePublicKey(t, privateKey.Public().(ed25519.PublicKey)), 0o644))
	require.NoError(t, os.WriteFile(proofPath, []byte(`{"payloadType":"test"}`), 0o600))

	loadedPrivate, err := provenancefile.LoadPrivateKey(privatePath)
	require.NoError(t, err)
	assert.Equal(t, privateKey, loadedPrivate)
	loadedPublic, err := provenancefile.LoadPublicKey(publicPath)
	require.NoError(t, err)
	assert.Equal(t, privateKey.Public(), loadedPublic)
	proof, err := provenancefile.ReadEnvelope(proofPath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"payloadType":"test"}`, string(proof))
}

// TestLoadPrivateKeyRejectsUnsafeInputs proves file shape and size checks fail safely.
func TestLoadPrivateKeyRejectsUnsafeInputs(t *testing.T) {
	t.Parallel()

	privatePEM := encodePrivateKey(t, testPrivateKey())
	const secret = "private-material-must-not-appear"
	tests := []struct {
		name    string
		prepare func(*testing.T) string
		wantErr string
	}{
		{
			name: "missing file",
			prepare: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "missing")
			},
			wantErr: "open signing key",
		},
		{
			name: "directory",
			prepare: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "must be a regular file",
		},
		{
			name: "oversized file",
			prepare: func(t *testing.T) string {
				t.Helper()
				path := filepath.Join(t.TempDir(), "oversized.pem")
				require.NoError(t, os.WriteFile(path, []byte(strings.Repeat(secret, 2048)), 0o600))
				return path
			},
			wantErr: "exceeds 16384 bytes",
		},
		{
			name: "multiple keys",
			prepare: func(t *testing.T) string {
				t.Helper()
				path := filepath.Join(t.TempDir(), "multiple.pem")
				data := append(append([]byte(nil), privatePEM...), privatePEM...)
				require.NoError(t, os.WriteFile(path, data, 0o600))
				return path
			},
			wantErr: "exactly one",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.prepare(t)

			_, err := provenancefile.LoadPrivateKey(path)

			require.ErrorContains(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), secret)
		})
	}
}

// TestReadEnvelopeRejectsOversizedInput proves the encoded proof limit is enforced before parsing.
func TestReadEnvelopeRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "proof.json")
	require.NoError(t, os.WriteFile(path, make([]byte, provenance.MaximumEnvelopeBytes+1), 0o600))

	_, err := provenancefile.ReadEnvelope(path)

	require.ErrorContains(t, err, "proof envelope")
	assert.Contains(t, err.Error(), "exceeds")
}

// testPrivateKey returns a deterministic Ed25519 fixture.
func testPrivateKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}

	return ed25519.NewKeyFromSeed(seed)
}

// encodePrivateKey creates a PKCS#8 PEM fixture.
func encodePrivateKey(t *testing.T, privateKey ed25519.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// encodePublicKey creates a SubjectPublicKeyInfo PEM fixture.
func encodePublicKey(t *testing.T, publicKey ed25519.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}
