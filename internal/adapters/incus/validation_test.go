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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/config"
	"github.com/meigma/incus-gh-runner/internal/incusvalidate"
)

// validationRequestRecorder stores requests received by a fake Incus daemon.
type validationRequestRecorder struct {
	mu       sync.Mutex
	requests []string
}

// append records one request method and URI.
func (r *validationRequestRecorder) append(request *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request.Method+" "+request.URL.RequestURI())
}

// snapshot returns an independent copy of the recorded requests.
func (r *validationRequestRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// TestValidationReaderUsesOnlyExactGETs proves the socket adapter cannot mutate Incus state.
func TestValidationReaderUsesOnlyExactGETs(t *testing.T) {
	t.Parallel()

	socketPath, recorder := startValidationIncusServer(t)
	server, err := connectUnix(context.Background(), socketPath)
	require.NoError(t, err)
	reader := NewValidationReader(server)
	t.Cleanup(reader.Close)

	_, err = reader.Read(context.Background(), validationReaderNames())
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{
		"GET /1.0",
		"GET /1.0",
		"GET /1.0/projects/github-runners",
		"GET /1.0/networks/runner-network?project=default",
		"GET /1.0/network-acls/runner-egress?project=default",
		"GET /1.0/profiles/runner?project=github-runners",
		"GET /1.0/storage-pools/runner-storage",
	}, recorder.snapshot())
}

// startValidationIncusServer starts the smallest Unix-socket Incus API used by the reader.
func startValidationIncusServer(t *testing.T) (string, *validationRequestRecorder) {
	t.Helper()

	tempDir, err := os.MkdirTemp( //nolint:usetesting // t.TempDir exceeds Unix socket length limits on macOS.
		"/tmp",
		"incus-validator-",
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(tempDir))
	})
	socketPath := filepath.Join(tempDir, "socket")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	recorder := &validationRequestRecorder{}
	server := &http.Server{
		Handler: validationIncusHandler(t, recorder),
	}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	return socketPath, recorder
}

// validationResponse returns one minimal API resource for an expected read path.
func validationResponse(path string) (any, bool) {
	responses := map[string]any{
		"/1.0": map[string]any{
			"auth":           "trusted",
			"api_extensions": []string{"projects", "network", "network_acl", "storage"},
			"config":         map[string]string{},
			"environment": map[string]any{
				"server_version":   "7.2.0",
				"server_clustered": false,
				"firewall":         "nftables",
			},
		},
		"/1.0/projects/github-runners": map[string]any{
			"name": "github-runners", "description": "runner project", "config": map[string]string{},
		},
		"/1.0/networks/runner-network": map[string]any{
			"name": "runner-network", "description": "runner network", "type": "bridge", "managed": true,
			"config": map[string]string{},
		},
		"/1.0/network-acls/runner-egress": map[string]any{
			"name": "runner-egress", "description": "runner ACL", "config": map[string]string{},
			"ingress": []any{}, "egress": []any{},
		},
		"/1.0/profiles/runner": map[string]any{
			"name": "runner", "description": "runner profile", "config": map[string]string{},
			"devices": map[string]any{},
		},
		"/1.0/storage-pools/runner-storage": map[string]any{
			"name": "runner-storage", "description": "runner storage", "driver": "zfs",
			"config": map[string]string{},
		},
	}
	response, ok := responses[path]
	return response, ok
}

// TestValidationNetworkACLProjectsEveryWritableField proves extra rule authority cannot disappear.
func TestValidationNetworkACLProjectsEveryWritableField(t *testing.T) {
	t.Parallel()

	acl := &api.NetworkACL{NetworkACLPut: api.NetworkACLPut{
		Description: "runner ACL",
		Config:      map[string]string{},
		Egress: []api.NetworkACLRule{{
			Action:          " allow ",
			Source:          " 10.0.0.1/32 ",
			Destination:     " 192.0.2.10/32 ",
			Protocol:        " tcp ",
			SourcePort:      " 1000 ",
			DestinationPort: " 3128 ",
			ICMPType:        " 8 ",
			ICMPCode:        " 0 ",
			Description:     " proxy ",
			State:           " enabled ",
		}},
	}}

	actual := validationNetworkACL(acl)

	assert.Equal(t, []incusvalidate.NetworkACLRule{{
		Action:          "allow",
		Source:          "10.0.0.1/32",
		Destination:     "192.0.2.10/32",
		Protocol:        "tcp",
		SourcePort:      "1000",
		DestinationPort: "3128",
		ICMPType:        "8",
		ICMPCode:        "0",
		Description:     "proxy",
		State:           "enabled",
	}}, actual.Egress)
}

