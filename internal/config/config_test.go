package config

import "testing"

func TestConfigDefaultsAndValidation(t *testing.T) {
	for key, value := range map[string]string{
		"SLACK_BOT_TOKEN": "xoxb-demo", "SLACK_APP_TOKEN": "xapp-demo",
		"LLM_OPENAI_BASE_URL": "https://gateway.example/v1/", "LLM_OPENAI_API_KEY": "demo", "LLM_MODEL": "demo",
		"SSH_HOST": "demo.example", "SSH_USER": "demo", "SSH_PRIVATE_KEY": "dummy PEM",
		"SSH_PORT": "", "SSH_KNOWN_HOSTS_FILE": "", "SRE_SLACK_USER_IDS": "", "DOCKER_ALLOWED_CONTAINERS": "",
	} {
		t.Setenv(key, value)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://gateway.example" || c.Port != 22 || len(c.SREs) != 0 || len(c.Containers) != 0 || c.KnownHosts == "" {
		t.Fatal("incorrect defaults")
	}
	t.Setenv("SSH_PORT", "0")
	if _, err := Load(); err == nil {
		t.Fatal("invalid port accepted")
	}
	t.Setenv("SSH_PORT", "22")
	t.Setenv("DOCKER_ALLOWED_CONTAINERS", "demo;whoami")
	if _, err := Load(); err == nil {
		t.Fatal("invalid allowlist accepted")
	}
	t.Setenv("DOCKER_ALLOWED_CONTAINERS", "demo-app")
	t.Setenv("LLM_OPENAI_BASE_URL", "https://user:secret@gateway.example")
	if _, err := Load(); err == nil {
		t.Fatal("URL credentials accepted")
	}
}
