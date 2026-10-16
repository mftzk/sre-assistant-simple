package docker

import (
	"context"
	"strings"
	"testing"

	"sre-assistant-simple/internal/authz"
)

func TestLogFunction(t *testing.T) {
	executor := &fakeExecutor{Output: "startup"}
	docker := &DockerTools{
		Exec:       executor,
		Containers: map[string]bool{"demo-app": true},
	}

	output, err := docker.Log(context.Background(), "demo-app", 25)
	if err != nil || output != "startup" {
		t.Fatal(output, err)
	}
	if len(executor.Commands) != 1 || executor.Commands[0] != "docker logs --timestamps --tail 25 -- 'demo-app'" {
		t.Fatal(executor.Commands)
	}

	for _, container := range []string{"other-app", "demo-app; whoami"} {
		if _, err := docker.Log(context.Background(), container, 25); err == nil {
			t.Fatal("invalid container accepted", container)
		}
	}
	if _, err := docker.Log(context.Background(), "demo-app", 201); err == nil {
		t.Fatal("oversized tail accepted")
	}
	if len(executor.Commands) != 1 {
		t.Fatal("invalid log request reached SSH")
	}
}

func TestRestartFunctionEnforcesAuthorization(t *testing.T) {
	executor := &fakeExecutor{Output: "demo-app"}
	docker := &DockerTools{
		Exec:       executor,
		Auth:       authz.New(map[string]bool{"USRE": true}),
		Containers: map[string]bool{"demo-app": true},
	}

	for _, request := range []struct {
		caller    string
		requested bool
		container string
	}{
		{"UDEV", true, "demo-app"},
		{"", true, "demo-app"},
		{"USRE", false, "demo-app"},
		{"USRE", true, "other-app"},
	} {
		_, err := docker.Restart(context.Background(), request.caller, request.requested, request.container)
		if err == nil {
			t.Fatal("unauthorized restart accepted", request)
		}
	}
	if len(executor.Commands) != 0 {
		t.Fatal("unauthorized restart reached SSH")
	}

	output, err := docker.Restart(context.Background(), "USRE", true, "demo-app")
	if err != nil || output != "demo-app" {
		t.Fatal(output, err)
	}
	if len(executor.Commands) != 1 || executor.Commands[0] != "docker restart --timeout 10 -- 'demo-app'" {
		t.Fatal(executor.Commands)
	}
}

func TestListShowsOnlyAllowlistedContainers(t *testing.T) {
	executor := &fakeExecutor{Output: "demo-app\trunning\tUp 5 minutes\tsre-demo:local\nsecret-db\trunning\tUp 2 days\tpostgres:16\n"}
	docker := &DockerTools{Exec: executor, Containers: map[string]bool{"demo-app": true, "demo-api": true}}

	output, err := docker.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "secret-db") || !strings.Contains(output, "demo-app\trunning") || !strings.Contains(output, "demo-api\tnot found on server") {
		t.Fatal(output)
	}
	if _, err := (&DockerTools{Exec: executor, Containers: map[string]bool{}}).List(context.Background()); err == nil {
		t.Fatal("empty allowlist accepted")
	}
}

func TestInspectNeverRequestsEnv(t *testing.T) {
	executor := &fakeExecutor{Output: `{"name":"/demo-api"}`}
	docker := &DockerTools{Exec: executor, Containers: map[string]bool{"demo-api": true}}

	if _, err := docker.Inspect(context.Background(), "demo-api"); err != nil {
		t.Fatal(err)
	}
	command := executor.Commands[0]
	if !strings.HasPrefix(command, "docker inspect --type container --format ") || !strings.HasSuffix(command, " -- 'demo-api'") || strings.Contains(command, "Env") {
		t.Fatal(command)
	}
	if _, err := docker.Inspect(context.Background(), "other-app"); err == nil || len(executor.Commands) != 1 {
		t.Fatal("container outside allowlist reached SSH")
	}
}
