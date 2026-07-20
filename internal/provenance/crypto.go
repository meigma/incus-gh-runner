package provenance

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"

	"github.com/secure-systems-lab/go-securesystemslib/dsse"
)

const (
	privateKeyPEMType = "PRIVATE KEY"
	publicKeyPEMType  = "PUBLIC KEY"
)

// Signer creates one-signature DSSE job machine proof envelopes.
type Signer struct {
	envelope *dsse.EnvelopeSigner
}

// NewSigner constructs a DSSE signer from one Ed25519 private key.
func NewSigner(privateKey ed25519.PrivateKey) (*Signer, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("Ed25519 private key has an invalid size")
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("derive Ed25519 public key")
	}
	adapter, err := newEd25519Adapter(privateKey, publicKey)
	if err != nil {
		return nil, err
	}
	envelope, err := dsse.NewEnvelopeSigner(adapter)
	if err != nil {
		return nil, fmt.Errorf("construct DSSE signer: %w", err)
	}

	return &Signer{envelope: envelope}, nil
}

// Sign validates and signs one complete version 1 payload.
func (s *Signer) Sign(ctx context.Context, payload Payload) ([]byte, error) {
	if s == nil || s.envelope == nil {
		return nil, errors.New("proof signer is not configured")
	}
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("validate proof payload: %w", err)
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode proof payload: %w", err)
	}
	if len(encodedPayload) > MaximumPayloadBytes {
		return nil, fmt.Errorf("proof payload exceeds %d bytes", MaximumPayloadBytes)
	}
	envelope, err := s.envelope.SignPayload(ctx, PayloadType, encodedPayload)
	if err != nil {
		return nil, fmt.Errorf("sign proof payload: %w", err)
	}
	encodedEnvelope, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode proof envelope: %w", err)
	}
	if len(encodedEnvelope) > MaximumEnvelopeBytes {
		return nil, fmt.Errorf("proof envelope exceeds %d bytes", MaximumEnvelopeBytes)
	}

	return encodedEnvelope, nil
}

// ParsePrivateKeyPEM parses exactly one PKCS#8 PEM-encoded Ed25519 private key.
func ParsePrivateKeyPEM(data []byte) (ed25519.PrivateKey, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != privateKeyPEMType || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("signing key must contain exactly one PKCS#8 PEM private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("signing key is not a valid PKCS#8 private key")
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("signing key must be an Ed25519 private key")
	}

	return bytes.Clone(privateKey), nil
}

// ParsePublicKeyPEM parses exactly one SubjectPublicKeyInfo PEM-encoded Ed25519 public key.
func ParsePublicKeyPEM(data []byte) (ed25519.PublicKey, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != publicKeyPEMType || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("public key must contain exactly one SubjectPublicKeyInfo PEM public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("public key is not valid SubjectPublicKeyInfo")
	}
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("public key must be an Ed25519 public key")
	}

	return bytes.Clone(publicKey), nil
}

// KeyID returns the version 1 key hint over SubjectPublicKeyInfo DER.
func KeyID(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", errors.New("Ed25519 public key has an invalid size")
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key identity: %w", err)
	}
	sum := sha256.Sum256(der)

	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// Verify authenticates one envelope and returns its exact verified payload JSON.
func Verify(
	ctx context.Context,
	encodedEnvelope []byte,
	publicKey ed25519.PublicKey,
	expectedHostID string,
) ([]byte, error) {
	if len(encodedEnvelope) == 0 {
		return nil, errors.New("proof envelope is empty")
	}
	if len(encodedEnvelope) > MaximumEnvelopeBytes {
		return nil, fmt.Errorf("proof envelope exceeds %d bytes", MaximumEnvelopeBytes)
	}
	if err := validateIdentity("expected host ID", expectedHostID); err != nil {
		return nil, err
	}
	adapter, err := newEd25519Adapter(nil, publicKey)
	if err != nil {
		return nil, err
	}
	var envelope dsse.Envelope
	if decodeErr := decodeStrictJSON(encodedEnvelope, &envelope); decodeErr != nil {
		return nil, fmt.Errorf("decode proof envelope: %w", decodeErr)
	}
	if envelope.PayloadType != PayloadType {
		return nil, errors.New("proof envelope payload type is not supported")
	}
	if len(envelope.Signatures) != 1 {
		return nil, errors.New("proof envelope must contain exactly one signature")
	}
	keyID, err := adapter.KeyID()
	if err != nil {
		return nil, err
	}
	if envelope.Signatures[0].KeyID != keyID {
		return nil, errors.New("proof envelope key ID does not match the selected public key")
	}
	decodedPayload, err := envelope.DecodeB64Payload()
	if err != nil {
		return nil, errors.New("proof envelope payload is not valid base64")
	}
	if len(decodedPayload) > MaximumPayloadBytes {
		return nil, fmt.Errorf("proof payload exceeds %d bytes", MaximumPayloadBytes)
	}
	verifier, err := dsse.NewEnvelopeVerifier(adapter)
	if err != nil {
		return nil, fmt.Errorf("construct DSSE verifier: %w", err)
	}
	accepted, verifiedPayload, err := verifier.VerifyAndDecode(ctx, &envelope)
	if err != nil {
		return nil, errors.New("proof signature verification failed")
	}
	if len(accepted) != 1 || !bytes.Equal(decodedPayload, verifiedPayload) {
		return nil, errors.New("proof signature verification returned an invalid result")
	}
	var payload Payload
	if decodeErr := decodeStrictJSON(verifiedPayload, &payload); decodeErr != nil {
		return nil, fmt.Errorf("decode verified proof payload: %w", decodeErr)
	}
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("validate verified proof payload: %w", err)
	}
	if payload.Host.ID != expectedHostID {
		return nil, errors.New("verified proof host ID does not match the expected host")
	}

	return bytes.Clone(verifiedPayload), nil
}

// ed25519Adapter supplies the local key ID and Ed25519 operations required by DSSE.
type ed25519Adapter struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

// newEd25519Adapter constructs one immutable DSSE Ed25519 adapter.
func newEd25519Adapter(privateKey ed25519.PrivateKey, publicKey ed25519.PublicKey) (*ed25519Adapter, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("Ed25519 public key has an invalid size")
	}
	if privateKey != nil && len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("Ed25519 private key has an invalid size")
	}
	keyID, err := KeyID(publicKey)
	if err != nil {
		return nil, err
	}

	return &ed25519Adapter{
		privateKey: bytes.Clone(privateKey),
		publicKey:  bytes.Clone(publicKey),
		keyID:      keyID,
	}, nil
}

// Sign creates a raw Ed25519 signature over DSSE pre-authentication bytes.
func (a *ed25519Adapter) Sign(ctx context.Context, data []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(a.privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("Ed25519 private key is unavailable")
	}

	return ed25519.Sign(a.privateKey, data), nil
}

// Verify authenticates a raw Ed25519 signature over DSSE pre-authentication bytes.
func (a *ed25519Adapter) Verify(ctx context.Context, data []byte, signature []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ed25519.Verify(a.publicKey, data, signature) {
		return errors.New("Ed25519 signature is invalid")
	}

	return nil
}

// KeyID returns the local SubjectPublicKeyInfo-based key hint.
func (a *ed25519Adapter) KeyID() (string, error) {
	return a.keyID, nil
}

// Public returns an isolated copy of the verification key.
func (a *ed25519Adapter) Public() crypto.PublicKey {
	return bytes.Clone(a.publicKey)
}

// decodeStrictJSON decodes exactly one JSON value while rejecting unknown fields.
func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON input contains more than one value")
		}
		return err
	}

	return nil
}
