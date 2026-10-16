package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sre-assistant-simple/internal/authz"
)

type Config struct {
	BotToken, AppToken, BaseURL, APIKey, Model string
	Host, User, PrivateKey, KnownHosts         string
	Port                                       int
	SkipHostKey                                bool
	SREs, Containers                           map[string]bool
}

func csvSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result[item] = true
		}
	}
	return result
}

// Load reads every setting from the environment. Nothing here comes from the
// model: host, key, SRE list and container allowlist are fixed at startup.
func Load() (Config, error) {
	c := Config{
		BotToken: os.Getenv("SLACK_BOT_TOKEN"),
		AppToken: os.Getenv("SLACK_APP_TOKEN"),

		// Accept the base URL with or without a trailing /v1.
		BaseURL: strings.TrimSuffix(strings.TrimRight(os.Getenv("LLM_OPENAI_BASE_URL"), "/"), "/v1"),
		APIKey:  os.Getenv("LLM_OPENAI_API_KEY"),
		Model:   os.Getenv("LLM_MODEL"),

		Host:       os.Getenv("SSH_HOST"),
		Port:       22,
		User:       os.Getenv("SSH_USER"),
		PrivateKey: os.Getenv("SSH_PRIVATE_KEY"),
		KnownHosts: os.Getenv("SSH_KNOWN_HOSTS_FILE"),
		// Demo-only escape hatch: accept any server host key (no MITM protection).
		SkipHostKey: os.Getenv("SSH_INSECURE_SKIP_HOST_KEY") == "true",

		SREs:       csvSet(os.Getenv("SRE_SLACK_USER_IDS")),
		Containers: csvSet(os.Getenv("DOCKER_ALLOWED_CONTAINERS")),
	}
	required := map[string]string{
		"SLACK_BOT_TOKEN":     c.BotToken,
		"SLACK_APP_TOKEN":     c.AppToken,
		"LLM_OPENAI_BASE_URL": c.BaseURL,
		"LLM_OPENAI_API_KEY":  c.APIKey,
		"LLM_MODEL":           c.Model,
		"SSH_HOST":            c.Host,
		"SSH_USER":            c.User,
		"SSH_PRIVATE_KEY":     c.PrivateKey,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return c, fmt.Errorf("%s wajib diisi", name)
		}
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return c, fmt.Errorf("LLM_OPENAI_BASE_URL harus berupa URL HTTP(S) tanpa credential/query/fragment")
	}
	if value := os.Getenv("SSH_PORT"); value != "" {
		c.Port, err = strconv.Atoi(value)
		if err != nil || c.Port < 1 || c.Port > 65535 {
			return c, fmt.Errorf("SSH_PORT tidak valid")
		}
	}
	if c.KnownHosts == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return c, err
		}
		c.KnownHosts = filepath.Join(home, ".ssh", "known_hosts")
	}
	for name := range c.Containers {
		if !authz.ValidContainer(name) {
			return c, fmt.Errorf("nama container allowlist tidak valid")
		}
	}
	return c, nil
}
