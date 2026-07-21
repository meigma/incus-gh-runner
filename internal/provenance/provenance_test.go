package provenance

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/secure-systems-lab/go-securesystemslib/dsse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	goldenKeyID         = "sha256:a050837d85070582ccf7394b0988847cc312cb88259b894899f6f239cf1791a5"
	goldenLaunchDigest  = "608ff2ca90fabbb21cc3bb5caa28e12294bfbae06afe4ad3e5aff4760b4f06cc"
	goldenProfileDigest = "1bc4059154d34a9eba1e19b132ce282ac392c2e95353e9d27fb944460d11334d"
)

// TestSignAndVerifyFixedReceipt proves the complete deterministic proof contract.
func TestSignAndVerifyFixedReceipt(t *testing.T) {
	t.Parallel()

	privateKey := fixedPrivateKey(t, 0)
	signer, err := NewSigner(privateKey)
	require.NoError(t, err)
	payload := fixedPayload()

	envelopeBytes, err := signer.Sign(context.Background(), payload)

	require.NoError(t, err)
	var envelope dsse.Envelope
	require.NoError(t, json.Unmarshal(envelopeBytes, &envelope))
	assert.Equal(t, PayloadType, envelope.PayloadType)
	require.Len(t, envelope.Signatures, 1)
	assert.Equal(t, goldenKeyID, envelope.Signatures[0].KeyID)
	verified, err := Verify(
		context.Background(),
		envelopeBytes,
		privateKey.Public().(ed25519.PublicKey),
		"builder-host-01",
	)
	require.NoError(t, err)
	expectedPayload, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedPayload), string(verified))
	assert.Contains(t, string(verified), `"claim":"`+Claim+`"`)
}

// TestSignAndVerifyUnreportedRunnerRequestID fixes the live JobStarted payload contract.
func TestSignAndVerifyUnreportedRunnerRequestID(t *testing.T) {
	t.Parallel()

	privateKey := fixedPrivateKey(t, 0)
	signer, err := NewSigner(privateKey)
	require.NoError(t, err)
	payload := fixedPayload()
	payload.GitHub.RunnerRequestID = 0

	envelope, err := signer.Sign(context.Background(), payload)
	require.NoError(t, err)
	verified, err := Verify(
		context.Background(),
		envelope,
		privateKey.Public().(ed25519.PublicKey),
		"builder-host-01",
	)
	require.NoError(t, err)
	assert.Contains(t, string(verified), `"runner_request_id":0`)
}

