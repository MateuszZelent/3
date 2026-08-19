package webui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/mumax/3/engine"
	"github.com/mumax/3/events"
	"github.com/mumax/3/log"
)

func init() {
	engine.DeclFunc("Tunnel", startTunnel, "Tunnel the web interface through SSH using the given host from your ssh config, empty string disables tunneling")
}

// SSHTunnel SSH Tunnel Configuration
type SSHTunnel struct {
	localIP    string // Worker WebUI address (localhost)
	localPort  uint16 // This will be dynamically assigned by the SSH server
	remoteIP   string // Worker WebUI address (localhost)
	remotePort uint16 // Worker WebUI port (e.g., 35369)
	SSHUser    string // SSH user on proxy server
	SSHHost    string // Proxy server address (e.g., proxy-server.com)
	SSHPort    string // SSH port on the proxy server (usually 22)

	mu       sync.Mutex
	sshConn  *ssh.Client
	listener net.Listener
}

// Load private keys from default locations like ~/.ssh/id_rsa
func loadPrivateKeys() []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		log.Log.Err("Could not resolve SSH home: %v", homeErr)
		return methods
	}

	// Try to load the private keys from the default file locations
	keyFiles := []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ed25519"),
	}

	for _, keyFile := range keyFiles {
		key, err := os.ReadFile(keyFile)
		if err != nil {
			continue // Skip if key file is not found or can't be read
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			log.Log.Err("Error parsing private key %s: %v", keyFile, err)
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	return methods
}

// Try to use SSH agent if available
func useSSHAgent() ssh.AuthMethod {
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			agentClient := agent.NewClient(conn)
			return ssh.PublicKeysCallback(agentClient.Signers)
		}
	}
	return nil
}

func fromConfig(host string, localPort, remotePort uint16) (tunnel SSHTunnel) {
	tunnel.localIP = "localhost"
	tunnel.localPort = localPort
	tunnel.remoteIP = "localhost"
	tunnel.remotePort = remotePort
	tunnel.SSHUser = ssh_config.Get(host, "User")
	tunnel.SSHHost = ssh_config.Get(host, "HostName")
	tunnel.SSHPort = ssh_config.Get(host, "Port")
	return
}

// Start the SSH reverse tunnel
// Start keeps the historical API while delegating lifecycle control to the
// context-aware implementation.
func (tunnel *SSHTunnel) Start(onReady func(string)) error {
	return tunnel.StartContext(context.Background(), onReady)
}

// StartContext starts the reverse tunnel and closes its SSH resources when ctx
// is cancelled or Close is called. A non-nil context is required by callers
// that own a longer-lived worker lifecycle.
func (tunnel *SSHTunnel) StartContext(ctx context.Context, onReady func(string)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	log.Log.Debug("Starting SSH tunnel")
	authMethods := []ssh.AuthMethod{}
	if agentAuth := useSSHAgent(); agentAuth != nil {
		authMethods = append(authMethods, agentAuth)
	}
	authMethods = append(authMethods, loadPrivateKeys()...)
	if len(authMethods) == 0 {
		return errors.New("no SSH keys or agent available")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve SSH home: %w", err)
	}
	hostKeyCallback, err := knownhosts.New(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		return fmt.Errorf("load SSH known_hosts: %w", err)
	}
	config := &ssh.ClientConfig{
		User:            tunnel.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         5 * time.Second,
	}
	sshPort := tunnel.SSHPort
	if sshPort == "" {
		sshPort = "22"
	}
	sshAddr := net.JoinHostPort(strings.Trim(tunnel.SSHHost, "[]"), sshPort)
	sshConn, err := ssh.Dial("tcp", sshAddr, config)
	if err != nil {
		return fmt.Errorf("failed to dial SSH: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = sshConn.Close()
		return err
	}
	listener, err := sshConn.Listen("tcp", net.JoinHostPort(tunnel.remoteIP, uint16ToString(tunnel.remotePort)))
	if err != nil {
		_ = sshConn.Close()
		return fmt.Errorf("failed to start reverse tunnel: %w", err)
	}
	tunnel.mu.Lock()
	tunnel.sshConn = sshConn
	tunnel.listener = listener
	tunnel.mu.Unlock()
	stopCloser := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = tunnel.Close()
		case <-stopCloser:
		}
	}()
	defer close(stopCloser)
	defer tunnel.Close()
	_, assignedPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return fmt.Errorf("parse reverse tunnel address: %w", err)
	}
	tunnel.remotePort, err = stringToUint16(assignedPort)
	if err != nil || tunnel.remotePort == 0 {
		return fmt.Errorf("invalid reverse tunnel port %q", assignedPort)
	}
	publicURL := "http://" + net.JoinHostPort(strings.Trim(tunnel.SSHHost, "[]"), uint16ToString(tunnel.remotePort)) + "/"
	log.Log.Info("Tunnel started: %s -> http://%s:%d", publicURL, tunnel.localIP, tunnel.localPort)
	if onReady != nil {
		onReady(publicURL)
	}
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return nil
			}
			return fmt.Errorf("accept reverse tunnel connection: %w", err)
		}
		localConn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(tunnel.localIP, uint16ToString(tunnel.localPort)))
		if err != nil {
			_ = clientConn.Close()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Log.Debug("Error connecting to local WebUI: %v", err)
			continue
		}
		go func(clientConn, localConn net.Conn) {
			defer clientConn.Close()
			defer localConn.Close()
			go func() { _, _ = io.Copy(localConn, clientConn) }()
			_, _ = io.Copy(clientConn, localConn)
		}(clientConn, localConn)
	}
}

