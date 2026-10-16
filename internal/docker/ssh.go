package docker

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"sre-assistant-simple/internal/config"
)

const (
	outputLimit      = 32 * 1024
	handshakeTimeout = 10 * time.Second
)

// SSHExecutor runs commands on the one server from config. Host, user and key
// come from the environment; the model can never choose them.
type SSHExecutor struct {
	address string
	config  *ssh.ClientConfig
}

func NewSSHExecutor(c config.Config) (*SSHExecutor, error) {
	signer, err := ssh.ParsePrivateKey(normalizePEM(c.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("SSH_PRIVATE_KEY: gagal parse PEM (gunakan key tanpa passphrase atau agent khusus demo)")
	}
	verify, err := hostKeyCallback(c)
	if err != nil {
		return nil, err
	}
	return &SSHExecutor{
		address: net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		config: &ssh.ClientConfig{
			User:            c.User,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: verify,
			Timeout:         handshakeTimeout,
		},
	}, nil
}

// normalizePEM repairs keys damaged by web forms that store env values on one
// line: literal "\n", spaces or no separators instead of newlines. A
// base64-encoded PEM is accepted too.
func normalizePEM(value string) []byte {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, `\n`, "\n"), "\r", ""))
	if !strings.Contains(value, "-----BEGIN ") {
		if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && strings.Contains(string(decoded), "-----BEGIN ") {
			value = strings.TrimSpace(string(decoded))
		}
	}
	match := pemBlock.FindStringSubmatch(value)
	if match == nil {
		return []byte(value)
	}
	body := strings.Join(strings.Fields(match[2]), "")
	var b strings.Builder
	b.WriteString("-----BEGIN " + match[1] + "-----\n")
	for len(body) > 70 {
		b.WriteString(body[:70] + "\n")
		body = body[70:]
	}
	b.WriteString(body + "\n-----END " + match[1] + "-----\n")
	return []byte(b.String())
}

var pemBlock = regexp.MustCompile(`(?s)-----BEGIN ([A-Z0-9 ]+)-----(.*?)-----END ([A-Z0-9 ]+)-----`)

// hostKeyCallback verifies the server against known_hosts, unless the demo
// explicitly opts out with SSH_INSECURE_SKIP_HOST_KEY=true.
func hostKeyCallback(c config.Config) (ssh.HostKeyCallback, error) {
	if c.SkipHostKey {
		log.Print("WARNING: SSH host key verification disabled (SSH_INSECURE_SKIP_HOST_KEY=true)")
		return ssh.InsecureIgnoreHostKey(), nil
	}
	verify, err := knownhosts.New(c.KnownHosts)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	return verify, nil
}

// Run opens a fresh SSH connection, runs one command and returns its output.
func (s *SSHExecutor) Run(ctx context.Context, command string) (string, error) {
	conn, err := (&net.Dialer{Timeout: handshakeTimeout}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		// The address stays in the operator log, not in the result sent to the model.
		log.Printf("ssh_connect_failed: %v", err)
		return "", fmt.Errorf("SSH connection gagal (host tidak terjangkau atau port ditutup)")
	}
	defer conn.Close()
	// Closing the TCP connection is how a canceled context stops a running command.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	client, err := s.handshake(ctx, conn)
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session gagal")
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	text := limitOutput(output)
	if err != nil {
		return text, fmt.Errorf("perintah Docker gagal; cek output")
	}
	return text, nil
}

// handshake authenticates within handshakeTimeout or the context deadline, whichever is sooner.
func (s *SSHExecutor) handshake(ctx context.Context, conn net.Conn) (*ssh.Client, error) {
	deadline := time.Now().Add(handshakeTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	cc, chans, reqs, err := ssh.NewClientConn(conn, s.address, s.config)
	if err != nil {
		// x/crypto errors name the failed step, e.g. "unable to authenticate,
		// attempted methods [none publickey]" or "knownhosts: key is unknown".
		log.Printf("ssh_handshake_failed: %v", err)
		return nil, fmt.Errorf("SSH handshake gagal: %s", handshakeReason(err))
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(cc, chans, reqs), nil
}

// handshakeReason turns a handshake error into a short hint without addresses.
func handshakeReason(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "unable to authenticate"):
		return "key ditolak server (cek SSH_USER dan authorized_keys)"
	case strings.Contains(message, "knownhosts"):
		return "host key tidak cocok dengan known_hosts"
	case strings.Contains(message, "i/o timeout"), strings.Contains(message, "deadline exceeded"):
		return "timeout saat handshake"
	default:
		return "lihat log bot (ssh_handshake_failed)"
	}
}

// limitOutput keeps the text sent to the LLM small. CombinedOutput buffers
// the whole output first, so this bounds the result, not memory use.
func limitOutput(output []byte) string {
	truncated := len(output) > outputLimit
	if truncated {
		output = output[:outputLimit]
	}
	text := strings.ToValidUTF8(string(output), "?")
	if truncated {
		text += "\n[output truncated at 32 KiB]"
	}
	return text
}
