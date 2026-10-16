package redact

import (
	"strings"
	"testing"
)

func TestSecrets(t *testing.T) {
	in := strings.Join([]string{
		"-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
		"https://user:pass123@git.example.com/repo.git",
		"AKIAABCDEFGHIJKLMNOP",
		"xoxb-1234567890-abcdef",
		"ghp_abcdefghijklmnopqrstuvwxyz0123",
		"sk-abcdefghijklmnopqrstuvwx",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
		"API_KEY=topsecret",
		"REDIS_PASSWORD: redispw",
	}, "\n")
	out := Secrets(in)
	for _, secret := range []string{"abc\n", "pass123", "AKIAABCDEFGHIJKLMNOP", "xoxb-1234567890", "ghp_abc", "sk-abc", "abcdefghijklmnopqrstuvwxyz", "topsecret", "redispw"} {
		if strings.Contains(out, secret) {
			t.Errorf("%q not redacted:\n%s", secret, out)
		}
	}
	if Secrets("GET /orders 200 15ms") != "GET /orders 200 15ms" || Secrets(out) != out {
		t.Fatal("over-redaction or not idempotent")
	}
}