// Close stops the reverse listener and SSH client. It is safe to call more
// than once and is also used by StartContext's cancellation watcher.
func (tunnel *SSHTunnel) Close() error {
	tunnel.mu.Lock()
	listener := tunnel.listener
	sshConn := tunnel.sshConn
	tunnel.listener = nil
	tunnel.sshConn = nil
	tunnel.mu.Unlock()
	var firstErr error
	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			firstErr = err
		}
	}
	if sshConn != nil {
		if err := sshConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func emitTunnelFailure(err error) {
	if err == nil {
		return
	}
	if emitErr := events.Emit(events.Event{Event: "tunnel_failed", Error: err.Error()}); emitErr != nil {
		log.Log.Err("tunnel failure event: %v", emitErr)
	}
	fmt.Printf("//tunnel-failed %s\n", err)
	log.Log.Err("SSH tunnel failed: %v", err)
}

func startTunnel(hostAndPort string) {
	go func() {
		localPort, err := getLocalPortWithRetry(5, 2*time.Second)
		if err != nil {
			emitTunnelFailure(fmt.Errorf("get local WebUI port: %w", err))
			return
		}

		remoteHost, remotePort, err := parseHostAndPort(hostAndPort)
		if err != nil {
			emitTunnelFailure(fmt.Errorf("parse SSH tunnel address: %w", err))
			return
		}
		tunnel := fromConfig(remoteHost, localPort, remotePort)
		if tunnel.SSHHost == "" {
			emitTunnelFailure(fmt.Errorf("no SSH host found in ~/.ssh/config for %s", remoteHost))
			return
		}
		if err := tunnel.Start(func(publicURL string) {
			if err := events.Emit(events.Event{Event: "tunnel_ready", PublicURL: publicURL}); err != nil {
				log.Log.Err("tunnel event: %v", err)
			}
			fmt.Printf("//tunnel-ready %s\n", publicURL)
		}); err != nil {
			emitTunnelFailure(err)
		}
	}()
}

// getLocalPortWithRetry attempts to retrieve and parse the local port from Metadata, retrying on failure.
func getLocalPortWithRetry(maxRetries int, retryInterval time.Duration) (uint16, error) {
	var localPort uint16
	var err error

	for range maxRetries {
		port := engine.WebMetadata.Snapshot()["port"]
		switch value := port.(type) {
		case int:
			if value > 0 && value <= 65535 {
				return uint16(value), nil
			}
		case string:
			localPort, err = stringToUint16(value)
			if err == nil && localPort > 0 {
				return localPort, nil // Successfully retrieved the port
			}
		}
		log.Log.Debug("Failed to get or parse port, retrying in %v...", retryInterval)
		time.Sleep(retryInterval)
	}

	return 0, fmt.Errorf("could not retrieve local port after %d retries", maxRetries)
}

func parseHostAndPort(hostAndPort string) (host string, port uint16, err error) {
	raw := strings.TrimSpace(hostAndPort)
	if raw == "" {
		return "", 0, errors.New("empty SSH tunnel host")
	}
	if parsedHost, parsedPort, splitErr := net.SplitHostPort(raw); splitErr == nil {
		parsed, parseErr := stringToUint16(parsedPort)
		return parsedHost, parsed, parseErr
	}
	if strings.Count(raw, ":") == 1 {
		parts := strings.SplitN(raw, ":", 2)
		parsed, parseErr := stringToUint16(parts[1])
		return parts[0], parsed, parseErr
	}
	if strings.HasPrefix(raw, "[") {
		return "", 0, fmt.Errorf("invalid SSH tunnel address %q", raw)
	}
	return raw, 0, nil
}
func stringToUint16(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(n), nil
}

func uint16ToString(n uint16) string {
	return strconv.FormatUint(uint64(n), 10)
}
