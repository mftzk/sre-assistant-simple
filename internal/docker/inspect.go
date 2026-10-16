package docker

import (
	"context"
	"fmt"

	"sre-assistant-simple/internal/authz"
)

// inspectTemplate picks only safe fields. Raw `docker inspect` includes Env,
// which often holds passwords and tokens, so it is never sent to the model.
const inspectTemplate = `{"name":{{json .Name}},"image":{{json .Config.Image}},"created":{{json .Created}},` +
	`"restart_count":{{.RestartCount}},"state":{{json .State}}}`

// Inspect explains why a container is in its current state: status, exit
// code, OOM kill, restart count and health. Reading state does not require SRE access.
func (d *DockerTools) Inspect(ctx context.Context, container string) (string, error) {
	if !authz.ValidContainer(container) || !d.Containers[container] {
		return "", fmt.Errorf("container tidak diizinkan")
	}

	command := "docker inspect --type container --format " + shellQuote(inspectTemplate) + " -- " + shellQuote(container)
	return d.runCommand(ctx, command)
}
