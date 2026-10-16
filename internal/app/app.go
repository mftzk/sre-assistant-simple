package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sre-assistant-simple/internal/agent"
	"sre-assistant-simple/internal/authz"
	"sre-assistant-simple/internal/config"
	"sre-assistant-simple/internal/docker"
	"sre-assistant-simple/internal/slackbot"
)

// Run wires the pieces together:
//
//	config -> SSH executor -> Docker tools -> harness (LLM + tools) -> Slack bot
func Run() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	c, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Tools: the only way the agent can touch the server.
	executor, err := docker.NewSSHExecutor(c)
	if err != nil {
		log.Fatal(err)
	}
	tools := &docker.DockerTools{
		Exec:       executor,
		Auth:       authz.New(c.SREs),
		Containers: c.Containers,
	}

	// Model: any Chat Completions endpoint with tool calling.
	llm := &agent.LLM{
		BaseURL: c.BaseURL,
		Key:     c.APIKey,
		Model:   c.Model,
		Client:  &http.Client{Timeout: 60 * time.Second},
	}

	// Harness: the loop between model and tools.
	harness := &agent.Harness{LLM: llm, Docker: tools}

	// Interface: Slack mentions in, thread replies out.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("starting sre_users=%d allowed_containers=%d", len(c.SREs), len(c.Containers))
	if err := slackbot.Run(ctx, c, harness); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
