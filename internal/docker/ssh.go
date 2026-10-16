package docker

import (
	"context"
	"fmt"
	"net"
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
	signer, err := ssh.ParsePrivateKey([]byte(c.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("SSH_PRIVATE_KEY: gagal parse PEM (gunakan key tanpa passphrase atau agent khusus demo)")
	}
	verify, err := knownhosts.New(c.KnownHosts)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	return &SSHExecutor{
		address: net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		config: &ssh.ClientConfig{
			User:            c.User,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: verify, // host key must match known_hosts
			Timeout:         handshakeTimeout,
		},
	}, nil
}

// Run opens a fresh SSH connection, runs one command and returns its output.
func (s *SSHExecutor) Run(ctx context.Context, command string) (string, error) {
	conn, err := (&net.Dialer{Timeout: handshakeTimeout}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return "", fmt.Errorf("SSH connection gagal")
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
		return nil, fmt.Errorf("SSH handshake gagal (cek key dan known_hosts)")
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(cc, chans, reqs), nil
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
