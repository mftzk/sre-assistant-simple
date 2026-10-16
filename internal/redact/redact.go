// Package redact scrubs well-known credential shapes from tool output before
// it reaches the model or Slack. Patterns are shape-specific (known prefixes)
// to keep false positives near zero; this is defense in depth, not a reason to
// hand secrets to the model.
package redact

import "regexp"

var patterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// PEM private key blocks.
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`), `[REDACTED PRIVATE KEY]`},
	// URL with embedded password: keep scheme and user.
	{regexp.MustCompile(`((?:https?|git|ssh|ftp|postgres(?:ql)?|mysql|redis|amqp)://[^/\s:@]+):[^/\s@]+@`), `$1:[REDACTED]@`},
	// AWS access key IDs.
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), `[REDACTED]`},
	// Slack tokens.
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`), `[REDACTED]`},
	// GitHub tokens.
	{regexp.MustCompile(`\b(?:gh[pousr]|github_pat)_[A-Za-z0-9_]{20,}\b`), `[REDACTED]`},
	// OpenAI-style API keys.
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{20,}\b`), `[REDACTED]`},
	// JWTs.
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`), `[REDACTED]`},
	// Authorization: Bearer <token>.
	{regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._\-]{20,}`), `${1}[REDACTED]`},
	// KEY=value where the key name says it is a secret (env dumps, config files).
	{regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:PASSWORD|PASSWD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)[A-Z0-9_]*\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
}

// Secrets replaces recognizable credentials in s. Idempotent.
func Secrets(s string) string {
	for _, p := range patterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}
