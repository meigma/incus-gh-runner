package incus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	incusclient "github.com/lxc/incus/v7/client"

	"github.com/meigma/incus-gh-runner/internal/config"
)

const pinnedCertificateMismatch = "remote certificate does not match incus.server_cert_file"

// pinningTransport preserves the Incus HTTP transport while exposing it to websocket dialers.
type pinningTransport struct {
	wrapped *http.Transport
}

// RoundTrip rejects plaintext redirects before using the pinned Incus transport.
func (p *pinningTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, errors.New("incus HTTPS connections cannot send plaintext requests")
	}
	return p.wrapped.RoundTrip(req)
}

// Transport returns the wrapped Incus HTTP transport used by websocket dialers.
func (p *pinningTransport) Transport() *http.Transport {
	return p.wrapped
}

// Connect constructs an Incus client for the configured transport and project.
//
// Connect loads HTTPS credentials from disk, pins the server certificate during
// the TLS handshake, and checks GetProject for the configured project so image
// and profile inheritance cannot hide a denied project.
func Connect(
	ctx context.Context,
	connection config.IncusConnection,
	project string,
) (incusclient.InstanceServer, error) {
	if err := connection.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("incus.project is required")
	}

	var server incusclient.InstanceServer
	var err error
	if strings.TrimSpace(connection.Socket) != "" {
		server, err = connectUnix(ctx, connection.Socket)
	} else {
		clientCert, clientKey, serverCert, loadErr := loadHTTPSCredentials(connection)
		if loadErr != nil {
			return nil, loadErr
		}
		server, err = connectHTTPS(ctx, connection.URL, clientCert, clientKey, serverCert)
	}
	if err != nil {
		return nil, err
	}

	if _, _, err := server.GetProject(project); err != nil {
		server.Disconnect()
		return nil, fmt.Errorf("access Incus project %q: %w", project, err)
	}

	return server.UseProject(project), nil
}

// ConnectUnix constructs a client for a project on a local Incus Unix socket.
func ConnectUnix(ctx context.Context, socketPath string, project string) (incusclient.InstanceServer, error) {
	server, err := connectUnix(ctx, socketPath)
	if err != nil {
		return nil, err
	}

	return server.UseProject(project), nil
}

// ConnectHTTPS constructs a client for a project on a remote Incus HTTPS endpoint.
func ConnectHTTPS(
	ctx context.Context,
	remoteURL string,
	clientCert string,
	clientKey string,
	serverCert string,
	project string,
) (incusclient.InstanceServer, error) {
	server, err := connectHTTPS(ctx, remoteURL, clientCert, clientKey, serverCert)
	if err != nil {
		return nil, err
	}

	return server.UseProject(project), nil
}

// connectUnix opens an Incus Unix socket without selecting a project.
func connectUnix(ctx context.Context, socketPath string) (incusclient.InstanceServer, error) {
	return incusclient.ConnectIncusUnixWithContext(ctx, socketPath, nil)
}

// connectHTTPS opens an Incus HTTPS endpoint with a pinned server certificate.
func connectHTTPS(
	ctx context.Context,
	remoteURL string,
	clientCert string,
	clientKey string,
	serverCert string,
) (incusclient.InstanceServer, error) {
	if err := validateRemoteHTTPSURL(remoteURL); err != nil {
		return nil, err
	}
	if _, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey)); err != nil {
		return nil, fmt.Errorf("parse incus.client_cert_file and incus.client_key_file: %w", err)
	}
	pinned, err := parseServerCertificate(serverCert)
	if err != nil {
		return nil, err
	}

	args := &incusclient.ConnectionArgs{
		TLSClientCert:    clientCert,
		TLSClientKey:     clientKey,
		TLSServerCert:    serverCert,
		TransportWrapper: pinServerCertificate(pinned),
	}
	return incusclient.ConnectIncusWithContext(ctx, remoteURL, args)
}

// loadHTTPSCredentials reads the HTTPS credential files for connection-time parsing.
func loadHTTPSCredentials(connection config.IncusConnection) (string, string, string, error) {
	clientCert, err := readConnectionFile(connection.ClientCertFile, "incus.client_cert_file")
	if err != nil {
		return "", "", "", err
	}
	clientKey, err := readConnectionFile(connection.ClientKeyFile, "incus.client_key_file")
	if err != nil {
		return "", "", "", err
	}
	serverCert, err := readConnectionFile(connection.ServerCertFile, "incus.server_cert_file")
	if err != nil {
		return "", "", "", err
	}

	return clientCert, clientKey, serverCert, nil
}

// readConnectionFile reads one credential file without including its contents in errors.
func readConnectionFile(path string, field string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", field, err)
	}

	return string(data), nil
}

// parseServerCertificate decodes the pinned Incus server certificate.
func parseServerCertificate(pemBytes string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemBytes))
	if block == nil {
		return nil, errors.New("parse incus.server_cert_file: invalid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse incus.server_cert_file: %w", err)
	}

	return cert, nil
}

// validateRemoteHTTPSURL checks that the remote Incus endpoint is an absolute HTTPS URL.
func validateRemoteHTTPSURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || strings.TrimSpace(raw) != raw || parsed.Opaque != "" ||
		!strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil {
		return errors.New("incus.url must be an absolute HTTPS URL")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" {
		return errors.New("incus.url must be a root endpoint without a path, query, or fragment")
	}

	return nil
}

// pinServerCertificate adds an exact leaf match on top of normal TLS verification.
//
// TLSServerCert only adds the pin to the system CA pool. IdenticalCertificate
// skips expiry and hostname checks. VerifyConnection keeps normal verification
// and requires the presented leaf to equal the pinned certificate. The wrapper
// is installed before GetServer so HTTP and websocket dialers share the pin.
func pinServerCertificate(pinned *x509.Certificate) func(*http.Transport) incusclient.HTTPTransporter {
	return func(transport *http.Transport) incusclient.HTTPTransporter {
		tlsConfig := transport.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
			transport.TLSClientConfig = tlsConfig
		}
		previous := tlsConfig.VerifyConnection
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if previous != nil {
				if err := previous(state); err != nil {
					return err
				}
			}
			if len(state.PeerCertificates) == 0 {
				return errors.New(pinnedCertificateMismatch)
			}
			if !state.PeerCertificates[0].Equal(pinned) {
				return errors.New(pinnedCertificateMismatch)
			}

			return nil
		}

		return &pinningTransport{wrapped: transport}
	}
}
