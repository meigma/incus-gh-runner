// Package incus adapts the Incus client used to manage runner instances.
package incus

import (
	"context"

	incusclient "github.com/lxc/incus/v7/client"
)

// ConnectUnix constructs a client for a project on a local Incus Unix socket.
func ConnectUnix(ctx context.Context, socketPath string, project string) (incusclient.InstanceServer, error) {
	client, err := incusclient.ConnectIncusUnixWithContext(ctx, socketPath, nil)
	if err != nil {
		return nil, err
	}

	return client.UseProject(project), nil
}
