package incus

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	reader, err := ConnectValidationReader(context.Background(), socketPath)
	require.NoError(t, err)
	t.Cleanup(reader.Close)

	_, err = reader.Read(context.Background(), incusvalidate.Names{
		Project:     "github-runners",
		Network:     "runner-network",
		NetworkACL:  "runner-egress",
		Profile:     "runner",
		StoragePool: "runner-storage",
	})
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
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
		}),
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
