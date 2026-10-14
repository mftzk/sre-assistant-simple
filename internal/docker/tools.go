package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"sre-assistant-simple/internal/agent"
	"sre-assistant-simple/internal/authz"
)

type Executor interface {
	Run(context.Context, string) (string, error)
}
type DockerTools struct {
	Exec       Executor
	Auth       *authz.Authorizer
	Containers map[string]bool
}
type toolArgs struct {
	Container string `json:"container"`
	Tail      *int   `json:"tail,omitempty"`
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func (d *DockerTools) Execute(ctx context.Context, caller string, restartAllowed bool, call agent.ToolCall) string {
	result := func(output string, err error) string {
		v := map[string]any{"ok": err == nil, "output": output}
		if err != nil {
			v["error"] = err.Error()
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	if call.Function.Name != "docker_logs" && call.Function.Name != "docker_restart" {
		return result("", fmt.Errorf("tool tidak dikenal"))
	}
	if call.Function.Name == "docker_restart" && !d.Auth.CanRestart(caller, restartAllowed) {
		return result("", fmt.Errorf("restart hanya untuk SRE dan permintaan restart eksplisit pada pesan saat ini"))
	}
	var args toolArgs
	dec := json.NewDecoder(strings.NewReader(call.Function.Arguments))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return result("", fmt.Errorf("argumen tool tidak valid"))
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return result("", fmt.Errorf("argumen tool harus satu objek JSON"))
	}
	if !authz.ValidContainer(args.Container) || !d.Containers[args.Container] {
		return result("", fmt.Errorf("container tidak diizinkan"))
	}
	command := "docker restart --timeout 10 -- " + shellQuote(args.Container)
	if call.Function.Name == "docker_logs" {
		tail := 100
		if args.Tail != nil {
			tail = *args.Tail
		}
		if tail < 1 || tail > 200 {
			return result("", fmt.Errorf("tail harus 1 sampai 200"))
		}
		command = "docker logs --timestamps --tail " + strconv.Itoa(tail) + " -- " + shellQuote(args.Container)
	} else if args.Tail != nil {
		return result("", fmt.Errorf("restart tidak menerima tail"))
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := d.Exec.Run(ctx, command)
	if ctx.Err() != nil {
		err = fmt.Errorf("SSH timeout/canceled; hasil restart mungkin sudah terjadi, jangan ulang otomatis")
	}
	return result(output, err)
}

// AllowedContainers gives the model names it may use without discovery.
func (d *DockerTools) AllowedContainers() []string {
	names := make([]string, 0, len(d.Containers))
	for name := range d.Containers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (d *DockerTools) CanRestart(caller, text string) bool {
	return d.Auth.CanRestart(caller, authz.RestartRequested(text))
}
