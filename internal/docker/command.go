package docker

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

const (
	maxCommandLength = 300
	// Remote-side limit, shorter than commandTimeout so the server kills the
	// command itself even when the SSH session is cut first.
	remoteTimeoutSeconds = 25
)

// allowedBinaries are read-only inspection commands. Anything that can write,
// spawn arbitrary processes or dump secrets wholesale (sh, bash, python, awk,
// sed, tee, curl, wget, rm, docker, env, sudo, ...) is deliberately missing.
var allowedBinaries = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true, "find": true,
	"stat": true, "wc": true, "df": true, "du": true, "free": true, "uptime": true,
	"ps": true, "hostname": true, "uname": true, "whoami": true, "id": true,
	"date": true, "journalctl": true, "systemctl": true, "ss": true, "ip": true,
	"dig": true, "lsblk": true, "vmstat": true,
}

// forbiddenChars enable redirection, pipes, chaining or command substitution.
const forbiddenChars = "><|;&$`(){}\\\n\r"

// Command runs one free-form, read-only shell command on the SSH host.
// Only SREs may use it: even read-only commands can read sensitive files.
// caller must be the real Slack user ID, never a model-supplied value.
func (d *DockerTools) Command(ctx context.Context, caller, command string) (string, error) {
	if !d.Auth.IsSRE(caller) {
		auditRejected(caller, command, "not an SRE")
		return "", fmt.Errorf("perintah SSH hanya untuk SRE")
	}
	if err := validateCommand(command); err != nil {
		auditRejected(caller, command, err.Error())
		return "", err
	}

	start := time.Now()
	remote := fmt.Sprintf("timeout -k 5 %d %s", remoteTimeoutSeconds, strings.TrimSpace(command))
	output, err := d.runCommand(ctx, remote)
	// Audit trail: who ran what, and whether it worked. Output is not logged.
	log.Printf("ssh_command user=%s ok=%t duration=%s command=%q", caller, err == nil, time.Since(start).Round(time.Millisecond), command)
	return output, err
}

// validateCommand applies the command policy: length cap, no shell
// metacharacters, an allowlisted binary and per-binary hardening.
func validateCommand(command string) error {
	command = strings.TrimSpace(command)
	switch {
	case command == "":
		return fmt.Errorf("command wajib diisi")
	case len(command) > maxCommandLength:
		return fmt.Errorf("command terlalu panjang (maksimum %d karakter)", maxCommandLength)
	case strings.ContainsAny(command, forbiddenChars):
		return fmt.Errorf("command berisi karakter shell terlarang (tanpa redirect, pipe, chaining, atau substitution)")
	}

	tokens := strings.Fields(command)
	binary := tokens[0]
	if strings.Contains(binary, "/") || !allowedBinaries[binary] {
		return fmt.Errorf("command %q tidak diizinkan; hanya perintah read-only: %s", binary, allowedList())
	}
	return hardenBinary(binary, tokens[1:])
}

// hardenBinary blocks the write-capable modes of otherwise read-only binaries.
func hardenBinary(binary string, args []string) error {
	firstArg := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			firstArg = arg
			break
		}
	}
	switch binary {
	case "find":
		for _, arg := range args {
			switch arg {
			case "-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls":
				return fmt.Errorf("find %s tidak diizinkan", arg)
			}
		}
	case "systemctl":
		switch firstArg {
		case "status", "show", "cat", "is-active", "is-enabled", "is-failed", "list-units", "list-unit-files", "list-timers":
		default:
			return fmt.Errorf("systemctl %q tidak diizinkan; hanya status/show/is-active/list-units", firstArg)
		}
	case "journalctl":
		for _, arg := range args {
			if strings.HasPrefix(arg, "--vacuum") || arg == "--rotate" || arg == "--flush" || arg == "--sync" || strings.HasPrefix(arg, "--relinquish") {
				return fmt.Errorf("journalctl %s tidak diizinkan", arg)
			}
		}
	case "ip":
		for _, arg := range args {
			switch arg {
			case "add", "del", "delete", "change", "replace", "set", "flush", "exec":
				return fmt.Errorf("ip %s tidak diizinkan", arg)
			}
		}
	case "date":
		for _, arg := range args {
			if arg == "-s" || strings.HasPrefix(arg, "--set") {
				return fmt.Errorf("date %s tidak diizinkan", arg)
			}
		}
	case "hostname":
		if firstArg != "" {
			return fmt.Errorf("hostname hanya boleh dibaca")
		}
	}
	return nil
}

func allowedList() string {
	names := make([]string, 0, len(allowedBinaries))
	for name := range allowedBinaries {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// auditRejected records denied attempts; %q keeps model input on one log line.
func auditRejected(caller, command, reason string) {
	if len(command) > maxCommandLength {
		command = command[:maxCommandLength] + "..."
	}
	log.Printf("ssh_command_rejected user=%s reason=%q command=%q", caller, reason, command)
}
