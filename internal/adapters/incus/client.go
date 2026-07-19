// Package incus adapts the Incus client used to manage runner instances.
package incus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	incusclient "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
)

var (
	errNotFound             = errors.New("incus resource not found")
	errInstanceFileNotFound = errors.New("incus instance file not found")
)

// ConnectUnix constructs a client for a project on a local Incus Unix socket.
func ConnectUnix(ctx context.Context, socketPath string, project string) (incusclient.InstanceServer, error) {
	client, err := incusclient.ConnectIncusUnixWithContext(ctx, socketPath, nil)
	if err != nil {
		return nil, err
	}

	return client.UseProject(project), nil
}

// client is the context-aware Incus surface required by Backend.
type client interface {
	GetImage(ctx context.Context, name string) error
	GetProfile(ctx context.Context, name string) error
	GetInstances(ctx context.Context) ([]api.Instance, error)
	GetInstance(ctx context.Context, name string) (*api.Instance, error)
	CreateInstance(ctx context.Context, request api.InstancesPost) error
	StartInstance(ctx context.Context, name string) error
	StopInstance(ctx context.Context, name string) error
	CreateInstanceFile(ctx context.Context, name string, path string, content []byte, mode int) error
	GetInstanceFile(ctx context.Context, name string, path string) ([]byte, error)
	GetInstanceConsoleLog(ctx context.Context, name string) ([]byte, error)
	DeleteInstance(ctx context.Context, name string) error
}

// serverClient adapts the Incus SDK to context-aware lifecycle calls.
type serverClient struct {
	server interface {
		WithContext(context.Context) incusclient.InstanceServer
	}
}

// newServerClient constructs a context-aware SDK adapter.
func newServerClient(server incusclient.InstanceServer) (*serverClient, error) {
	if server == nil {
		return nil, errors.New("incus server is required")
	}
	contextual, ok := server.(interface {
		WithContext(context.Context) incusclient.InstanceServer
	})
	if !ok {
		return nil, errors.New("incus server does not support request contexts")
	}

	return &serverClient{server: contextual}, nil
}

// contextual returns an SDK view bound to ctx.
func (c *serverClient) contextual(ctx context.Context) incusclient.InstanceServer {
	return c.server.WithContext(ctx)
}

// GetImage verifies that name resolves in the selected project.
func (c *serverClient) GetImage(ctx context.Context, name string) error {
	server := c.contextual(ctx)
	_, _, err := server.GetImage(name)
	classified := classifyError(err)
	if err == nil || !errors.Is(classified, errNotFound) {
		return classified
	}

	_, _, err = server.GetImageAlias(name)
	return classifyError(err)
}

// GetProfile verifies that name resolves in the selected project.
func (c *serverClient) GetProfile(ctx context.Context, name string) error {
	_, _, err := c.contextual(ctx).GetProfile(name)
	return classifyError(err)
}

// GetInstances returns virtual machines in the selected project.
func (c *serverClient) GetInstances(ctx context.Context) ([]api.Instance, error) {
	instances, err := c.contextual(ctx).GetInstances(api.InstanceTypeVM)
	return instances, classifyError(err)
}

// GetInstance returns one instance by name.
func (c *serverClient) GetInstance(ctx context.Context, name string) (*api.Instance, error) {
	instance, _, err := c.contextual(ctx).GetInstance(name)
	return instance, classifyError(err)
}

// CreateInstance creates one stopped virtual machine and waits for completion.
func (c *serverClient) CreateInstance(ctx context.Context, request api.InstancesPost) error {
	operation, err := c.contextual(ctx).CreateInstance(request)
	if err != nil {
		return classifyError(err)
	}

	return operation.WaitContext(ctx)
}

// StartInstance starts name and waits for completion.
func (c *serverClient) StartInstance(ctx context.Context, name string) error {
	operation, err := c.contextual(ctx).UpdateInstanceState(name, api.InstanceStatePut{Action: "start"}, "")
	if err != nil {
		return classifyError(err)
	}

	return operation.WaitContext(ctx)
}

// StopInstance forcibly stops name and waits for completion.
func (c *serverClient) StopInstance(ctx context.Context, name string) error {
	operation, err := c.contextual(ctx).UpdateInstanceState(
		name,
		api.InstanceStatePut{Action: "stop", Force: true},
		"",
	)
	if err != nil {
		return classifyError(err)
	}

	return operation.WaitContext(ctx)
}

// CreateInstanceFile writes a root-owned file through the Incus agent.
func (c *serverClient) CreateInstanceFile(
	ctx context.Context,
	name string,
	path string,
	content []byte,
	mode int,
) error {
	err := c.contextual(ctx).CreateInstanceFile(name, path, incusclient.InstanceFileArgs{
		Content: bytes.NewReader(content),
		UID:     0,
		GID:     0,
		Mode:    mode,
		Type:    "file",
	})
	return classifyError(err)
}

// GetInstanceFile reads a guest file through the Incus agent.
func (c *serverClient) GetInstanceFile(ctx context.Context, name string, path string) ([]byte, error) {
	server := c.contextual(ctx)
	content, _, err := server.GetInstanceFile(name, path)
	if err != nil {
		return nil, classifyInstanceFileError(err, func() error {
			_, _, lookupErr := server.GetInstance(name)
			return lookupErr
		})
	}
	defer content.Close()

	data, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("read guest file: %w", err)
	}

	return data, nil
}

// classifyInstanceFileError distinguishes an absent guest file from an absent instance.
func classifyInstanceFileError(err error, confirmInstance func() error) error {
	if !api.StatusErrorCheck(err, http.StatusNotFound) {
		return classifyError(err)
	}
	if confirmErr := confirmInstance(); confirmErr != nil {
		return fmt.Errorf("confirm instance after guest file lookup: %w", classifyError(confirmErr))
	}

	return errInstanceFileNotFound
}

// GetInstanceConsoleLog returns the buffered serial console log.
func (c *serverClient) GetInstanceConsoleLog(ctx context.Context, name string) ([]byte, error) {
	content, err := c.contextual(ctx).GetInstanceConsoleLog(name, nil)
	if err != nil {
		return nil, classifyError(err)
	}
	defer content.Close()

	data, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("read console log: %w", err)
	}

	return data, nil
}

// DeleteInstance deletes name and waits for completion.
func (c *serverClient) DeleteInstance(ctx context.Context, name string) error {
	operation, err := c.contextual(ctx).DeleteInstance(name)
	if err != nil {
		return classifyError(err)
	}

	return operation.WaitContext(ctx)
}

// classifyError gives the backend one stable not-found identity.
func classifyError(err error) error {
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return errNotFound
	}

	return err
}
