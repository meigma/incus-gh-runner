package incus

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	incusclient "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
)

const functionalTestProject = "runners"

func TestConnectHTTPSValidMTLS(t *testing.T) {
	t.Parallel()

	fixture := newPinnedIncusFixture(t)
	server := connectPinnedIncus(t, fixture)

	info, err := server.GetConnectionInfo()
	require.NoError(t, err)
	assert.Equal(t, functionalTestProject, info.Project)
}

func TestConnectHTTPSRejectsMissingPinFile(t *testing.T) {
	t.Parallel()

	fixture := newPinnedIncusFixture(t)
	connection := fixture.connection
	connection.ServerCertFile = filepath.Join(t.TempDir(), "missing-server.crt")

	_, err := Connect(context.Background(), connection, functionalTestProject)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "incus.server_cert_file")
	assert.NotContains(t, err.Error(), "BEGIN")
}

func TestConnectHTTPSRejectsWrongPin(t *testing.T) {
	t.Parallel()

	fixture := newPinnedIncusFixture(t)
	other := issueCertificate(t, certificateOptions{commonName: "other", serverAuth: true})
	connection := fixture.connection
	connection.ServerCertFile = writePEMFile(t, t.TempDir(), "other.crt", other.certPEM)

	_, err := Connect(context.Background(), connection, functionalTestProject)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "BEGIN")
}

func TestConnectHTTPSRejectsCAValidWrongCertificate(t *testing.T) {
	t.Parallel()

	ca := issueCertificate(t, certificateOptions{commonName: "ca", isCA: true})
	leaf := issueCertificate(t, certificateOptions{
		commonName: "incus",
		serverAuth: true,
		parent:     ca.cert,
		parentKey:  ca.key,
	})
	client := issueCertificate(t, certificateOptions{commonName: "controller", clientAuth: true})
	fake := startFakeIncus(t, fakeIncusOptions{
		serverCert: leaf.tlsCertificate(t),
		clientCA:   certPool(t, client.cert),
		project:    functionalTestProject,
	})
	dir := t.TempDir()
	connection := config.IncusConnection{
		URL:            fake.URL,
		ClientCertFile: writePEMFile(t, dir, "client.crt", client.certPEM),
		ClientKeyFile:  writePEMFile(t, dir, "client.key", client.keyPEM),
		ServerCertFile: writePEMFile(t, dir, "server.crt", ca.certPEM),
	}

	_, err := Connect(context.Background(), connection, functionalTestProject)

	require.Error(t, err)
	assert.Contains(t, err.Error(), pinnedCertificateMismatch)
	assert.NotContains(t, err.Error(), "BEGIN")
}

func TestConnectHTTPSRejectsMismatchedClientKey(t *testing.T) {
	t.Parallel()

	fixture := newPinnedIncusFixture(t)
	other := issueCertificate(t, certificateOptions{commonName: "other", clientAuth: true})
	connection := fixture.connection
	connection.ClientKeyFile = writePEMFile(t, t.TempDir(), "other.key", other.keyPEM)

	_, err := Connect(context.Background(), connection, functionalTestProject)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "incus.client_cert_file")
	assert.Contains(t, err.Error(), "incus.client_key_file")
	assert.NotContains(t, err.Error(), "BEGIN")
}

func TestConnectHTTPSRejectsMalformedFiles(t *testing.T) {
	t.Parallel()

	fixture := newPinnedIncusFixture(t)
	tests := []struct {
		name  string
		field string
		path  string
	}{
		{
			name:  "malformed client certificate",
			field: "incus.client_cert_file",
			path:  writePEMFile(t, t.TempDir(), "client.crt", []byte("not-a-certificate")),
		},
		{
			name:  "malformed server certificate",
			field: "incus.server_cert_file",
			path:  writePEMFile(t, t.TempDir(), "server.crt", []byte("not-a-certificate")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			connection := fixture.connection
			if tt.field == "incus.client_cert_file" {
				connection.ClientCertFile = tt.path
			} else {
				connection.ServerCertFile = tt.path
			}

			_, err := Connect(context.Background(), connection, functionalTestProject)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.field)
			assert.NotContains(t, err.Error(), "BEGIN")
			assert.NotContains(t, err.Error(), "not-a-certificate")
		})
	}
}

func TestConnectHTTPSRejectsExpiredServerCertificate(t *testing.T) {
	t.Parallel()

	expired := issueCertificate(t, certificateOptions{commonName: "incus", serverAuth: true, expired: true})
	client := issueCertificate(t, certificateOptions{commonName: "controller", clientAuth: true})
	fake := startFakeIncus(t, fakeIncusOptions{
		serverCert: expired.tlsCertificate(t),
		clientCA:   certPool(t, client.cert),
		project:    functionalTestProject,
	})
	dir := t.TempDir()
	connection := config.IncusConnection{
		URL:            fake.URL,
		ClientCertFile: writePEMFile(t, dir, "client.crt", client.certPEM),
		ClientKeyFile:  writePEMFile(t, dir, "client.key", client.keyPEM),
		ServerCertFile: writePEMFile(t, dir, "server.crt", expired.certPEM),
	}

	_, err := Connect(context.Background(), connection, functionalTestProject)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "BEGIN")
}

