package docker

import (
	"context"
	"fmt"
	"strings"
)

// List shows the state of every allowlisted container. Reading state does not
// require SRE access. Containers outside the allowlist are never shown.
func (d *DockerTools) List(ctx context.Context) (string, error) {
	if len(d.Containers) == 0 {
		return "", fmt.Errorf("allowlist container kosong")
	}

	command := "docker ps --all --no-trunc --format " + shellQuote("{{.Names}}\t{{.State}}\t{{.Status}}\t{{.Image}}")
	output, err := d.runCommand(ctx, command)
	if err != nil {
		return "", err
	}

	// Filter in Go so the model only sees allowlisted names.
	lines := []string{"NAME\tSTATE\tSTATUS\tIMAGE"}
	found := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		name, _, _ := strings.Cut(line, "\t")
		if d.Containers[name] {
			lines = append(lines, line)
			found[name] = true
		}
	}
	for _, name := range d.AllowedContainers() {
		if !found[name] {
			lines = append(lines, name+"\tnot found on server")
		}
	}
	return strings.Join(lines, "\n"), nil
}