// TestValidationProfileCopiesDevices proves snapshot data is independent of SDK objects.
func TestValidationProfileCopiesDevices(t *testing.T) {
	t.Parallel()

	profile := &api.Profile{ProfilePut: api.ProfilePut{
		Description: "runner profile",
		Config:      map[string]string{"security.nesting": "false"},
		Devices:     map[string]map[string]string{"root": {"type": "disk"}},
	}}

	actual := validationProfile(profile)
	profile.Config["security.nesting"] = "true"
	profile.Devices["root"]["type"] = "none"

	assert.Equal(t, "false", actual.Config["security.nesting"])
	assert.Equal(t, "disk", actual.Devices["root"]["type"])
}

// TestValidationReaderHTTPSUsesOnlyExactGETs proves HTTPS validation still only reads named resources.
func TestValidationReaderHTTPSUsesOnlyExactGETs(t *testing.T) {
	t.Parallel()

	env := startValidationIncusHTTPSServer(t)
	server, err := Connect(context.Background(), config.IncusConnection{
		URL:            env.url,
		ClientCertFile: env.clientCertFile,
		ClientKeyFile:  env.clientKeyFile,
		ServerCertFile: env.serverCertFile,
	}, "github-runners")
	require.NoError(t, err)
	reader := NewValidationReader(server)
	t.Cleanup(reader.Close)

	_, err = reader.Read(context.Background(), validationReaderNames())
	require.NoError(t, err)
	assertValidationReadOnlyGETs(t, env.recorder.snapshot())
}

// TestConnectRejectsMismatchedServerCertificatePin proves a wrong pin cannot read Incus state.
func TestConnectRejectsMismatchedServerCertificatePin(t *testing.T) {
	t.Parallel()

	env := startValidationIncusHTTPSServer(t)
	_, err := Connect(context.Background(), config.IncusConnection{
		URL:            env.url,
		ClientCertFile: env.clientCertFile,
		ClientKeyFile:  env.clientKeyFile,
		ServerCertFile: env.wrongCertFile,
	}, "github-runners")
	require.Error(t, err)
}

func validationReaderNames() incusvalidate.Names {
	return incusvalidate.Names{
		Project:     "github-runners",
		Network:     "runner-network",
		NetworkACL:  "runner-egress",
		Profile:     "runner",
		StoragePool: "runner-storage",
	}
}

func validationIncusHandler(t *testing.T, recorder *validationRequestRecorder) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		recorder.append(request)
		if request.Method != http.MethodGet {
			http.Error(writer, "read-only test server", http.StatusMethodNotAllowed)
			return
		}

		metadata, ok := validationResponse(request.URL.Path)
		if !ok {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(api.ResponseRaw{
			Type:       api.SyncResponse,
			Status:     "Success",
			StatusCode: http.StatusOK,
			Metadata:   metadata,
		}); err != nil {
			t.Errorf("encode fake Incus response: %v", err)
		}
	})
}

func assertValidationReadOnlyGETs(t *testing.T, requests []string) {
	t.Helper()
	for _, request := range requests {
		assert.True(t, strings.HasPrefix(request, "GET "), "validation issued a mutating request: %s", request)
	}
	for _, want := range []string{
		"GET /1.0/projects/github-runners",
		"GET /1.0/networks/runner-network?project=default",
		"GET /1.0/network-acls/runner-egress?project=default",
		"GET /1.0/profiles/runner?project=github-runners",
	} {
		assert.Contains(t, requests, want)
	}
}

type validationHTTPSEnv struct {
	url            string
	clientCertFile string
	clientKeyFile  string
	serverCertFile string
	wrongCertFile  string
	recorder       *validationRequestRecorder
}

func startValidationIncusHTTPSServer(t *testing.T) validationHTTPSEnv {
	t.Helper()

	directory := t.TempDir()
	clientCertPEM, clientKeyPEM, _ := generateValidationTLSCertificate(t)
	serverCertPEM, _, serverTLS := generateValidationTLSCertificate(t, "127.0.0.1", "localhost")
	wrongCertPEM, _, _ := generateValidationTLSCertificate(t, "127.0.0.1")

	clientPool := x509.NewCertPool()
	require.True(t, clientPool.AppendCertsFromPEM(clientCertPEM))
	recorder := &validationRequestRecorder{}
	server := httptest.NewUnstartedServer(validationIncusHandler(t, recorder))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverTLS},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientPool,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	return validationHTTPSEnv{
		url:            server.URL,
		clientCertFile: writeValidationPEM(t, directory, "client.crt", clientCertPEM),
		clientKeyFile:  writeValidationPEM(t, directory, "client.key", clientKeyPEM),
		serverCertFile: writeValidationPEM(t, directory, "server.crt", serverCertPEM),
		wrongCertFile:  writeValidationPEM(t, directory, "wrong.crt", wrongCertPEM),
		recorder:       recorder,
	}
}

func writeValidationPEM(t *testing.T, directory string, name string, pemBytes []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	require.NoError(t, os.WriteFile(path, pemBytes, 0o600))
	return path
}

func generateValidationTLSCertificate(t *testing.T, hosts ...string) ([]byte, []byte, tls.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "incus-validator-test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return certPEM, keyPEM, tlsCert
}
