package docker

import (
	"context"
	"fmt"

	"sre-assistant-simple/internal/authz"
)

// Restart checks SRE authorization even when called without the dispatcher.
// caller and restartAllowed must come from the current Slack event, not the LLM.
func (d *DockerTools) Restart(ctx context.Context, caller string, restartAllowed bool, container string) (string, error) {
	if !d.Auth.CanRestart(caller, restartAllowed) {
		return "", fmt.Errorf("restart hanya untuk SRE dan permintaan restart eksplisit pada pesan saat ini")
	}
	if !authz.ValidContainer(container) || !d.Containers[container] {
		return "", fmt.Errorf("container tidak diizinkan")
	}

	command := "docker restart --timeout 10 -- " + shellQuote(container)
	return d.runCommand(ctx, command)
}