// TestVerifyRejectsUntrustedOrMalformedProofs proves verification fails closed.
func TestVerifyRejectsUntrustedOrMalformedProofs(t *testing.T) {
	t.Parallel()

	privateKey := fixedPrivateKey(t, 0)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signer, err := NewSigner(privateKey)
	require.NoError(t, err)
	valid, err := signer.Sign(context.Background(), fixedPayload())
	require.NoError(t, err)

	tests := []struct {
		name           string
		prepare        func(*testing.T) ([]byte, ed25519.PublicKey, string)
		wantErrContain string
	}{
		{
			name: "changed payload",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				envelope := decodeEnvelope(t, valid)
				payload, decodeErr := base64.StdEncoding.DecodeString(envelope.Payload)
				require.NoError(t, decodeErr)
				payload[0] ^= 1
				envelope.Payload = base64.StdEncoding.EncodeToString(payload)
				return encodeEnvelope(t, envelope), publicKey, "builder-host-01"
			},
			wantErrContain: "signature verification failed",
		},
		{
			name: "changed signature",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				envelope := decodeEnvelope(t, valid)
				signature, decodeErr := base64.StdEncoding.DecodeString(envelope.Signatures[0].Sig)
				require.NoError(t, decodeErr)
				signature[0] ^= 1
				envelope.Signatures[0].Sig = base64.StdEncoding.EncodeToString(signature)
				return encodeEnvelope(t, envelope), publicKey, "builder-host-01"
			},
			wantErrContain: "signature verification failed",
		},
		{
			name: "wrong key",
			prepare: func(*testing.T) ([]byte, ed25519.PublicKey, string) {
				return valid, fixedPrivateKey(t, 32).Public().(ed25519.PublicKey), "builder-host-01"
			},
			wantErrContain: "key ID does not match",
		},
		{
			name: "wrong host ID",
			prepare: func(*testing.T) ([]byte, ed25519.PublicKey, string) {
				return valid, publicKey, "builder-host-02"
			},
			wantErrContain: "host ID does not match",
		},
		{
			name: "wrong payload type",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				envelope := decodeEnvelope(t, valid)
				envelope.PayloadType = "application/example"
				return encodeEnvelope(t, envelope), publicKey, "builder-host-01"
			},
			wantErrContain: "payload type is not supported",
		},
		{
			name: "extra signature",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				envelope := decodeEnvelope(t, valid)
				envelope.Signatures = append(envelope.Signatures, envelope.Signatures[0])
				return encodeEnvelope(t, envelope), publicKey, "builder-host-01"
			},
			wantErrContain: "exactly one signature",
		},
		{
			name: "oversized envelope",
			prepare: func(*testing.T) ([]byte, ed25519.PublicKey, string) {
				return make([]byte, MaximumEnvelopeBytes+1), publicKey, "builder-host-01"
			},
			wantErrContain: "proof envelope exceeds",
		},
		{
			name: "invalid positive ID",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				payload := fixedPayload()
				payload.GitHub.WorkflowRunID = 0
				body, marshalErr := json.Marshal(payload)
				require.NoError(t, marshalErr)
				return signRawEnvelope(t, privateKey, PayloadType, body), publicKey, "builder-host-01"
			},
			wantErrContain: "workflow_run_id must be positive",
		},
		{
			name: "negative runner request ID",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				payload := fixedPayload()
				payload.GitHub.RunnerRequestID = -1
				body, marshalErr := json.Marshal(payload)
				require.NoError(t, marshalErr)
				return signRawEnvelope(t, privateKey, PayloadType, body), publicKey, "builder-host-01"
			},
			wantErrContain: "runner_request_id must not be negative",
		},
		{
			name: "unknown payload field",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				body, marshalErr := json.Marshal(fixedPayload())
				require.NoError(t, marshalErr)
				body = append(body[:len(body)-1], []byte(`,"unexpected":true}`)...)
				return signRawEnvelope(t, privateKey, PayloadType, body), publicKey, "builder-host-01"
			},
			wantErrContain: "unknown field",
		},
		{
			name: "unknown envelope field",
			prepare: func(t *testing.T) ([]byte, ed25519.PublicKey, string) {
				t.Helper()
				body := append([]byte(nil), valid[:len(valid)-1]...)
				body = append(body, []byte(`,"unexpected":true}`)...)
				return body, publicKey, "builder-host-01"
			},
			wantErrContain: "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			envelope, key, hostID := tt.prepare(t)

			verified, verifyErr := Verify(context.Background(), envelope, key, hostID)

			require.Error(t, verifyErr)
			assert.Contains(t, verifyErr.Error(), tt.wantErrContain)
			assert.Empty(t, verified, "unverified payload bytes must never be returned")
		})
	}
}

// TestSignRejectsOversizedPayload proves signer-side decoded payload limits.
func TestSignRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	signer, err := NewSigner(fixedPrivateKey(t, 0))
	require.NoError(t, err)
	payload := fixedPayload()
	for range 20 {
		payload.Machine.Profiles = append(payload.Machine.Profiles, Profile{
			Name:   strings.Repeat("p", MaximumIdentityBytes),
			SHA256: strings.Repeat("a", 64),
		})
	}

	_, err = signer.Sign(context.Background(), payload)

	require.ErrorContains(t, err, "proof payload exceeds")
}

// TestKeyIDUsesSubjectPublicKeyInfoDER fixes the enrolled key identity format.
func TestKeyIDUsesSubjectPublicKeyInfoDER(t *testing.T) {
	t.Parallel()

	publicKey := fixedPrivateKey(t, 0).Public().(ed25519.PublicKey)
	keyID, err := KeyID(publicKey)

	require.NoError(t, err)
	assert.Equal(t, goldenKeyID, keyID)
	openSSHKeyID, err := dsse.SHA256KeyID(publicKey)
	require.NoError(t, err)
	assert.NotEqual(t, openSSHKeyID, keyID, "the OpenSSH helper format must not be used")
}

