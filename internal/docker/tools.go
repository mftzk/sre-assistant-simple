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

// Execute translates the LLM tool call into an ordinary Go function call.
func (d *DockerTools) Execute(ctx context.Context, caller string, restartAllowed bool, call agent.ToolCall) string {
	var output string
	var err error

	switch call.Function.Name {
	case "docker_logs":
		var args toolArgs
		args, err = parseArguments(call.Function.Arguments)
		if err == nil {
			tail := 100
			if args.Tail != nil {
				tail = *args.Tail
			}
			output, err = d.Log(ctx, args.Container, tail)
		}

	case "docker_restart":
		// Reject unauthorized calls before reading model-supplied arguments.
		if !d.Auth.CanRestart(caller, restartAllowed) {
			err = fmt.Errorf("restart hanya untuk SRE dan permintaan restart eksplisit pada pesan saat ini")
			break
		}

		var args toolArgs
		args, err = parseArguments(call.Function.Arguments)
		if err == nil {
			if args.Tail != nil {
				err = fmt.Errorf("restart tidak menerima tail")
			} else {
				output, err = d.Restart(ctx, caller, restartAllowed, args.Container)
			}
		}

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

type toolResult struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

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

// runCommand applies the same SSH deadline to both operations.
func (d *DockerTools) runCommand(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