func TestConnectHTTPSWebSocketUsesPinnedCertificate(t *testing.T) {
	t.Parallel()

	fixture := newPinnedIncusFixture(t)
	server := connectPinnedIncus(t, fixture)

	listener, err := server.GetEvents()
	require.NoError(t, err)
	listener.Disconnect()
}

func TestConnectHTTPSWebSocketRejectsUnpinnedCertificate(t *testing.T) {
	t.Parallel()

	fixture := newPinnedIncusFixture(t)
	server := connectPinnedIncus(t, fixture)
	other := issueCertificate(t, certificateOptions{commonName: "other", serverAuth: true})
	unpinned := startFakeIncus(t, fakeIncusOptions{
		serverCert: other.tlsCertificate(t),
		clientCA:   certPool(t, fixture.client.cert),
		project:    functionalTestProject,
	})

	httpClient, err := server.GetHTTPClient()
	require.NoError(t, err)
	transporter, ok := httpClient.Transport.(incusclient.HTTPTransporter)
	require.True(t, ok, "HTTPS client must expose the pinning transport to websocket dialers")
	transport := transporter.Transport()
	dialer := websocket.Dialer{
		NetDialTLSContext: transport.DialTLSContext,
		TLSClientConfig:   transport.TLSClientConfig,
		HandshakeTimeout:  5 * time.Second,
	}

	_, _, err = dialer.Dial("wss://"+unpinned.Host+"/1.0/events", nil)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "BEGIN")
}

func TestConnectRejectsDeniedProject(t *testing.T) {
	t.Parallel()

	client := issueCertificate(t, certificateOptions{commonName: "controller", clientAuth: true})
	serverCert := issueCertificate(t, certificateOptions{commonName: "incus", serverAuth: true})
	fake := startFakeIncus(t, fakeIncusOptions{
		serverCert:  serverCert.tlsCertificate(t),
		clientCA:    certPool(t, client.cert),
		project:     functionalTestProject,
		denyProject: true,
	})
	dir := t.TempDir()
	connection := config.IncusConnection{
		URL:            fake.URL,
		ClientCertFile: writePEMFile(t, dir, "client.crt", client.certPEM),
		ClientKeyFile:  writePEMFile(t, dir, "client.key", client.keyPEM),
		ServerCertFile: writePEMFile(t, dir, "server.crt", serverCert.certPEM),
	}

	_, err := Connect(context.Background(), connection, functionalTestProject)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `access Incus project "runners"`)
}

func TestConnectHTTPSRejectsPlaintextURL(t *testing.T) {
	t.Parallel()

	_, err := ConnectHTTPS(
		context.Background(),
		"http://127.0.0.1:8443",
		"cert",
		"key",
		"server",
		functionalTestProject,
	)

	require.EqualError(t, err, "incus.url must be an absolute HTTPS URL")
}

// TestConnectHTTPSRejectsPlaintextRedirect prevents an authenticated endpoint from bypassing the pin.
func TestConnectHTTPSRejectsPlaintextRedirect(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	plaintext := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeIncusSync(writer, map[string]any{"auth": "trusted", "api_extensions": []string{"projects"}})
	}))
	t.Cleanup(plaintext.Close)
	client := issueCertificate(t, certificateOptions{commonName: "controller", clientAuth: true})
	pinned := issueCertificate(t, certificateOptions{commonName: "incus", serverAuth: true})
	endpoint := startFakeIncus(t, fakeIncusOptions{
		serverCert:  pinned.tlsCertificate(t),
		clientCA:    certPool(t, client.cert),
		redirectURL: plaintext.URL,
	})

	server, err := ConnectHTTPS(context.Background(), endpoint.URL,
		string(client.certPEM), string(client.keyPEM), string(pinned.certPEM), functionalTestProject)
	if server != nil {
		t.Cleanup(server.Disconnect)
	}
	require.Error(t, err)
	assert.Zero(t, requests.Load(), "an HTTPS client must not send a request over plaintext")
}

type pinnedIncusFixture struct {
	connection config.IncusConnection
	client     testCertificate
	server     testCertificate
	fake       *fakeIncus
}

