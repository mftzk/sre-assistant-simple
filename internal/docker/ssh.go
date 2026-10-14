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

const outputLimit = 32 * 1024

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
	return &SSHExecutor{net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), &ssh.ClientConfig{User: c.User,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: verify, Timeout: 10 * time.Second}}, nil
}

func (s *SSHExecutor) Run(ctx context.Context, command string) (string, error) {
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return "", fmt.Errorf("SSH connection gagal")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	cc, chans, reqs, err := ssh.NewClientConn(conn, s.address, s.config)
	if err != nil {
		return "", fmt.Errorf("SSH handshake gagal (cek key dan known_hosts)")
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(cc, chans, reqs)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session gagal")
	}
	defer session.Close()
	output, err := session.CombinedOutput(command)

	// Keep the response sent to the LLM small. CombinedOutput buffers it first.
	truncated := len(output) > outputLimit
	if truncated {
		output = output[:outputLimit]
	}
	text := strings.ToValidUTF8(string(output), "?")
	if truncated {
		text += "\n[output truncated at 32 KiB]"
	}

	if err != nil {
		return text, fmt.Errorf("perintah Docker gagal; cek output")
	}
	return text, nil
}
