package docker

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"sre-assistant-simple/internal/config"
)

func testKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// A real loopback SSH server tests the transport without Docker or external hosts.
func localSSH(t *testing.T, output string, stall bool) (config.Config, <-chan string) {
	t.Helper()
	hostSigner, _ := testKey(t)
	clientSigner, clientPEM := testKey(t)
	serverConfig := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() != "demo" || string(key.Marshal()) != string(clientSigner.PublicKey().Marshal()) {
			return nil, io.EOF
		}
		return nil, nil
	}}
	serverConfig.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	command := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		server, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(reqs)
		for ch := range chans {
			if ch.ChannelType() != "session" {
				_ = ch.Reject(ssh.UnknownChannelType, "session only")
				continue
			}
			channel, requests, err := ch.Accept()
			if err != nil {
				return
			}
			for req := range requests {
				if req.Type != "exec" {
					_ = req.Reply(false, nil)
					continue
				}
				var payload struct{ Command string }
				if ssh.Unmarshal(req.Payload, &payload) != nil {
					return
				}
				command <- payload.Command
				_ = req.Reply(true, nil)
				if stall {
					_, _ = io.Copy(io.Discard, channel)
				} else {
					_, _ = channel.Write([]byte(output))
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				}
				_ = channel.Close()
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			t.Error("SSH test server did not terminate")
		}
	})
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portInt, _ := strconv.Atoi(port)
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{listener.Addr().String()}, hostSigner.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return config.Config{Host: host, Port: portInt, User: "demo", PrivateKey: clientPEM, KnownHosts: path}, command
}
func TestSSHAuthenticatedCommandAndOutputBound(t *testing.T) {
	c, command := localSSH(t, strings.Repeat("x", outputLimit+100), false)
	exec, err := NewSSHExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.Run(ctx, "docker logs --timestamps --tail 100 -- 'demo-app'")
	if err != nil || output != strings.Repeat("x", outputLimit)+"\n[output truncated at 32 KiB]" {
		t.Fatal(err, len(output))
	}
	if got := <-command; got != "docker logs --timestamps --tail 100 -- 'demo-app'" {
		t.Fatal(got)
	}
}
func TestSSHRejectsUnknownHost(t *testing.T) {
	c, command := localSSH(t, "should not run", false)
	if err := os.WriteFile(c.KnownHosts, nil, 0600); err != nil {
		t.Fatal(err)
	}
	exec, err := NewSSHExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := exec.Run(ctx, "docker restart --timeout 10 -- 'demo-app'"); err == nil {
		t.Fatal("untrusted host accepted")
	}
	select {
	case cmd := <-command:
		t.Fatal("command executed on untrusted host", cmd)
	default:
	}
}
func TestSSHTimeoutClosesConnection(t *testing.T) {
	c, _ := localSSH(t, "", true)
	exec, err := NewSSHExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := exec.Run(ctx, "docker restart --timeout 10 -- 'demo-app'"); err == nil {
		t.Fatal("stall succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout did not stop transport")
	}
}

func TestSSHSkipHostKeyAcceptsUnknownHost(t *testing.T) {
	c, _ := localSSH(t, "ok", false)
	c.KnownHosts = filepath.Join(t.TempDir(), "missing")
	c.SkipHostKey = true
	exec, err := NewSSHExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := exec.Run(ctx, "docker logs --timestamps --tail 100 -- 'demo-app'"); err != nil {
		t.Fatal(err)
	}
}