// TestKeyParsingRequiresOneEd25519Key proves exact private and public formats.
func TestKeyParsingRequiresOneEd25519Key(t *testing.T) {
	t.Parallel()

	privateKey := fixedPrivateKey(t, 0)
	privatePEM := encodePrivateKey(t, privateKey)
	publicPEM := encodePublicKey(t, privateKey.Public().(ed25519.PublicKey))
	nonEd25519Key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	nonEd25519PrivateDER, err := x509.MarshalPKCS8PrivateKey(nonEd25519Key)
	require.NoError(t, err)
	nonEd25519PrivatePEM := pem.EncodeToMemory(&pem.Block{Type: privateKeyPEMType, Bytes: nonEd25519PrivateDER})
	nonEd25519PublicDER, err := x509.MarshalPKIXPublicKey(&nonEd25519Key.PublicKey)
	require.NoError(t, err)
	nonEd25519PublicPEM := pem.EncodeToMemory(&pem.Block{Type: publicKeyPEMType, Bytes: nonEd25519PublicDER})
	parsedPrivate, err := ParsePrivateKeyPEM(privatePEM)
	require.NoError(t, err)
	assert.Equal(t, privateKey, parsedPrivate)
	parsedPublic, err := ParsePublicKeyPEM(publicPEM)
	require.NoError(t, err)
	assert.Equal(t, privateKey.Public(), parsedPublic)

	const secret = "key-material-must-not-appear"
	tests := []struct {
		name    string
		parse   func([]byte) error
		data    []byte
		wantErr string
	}{
		{
			name:  "malformed private key",
			parse: func(data []byte) error { _, parseErr := ParsePrivateKeyPEM(data); return parseErr },
			data:  []byte(secret), wantErr: "exactly one",
		},
		{
			name:  "multiple private keys",
			parse: func(data []byte) error { _, parseErr := ParsePrivateKeyPEM(data); return parseErr },
			data:  append(append([]byte(nil), privatePEM...), privatePEM...), wantErr: "exactly one",
		},
		{
			name:  "private key passed as public",
			parse: func(data []byte) error { _, parseErr := ParsePublicKeyPEM(data); return parseErr },
			data:  privatePEM, wantErr: "exactly one",
		},
		{
			name:  "public key passed as private",
			parse: func(data []byte) error { _, parseErr := ParsePrivateKeyPEM(data); return parseErr },
			data:  publicPEM, wantErr: "exactly one",
		},
		{
			name:  "non-Ed25519 private key",
			parse: func(data []byte) error { _, parseErr := ParsePrivateKeyPEM(data); return parseErr },
			data:  nonEd25519PrivatePEM, wantErr: "must be an Ed25519 private key",
		},
		{
			name:  "non-Ed25519 public key",
			parse: func(data []byte) error { _, parseErr := ParsePublicKeyPEM(data); return parseErr },
			data:  nonEd25519PublicPEM, wantErr: "must be an Ed25519 public key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parseErr := tt.parse(tt.data)
			require.ErrorContains(t, parseErr, tt.wantErr)
			assert.NotContains(t, parseErr.Error(), secret)
		})
	}
}

// TestLaunchAndProfileGoldenVectors fixes the exact JSON and digest inputs.
func TestLaunchAndProfileGoldenVectors(t *testing.T) {
	t.Parallel()

	input := LaunchInput{
		Version:          Version,
		InstanceType:     LaunchInstanceType,
		ImageFingerprint: strings.Repeat("1", 64),
		Profiles: []Profile{{
			Name:   "runner",
			SHA256: strings.Repeat("2", 64),
		}},
		Config: map[string]string{"limits.memory": "4GiB", "limits.cpu": "2"},
		Devices: map[string]map[string]string{
			"root": {"type": "disk", "path": "/", "pool": "default"},
		},
	}
	wantJSON := `{"version":1,"instance_type":"virtual-machine","image_fingerprint":"` +
		strings.Repeat("1", 64) + `","profiles":[{"name":"runner","sha256":"` +
		strings.Repeat("2", 64) + `"}],"config":{"limits.cpu":"2","limits.memory":"4GiB"},` +
		`"devices":{"root":{"path":"/","pool":"default","type":"disk"}}}`

	encoded, err := LaunchBytes(input)
	require.NoError(t, err)
	assert.True(t, bytes.Equal([]byte(wantJSON), encoded), "launch JSON bytes must remain exact")
	digest, err := LaunchDigest(input)
	require.NoError(t, err)
	assert.Equal(t, goldenLaunchDigest, digest)
	profileDigest, err := ProfileDigest(
		map[string]string{"limits.memory": "4GiB", "limits.cpu": "2"},
		map[string]map[string]string{"root": {"type": "disk", "path": "/", "pool": "default"}},
	)
	require.NoError(t, err)
	assert.Equal(t, goldenProfileDigest, profileDigest)
}

