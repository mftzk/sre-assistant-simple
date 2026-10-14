package docker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sre-assistant-simple/internal/agent"
	"sre-assistant-simple/internal/authz"
)

type fakeExecutor struct {
	Commands []string
	Output   string
	Err      error
}

func (f *fakeExecutor) Run(_ context.Context, command string) (string, error) {
	f.Commands = append(f.Commands, command)
	return f.Output, f.Err
}
func toolCall(name, args string) agent.ToolCall {
	return agent.ToolCall{ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: name, Arguments: args}}
}
func toolOK(t *testing.T, result string) bool {
	t.Helper()
	var data struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatal(err)
	}
	return data.OK
}
func TestDockerPolicyAndCommands(t *testing.T) {
	for _, tc := range []struct {
		name, caller, tool, args string
		requested, wantOK        bool
		command                  string
	}{
		{"logs-public", "UDEV", "docker_logs", `{"container":"demo-app"}`, false, true, "docker logs --timestamps --tail 100 -- 'demo-app'"},
		{"logs-tail", "UDEV", "docker_logs", `{"container":"demo-app","tail":12}`, false, true, "docker logs --timestamps --tail 12 -- 'demo-app'"},
		{"restart-sre", "USRE", "docker_restart", `{"container":"demo-app"}`, true, true, "docker restart --timeout 10 -- 'demo-app'"},
		{"restart-nonsre", "UDEV", "docker_restart", `{"container":"demo-app","user_id":"USRE"}`, true, false, ""},
		{"restart-empty-id", "", "docker_restart", `{"container":"demo-app"}`, true, false, ""},
		{"restart-no-request", "USRE", "docker_restart", `{"container":"demo-app"}`, false, false, ""},
		{"other-container", "USRE", "docker_restart", `{"container":"prod"}`, true, false, ""},
		{"injection", "USRE", "docker_logs", `{"container":"demo-app; touch /tmp/pwn"}`, false, false, ""},
		{"option-injection", "USRE", "docker_logs", `{"container":"--help"}`, false, false, ""},
		{"tail-zero", "UDEV", "docker_logs", `{"container":"demo-app","tail":0}`, false, false, ""},
		{"tail-large", "UDEV", "docker_logs", `{"container":"demo-app","tail":201}`, false, false, ""},
		{"tail-fraction", "UDEV", "docker_logs", `{"container":"demo-app","tail":1.5}`, false, false, ""},
		{"unknown-arg", "USRE", "docker_logs", `{"container":"demo-app","command":"rm"}`, false, false, ""},
		{"invalid-json", "USRE", "docker_logs", `{`, false, false, ""},
		{"extra-json", "USRE", "docker_logs", `{"container":"demo-app"} {}`, false, false, ""},
		{"unknown-tool", "USRE", "shell", `{"container":"demo-app"}`, true, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeExecutor{Output: "hello"}
			d := &DockerTools{f, authz.New(map[string]bool{"USRE": true}), map[string]bool{"demo-app": true}}
			if got := toolOK(t, d.Execute(context.Background(), tc.caller, tc.requested, toolCall(tc.tool, tc.args))); got != tc.wantOK {
				t.Fatalf("ok=%t want %t", got, tc.wantOK)
			}
			if tc.command == "" {
				if len(f.Commands) != 0 {
					t.Fatal("denied tool reached SSH")
				}
			} else if len(f.Commands) != 1 || f.Commands[0] != tc.command {
				t.Fatalf("commands=%v", f.Commands)
			}
		})
	}
}
func TestEmptyAllowlistAndExecutionFailure(t *testing.T) {
	f := &fakeExecutor{Err: errors.New("offline")}
	d := &DockerTools{Exec: f, Auth: authz.New(nil), Containers: map[string]bool{}}
	if toolOK(t, d.Execute(context.Background(), "U", false, toolCall("docker_logs", `{"container":"demo-app"}`))) || len(f.Commands) > 0 {
		t.Fatal("empty allowlist did not deny")
	}
	d.Containers = map[string]bool{"demo-app": true}
	if toolOK(t, d.Execute(context.Background(), "U", false, toolCall("docker_logs", `{"container":"demo-app"}`))) {
		t.Fatal("failed execution marked successful")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := d.Execute(ctx, "U", false, toolCall("docker_logs", `{"container":"demo-app"}`))
	if !strings.Contains(result, "jangan ulang otomatis") {
		t.Fatal(result)
	}
}