func newPinnedIncusFixture(t *testing.T) pinnedIncusFixture {
	t.Helper()
	client := issueCertificate(t, certificateOptions{commonName: "controller", clientAuth: true})
	serverCert := issueCertificate(t, certificateOptions{commonName: "incus", serverAuth: true})
	fake := startFakeIncus(t, fakeIncusOptions{
		serverCert: serverCert.tlsCertificate(t),
		clientCA:   certPool(t, client.cert),
		project:    functionalTestProject,
	})
	dir := t.TempDir()
	return pinnedIncusFixture{
		connection: config.IncusConnection{
			URL:            fake.URL,
			ClientCertFile: writePEMFile(t, dir, "client.crt", client.certPEM),
			ClientKeyFile:  writePEMFile(t, dir, "client.key", client.keyPEM),
			ServerCertFile: writePEMFile(t, dir, "server.crt", serverCert.certPEM),
		},
		client: client,
		server: serverCert,
		fake:   fake,
	}
}

func connectPinnedIncus(t *testing.T, fixture pinnedIncusFixture) incusclient.InstanceServer {
	t.Helper()
	server, err := Connect(context.Background(), fixture.connection, functionalTestProject)
	require.NoError(t, err)
	t.Cleanup(server.Disconnect)
	return server
}

type fakeIncusOptions struct {
	serverCert  tls.Certificate
	clientCA    *x509.CertPool
	project     string
	denyProject bool
	redirectURL string
}

type fakeIncus struct {
	URL  string
	Host string
}

func startFakeIncus(t *testing.T, options fakeIncusOptions) *fakeIncus {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /1.0", func(writer http.ResponseWriter, request *http.Request) {
		if options.redirectURL != "" {
			http.Redirect(writer, request, options.redirectURL, http.StatusFound)
			return
		}
		writeIncusSync(writer, map[string]any{
			"api_extensions": []string{"projects"},
			"api_status":     "stable",
			"api_version":    "1.0",
			"auth":           "trusted",
			"public":         false,
			"environment":    map[string]any{},
		})
	})
	mux.HandleFunc("GET /1.0/projects/{name}", func(writer http.ResponseWriter, request *http.Request) {
		name := request.PathValue("name")
		if options.denyProject || name != options.project {
			writeIncusError(writer, http.StatusForbidden, "Not authorized")
			return
		}
		writeIncusSync(writer, map[string]any{
			"name":    name,
			"config":  map[string]string{},
			"used_by": []string{},
		})
	})
	mux.HandleFunc("/1.0/events", func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsListener := tls.NewListener(listener, &tls.Config{
		Certificates: []tls.Certificate{options.serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    options.clientCA,
		MinVersion:   tls.VersionTLS13,
	})
	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(tlsListener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	host := listener.Addr().String()
	return &fakeIncus{URL: "https://" + host, Host: host}
}

func writeIncusSync(writer http.ResponseWriter, metadata any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"type":        "sync",
		"status":      "Success",
		"status_code": 200,
		"metadata":    metadata,
	})
}

func writeIncusError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(api.Response{
		Type:       api.ErrorResponse,
		Error:      message,
		Code:       status,
		StatusCode: status,
	})
}

type certificateOptions struct {
	commonName string
	isCA       bool
	expired    bool
	clientAuth bool
	serverAuth bool
	parent     *x509.Certificate
	parentKey  *ecdsa.PrivateKey
}

type testCertificate struct {
	certPEM []byte
	keyPEM  []byte
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
}

func (c testCertificate) tlsCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	parsed, err := tls.X509KeyPair(c.certPEM, c.keyPEM)
	require.NoError(t, err)
	return parsed
}

func issueCertificate(t *testing.T, options certificateOptions) testCertificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	now := time.Now()
	notBefore := now.Add(-time.Hour)
	notAfter := now.Add(24 * time.Hour)
	if options.expired {
		notBefore = now.Add(-2 * time.Hour)
		notAfter = now.Add(-time.Hour)
	}

	usage := x509.KeyUsageDigitalSignature
	if options.isCA {
		usage |= x509.KeyUsageCertSign
	}
	var ext []x509.ExtKeyUsage
	if options.serverAuth {
		ext = append(ext, x509.ExtKeyUsageServerAuth)
	}
	if options.clientAuth {
		ext = append(ext, x509.ExtKeyUsageClientAuth)
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: options.commonName},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              usage,
		ExtKeyUsage:           ext,
		BasicConstraintsValid: true,
		IsCA:                  options.isCA,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	parent := template
	signer := key
	if options.parent != nil {
		parent = options.parent
		signer = options.parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	return testCertificate{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}),
		cert:    cert,
		key:     key,
	}
}

func certPool(t *testing.T, cert *x509.Certificate) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})))
	return pool
}

func writePEMFile(t *testing.T, directory string, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestConnectRequiresProject(t *testing.T) {
	t.Parallel()

	_, err := Connect(context.Background(), config.IncusConnection{Socket: "/var/lib/incus/unix.socket"}, "")

	require.EqualError(t, err, "incus.project is required")
}

func TestConnectRejectsInvalidConnection(t *testing.T) {
	t.Parallel()

	_, err := Connect(context.Background(), config.IncusConnection{}, functionalTestProject)

	require.EqualError(t, err, "configure exactly one of incus.socket or incus.url")
}