// TestProfileDigestPreservesNilMaps proves the existing nil-versus-empty byte distinction.
func TestProfileDigestPreservesNilMaps(t *testing.T) {
	t.Parallel()

	nilDigest, err := ProfileDigest(nil, nil)
	require.NoError(t, err)
	emptyDigest, err := ProfileDigest(map[string]string{}, map[string]map[string]string{})
	require.NoError(t, err)

	assert.NotEqual(t, nilDigest, emptyDigest)
}

// fixedPrivateKey returns a deterministic Ed25519 key for golden vectors.
func fixedPrivateKey(t *testing.T, offset byte) ed25519.PrivateKey {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index) + offset
	}

	return ed25519.NewKeyFromSeed(seed)
}

// fixedPayload returns one complete deterministic receipt.
func fixedPayload() Payload {
	return Payload{
		Version:  Version,
		Claim:    Claim,
		IssuedAt: time.Date(2026, 7, 20, 20, 15, 32, 123456000, time.UTC),
		Host: Host{
			ID:                "builder-host-01",
			ControllerVersion: "1.1.0",
			ControllerCommit:  "0123456789abcdef",
		},
		GitHub: GitHub{
			Owner:           "meigma",
			Repository:      "builder-images",
			WorkflowRef:     "meigma/builder-images/.github/workflows/build.yml@refs/heads/main",
			WorkflowRunID:   123456789,
			JobID:           "01234567-89ab-cdef-0123-456789abcdef",
			RunnerRequestID: 123456,
			RunnerID:        7890,
			RunnerName:      "incus-gh-runner-01234567-89ab-cdef-0123-456789abcdef",
			EventName:       "workflow_dispatch",
			ScaleSetID:      42,
			ScaleSetName:    "incus-linux-x64",
		},
		Machine: Machine{
			IncusProject:              "github-runners",
			InstanceName:              "incus-gh-runner-01234567-89ab-cdef-0123-456789abcdef",
			InstanceUUID:              "fedcba98-7654-3210-fedc-ba9876543210",
			ImageFingerprint:          strings.Repeat("1", 64),
			LaunchConfigurationSHA256: strings.Repeat("2", 64),
			Profiles: []Profile{{
				Name:   "github-runner",
				SHA256: strings.Repeat("3", 64),
			}},
		},
	}
}

// signRawEnvelope signs caller-provided payload bytes for negative verifier tests.
func signRawEnvelope(t *testing.T, privateKey ed25519.PrivateKey, payloadType string, body []byte) []byte {
	t.Helper()
	adapter, err := newEd25519Adapter(privateKey, privateKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)
	signer, err := dsse.NewEnvelopeSigner(adapter)
	require.NoError(t, err)
	envelope, err := signer.SignPayload(context.Background(), payloadType, body)
	require.NoError(t, err)

	return encodeEnvelope(t, *envelope)
}

// decodeEnvelope parses a trusted test fixture.
func decodeEnvelope(t *testing.T, data []byte) dsse.Envelope {
	t.Helper()
	var envelope dsse.Envelope
	require.NoError(t, json.Unmarshal(data, &envelope))

	return envelope
}

// encodeEnvelope serializes a test envelope.
func encodeEnvelope(t *testing.T, envelope dsse.Envelope) []byte {
	t.Helper()
	data, err := json.Marshal(envelope)
	require.NoError(t, err)

	return data
}

// encodePrivateKey creates the required PKCS#8 PEM test fixture.
func encodePrivateKey(t *testing.T, privateKey ed25519.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: privateKeyPEMType, Bytes: der})
}

// encodePublicKey creates the required SubjectPublicKeyInfo PEM test fixture.
func encodePublicKey(t *testing.T, publicKey ed25519.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: publicKeyPEMType, Bytes: der})
}
