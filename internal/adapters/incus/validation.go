package incus

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	incusclient "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"

	"github.com/meigma/incus-gh-runner/internal/incusvalidate"
)

// ValidationReader reads only the named Incus resources required for baseline validation.
type ValidationReader struct {
	server incusclient.InstanceServer
}

var _ incusvalidate.Reader = (*ValidationReader)(nil)

// ConnectValidationReader connects a read-only validator to one explicit local Incus socket.
func ConnectValidationReader(ctx context.Context, socketPath string) (*ValidationReader, error) {
	if strings.TrimSpace(socketPath) == "" {
		return nil, errors.New("incus validation socket path is required")
	}

	server, err := incusclient.ConnectIncusUnixWithContext(ctx, socketPath, nil)
	if err != nil {
		return nil, fmt.Errorf("connect Incus validation socket: %w", err)
	}

	return &ValidationReader{server: server}, nil
}

// Close releases resources held by the Incus client.
func (r *ValidationReader) Close() {
	if r == nil || r.server == nil {
		return
	}
	r.server.Disconnect()
}

// Read returns a fresh snapshot using only Incus GET operations.
func (r *ValidationReader) Read(ctx context.Context, names incusvalidate.Names) (incusvalidate.Snapshot, error) {
	if r == nil || r.server == nil {
		return incusvalidate.Snapshot{}, errors.New("incus validation reader is not connected")
	}

	server, err := validationServerWithContext(ctx, r.server)
	if err != nil {
		return incusvalidate.Snapshot{}, err
	}
	serverState, _, err := server.GetServer()
	if err != nil {
		return incusvalidate.Snapshot{}, fmt.Errorf("read server: %w", err)
	}
	project, _, err := server.GetProject(names.Project)
	if err != nil {
		return incusvalidate.Snapshot{}, fmt.Errorf("read project %q: %w", names.Project, err)
	}

	defaultProject := server.UseProject(api.ProjectDefaultName)
	network, _, err := defaultProject.GetNetwork(names.Network)
	if err != nil {
		return incusvalidate.Snapshot{}, fmt.Errorf("read default-project network %q: %w", names.Network, err)
	}
	networkACL, _, err := defaultProject.GetNetworkACL(names.NetworkACL)
	if err != nil {
		return incusvalidate.Snapshot{}, fmt.Errorf("read default-project network ACL %q: %w", names.NetworkACL, err)
	}

	runnerProject := server.UseProject(names.Project)
	profile, _, err := runnerProject.GetProfile(names.Profile)
	if err != nil {
		return incusvalidate.Snapshot{}, fmt.Errorf("read runner-project profile %q: %w", names.Profile, err)
	}
	storagePool, _, err := server.GetStoragePool(names.StoragePool)
	if err != nil {
		return incusvalidate.Snapshot{}, fmt.Errorf("read storage pool %q: %w", names.StoragePool, err)
	}

	return incusvalidate.Snapshot{
		Server:      validationServerState(serverState),
		Project:     validationProject(project),
		Network:     validationNetwork(network),
		NetworkACL:  validationNetworkACL(networkACL),
		Profile:     validationProfile(profile),
		StoragePool: validationStoragePool(storagePool),
	}, nil
}

// validationServerWithContext returns an SDK view bound to the command context.
func validationServerWithContext(
	ctx context.Context,
	server incusclient.InstanceServer,
) (incusclient.InstanceServer, error) {
	contextual, ok := server.(interface {
		WithContext(context.Context) incusclient.InstanceServer
	})
	if !ok {
		return nil, errors.New("incus server does not support request contexts")
	}

	return contextual.WithContext(ctx), nil
}

// validationServerState projects the daemon fields enforced by the baseline.
func validationServerState(server *api.Server) incusvalidate.ServerState {
	return incusvalidate.ServerState{
		Auth:           server.Auth,
		APIExtensions:  append([]string(nil), server.APIExtensions...),
		Config:         maps.Clone(server.Config),
		Version:        server.Environment.ServerVersion,
		Clustered:      server.Environment.ServerClustered,
		FirewallDriver: server.Environment.Firewall,
	}
}

// validationProject projects writable project state.
func validationProject(project *api.Project) incusvalidate.Project {
	writable := project.Writable()
	return incusvalidate.Project{
		Description: writable.Description,
		Config:      maps.Clone(writable.Config),
	}
}

// validationNetwork projects writable network state plus immutable type and management fields.
func validationNetwork(network *api.Network) incusvalidate.Network {
	writable := network.Writable()
	return incusvalidate.Network{
		Description: writable.Description,
		Type:        network.Type,
		Managed:     network.Managed,
		Config:      maps.Clone(writable.Config),
	}
}

// validationNetworkACL projects and normalizes every writable ACL field.
func validationNetworkACL(networkACL *api.NetworkACL) incusvalidate.NetworkACL {
	writable := networkACL.Writable()
	return incusvalidate.NetworkACL{
		Description: writable.Description,
		Config:      maps.Clone(writable.Config),
		Ingress:     validationNetworkACLRules(writable.Ingress),
		Egress:      validationNetworkACLRules(writable.Egress),
	}
}

// validationNetworkACLRules copies and normalizes ACL rules returned by Incus.
func validationNetworkACLRules(rules []api.NetworkACLRule) []incusvalidate.NetworkACLRule {
	result := make([]incusvalidate.NetworkACLRule, 0, len(rules))
	for _, rule := range rules {
		rule.Normalise()
		result = append(result, incusvalidate.NetworkACLRule{
			Action:          rule.Action,
			Source:          rule.Source,
			Destination:     rule.Destination,
			Protocol:        rule.Protocol,
			SourcePort:      rule.SourcePort,
			DestinationPort: rule.DestinationPort,
			ICMPType:        rule.ICMPType,
			ICMPCode:        rule.ICMPCode,
			Description:     rule.Description,
			State:           rule.State,
		})
	}

	return result
}

// validationProfile projects writable profile state.
func validationProfile(profile *api.Profile) incusvalidate.Profile {
	writable := profile.Writable()
	return incusvalidate.Profile{
		Description: writable.Description,
		Config:      maps.Clone(writable.Config),
		Devices:     cloneDevices(writable.Devices),
	}
}

// cloneDevices returns an independent copy of an Incus device map.
func cloneDevices(devices map[string]map[string]string) map[string]map[string]string {
	cloned := make(map[string]map[string]string, len(devices))
	for name, config := range devices {
		cloned[name] = maps.Clone(config)
	}

	return cloned
}

// validationStoragePool projects writable storage state plus the immutable driver.
func validationStoragePool(storagePool *api.StoragePool) incusvalidate.StoragePool {
	writable := storagePool.Writable()
	return incusvalidate.StoragePool{
		Description: writable.Description,
		Driver:      storagePool.Driver,
		Config:      maps.Clone(writable.Config),
	}
}
