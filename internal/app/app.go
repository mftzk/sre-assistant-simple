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

func Run() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	c, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	executor, err := docker.NewSSHExecutor(c)
	if err != nil {
		log.Fatal(err)
	}
	h := &agent.Harness{LLM: &agent.LLM{BaseURL: c.BaseURL, Key: c.APIKey, Model: c.Model, Client: &http.Client{Timeout: 60 * time.Second}},
		Docker: &docker.DockerTools{Exec: executor, Auth: authz.New(c.SREs), Containers: c.Containers}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("starting sre_users=%d allowed_containers=%d", len(c.SREs), len(c.Containers))
	if err := slackbot.Run(ctx, c, h); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
