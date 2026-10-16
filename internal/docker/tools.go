package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"sre-assistant-simple/internal/agent"
	"sre-assistant-simple/internal/authz"
)

const (
	defaultTail    = 100
	maxTail        = 200
	commandTimeout = 30 * time.Second
)

// Executor runs one shell command on the server. Tests replace it with a fake.
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

// toolResult is what the model sees after a tool runs.
type toolResult struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Execute translates the LLM tool call into an ordinary Go function call.
func (d *DockerTools) Execute(ctx context.Context, caller string, restartAllowed bool, call agent.ToolCall) string {
	var output string
	var err error

	switch call.Function.Name {
	case "docker_list":
		output, err = d.executeList(ctx, call.Function.Arguments)
	case "docker_inspect":
		output, err = d.executeInspect(ctx, call.Function.Arguments)
	case "docker_logs":
		output, err = d.executeLog(ctx, call.Function.Arguments)
	case "docker_restart":
		output, err = d.executeRestart(ctx, caller, restartAllowed, call.Function.Arguments)
	default:
		err = fmt.Errorf("tool tidak dikenal")
	}

	result := toolResult{OK: err == nil, Output: output}
	if err != nil {
		result.Error = err.Error()
	}
	encoded, _ := json.Marshal(result)
	return string(encoded)
}

func (d *DockerTools) executeList(ctx context.Context, arguments string) (string, error) {
	args, err := parseArguments(arguments)
	if err != nil {
		return "", err
	}
	if args.Container != "" || args.Tail != nil {
		return "", fmt.Errorf("list tidak menerima argumen")
	}
	return d.List(ctx)
}

func (d *DockerTools) executeInspect(ctx context.Context, arguments string) (string, error) {
	args, err := parseArguments(arguments)
	if err != nil {
		return "", err
	}
	if args.Tail != nil {
		return "", fmt.Errorf("inspect tidak menerima tail")
	}
	return d.Inspect(ctx, args.Container)
}

func (d *DockerTools) executeLog(ctx context.Context, arguments string) (string, error) {
	args, err := parseArguments(arguments)
	if err != nil {
		return "", err
	}
	tail := defaultTail
	if args.Tail != nil {
		tail = *args.Tail
	}
	return d.Log(ctx, args.Container, tail)
}

func (d *DockerTools) executeRestart(ctx context.Context, caller string, restartAllowed bool, arguments string) (string, error) {
	// Reject unauthorized calls before reading model-supplied arguments.
	if !d.Auth.CanRestart(caller, restartAllowed) {
		return "", fmt.Errorf("restart hanya untuk SRE dan permintaan restart eksplisit pada pesan saat ini")
	}
	args, err := parseArguments(arguments)
	if err != nil {
		return "", err
	}
	if args.Tail != nil {
		return "", fmt.Errorf("restart tidak menerima tail")
	}
	return d.Restart(ctx, caller, restartAllowed, args.Container)
}

// parseArguments accepts exactly one JSON object with known fields only.
func parseArguments(arguments string) (toolArgs, error) {
	var args toolArgs
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&args); err != nil {
		return args, fmt.Errorf("argumen tool tidak valid")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return args, fmt.Errorf("argumen tool harus satu objek JSON")
	}
	return args, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// runCommand applies the same SSH deadline to both operations.
func (d *DockerTools) runCommand(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	output, err := d.Exec.Run(ctx, command)
	if ctx.Err() != nil {
		return output, fmt.Errorf("SSH timeout/canceled; hasil restart mungkin sudah terjadi, jangan ulang otomatis")
	}
	return output, err
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
