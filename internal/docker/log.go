package docker

import (
	"context"
	"fmt"

	"sre-assistant-simple/internal/authz"
)

// Log reads recent container logs. Reading logs does not require SRE access.
func (d *DockerTools) Log(ctx context.Context, container string, tail int) (string, error) {
	if !authz.ValidContainer(container) || !d.Containers[container] {
		return "", fmt.Errorf("container tidak diizinkan")
	}
	if tail < 1 || tail > maxTail {
		return "", fmt.Errorf("tail harus 1 sampai 200")
	}

	command := fmt.Sprintf("docker logs --timestamps --tail %d -- %s", tail, shellQuote(container))
	return d.runCommand(ctx, command)
}
