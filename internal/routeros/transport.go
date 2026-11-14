package routeros

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	ros "github.com/go-routeros/routeros/v3"
)

var ErrNonLocalTarget = errors.New("RouterOS target must be a private, loopback or link-local IP address")

type APIClient interface {
	RunContext(context.Context, ...string) (*ros.Reply, error)
	Close() error
}

type TLSConfig struct {
	Address  string
	Username string
	Password string
	Timeout  time.Duration
}

// DialTLS establishes an encrypted connection to a local RouterOS API endpoint.
// It deliberately accepts IP literals only: DNS rebinding and public destinations
// are excluded from this local management channel.
func DialTLS(ctx context.Context, cfg TLSConfig) (*Transport, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(cfg.Address))
	if err != nil { return nil, fmt.Errorf("RouterOS address must be host:port: %w", err) }
	ip := net.ParseIP(host)
	if ip == nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return nil, ErrNonLocalTarget
	}
	if port == "" { return nil, errors.New("RouterOS port is required") }
	portNumber, portErr := strconv.Atoi(port)
	if portErr != nil || portNumber < 1 || portNumber > 65535 { return nil, errors.New("invalid RouterOS port") }
	if cfg.Username == "" || cfg.Password == "" { return nil, errors.New("RouterOS credentials are required") }
	if cfg.Timeout <= 0 { cfg.Timeout = 8 * time.Second }

	dialCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
	conn, err := (&tls.Dialer{NetDialer: &net.Dialer{Timeout: cfg.Timeout}, Config: tlsConfig}).DialContext(dialCtx, "tcp", cfg.Address)
	if err != nil { return nil, fmt.Errorf("connect to local RouterOS API over TLS: %w", err) }
	if err := conn.SetDeadline(time.Now().Add(cfg.Timeout)); err != nil { _ = conn.Close(); return nil, err }
	client, err := ros.NewClient(conn)
	if err != nil { _ = conn.Close(); return nil, err }
	if err := client.LoginContext(dialCtx, cfg.Username, cfg.Password); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("RouterOS authentication failed: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})
	// Async mode is required by this library for RunContext cancellation to work.
	_ = client.Async()
	return &Transport{client: client, timeout: cfg.Timeout}, nil
}

type Transport struct {
	client APIClient
	timeout time.Duration
}

func NewTransport(client APIClient, timeout time.Duration) (*Transport, error) {
	if client == nil { return nil, errors.New("RouterOS API client is required") }
	if timeout <= 0 { timeout = 8 * time.Second }
	return &Transport{client: client, timeout: timeout}, nil
}

func (t *Transport) Close() error { return t.client.Close() }

// Execute accepts only the three fixed read-only print commands exposed by Client.
// Callers cannot use this transport to issue arbitrary RouterOS commands.
func (t *Transport) Execute(ctx context.Context, words []string) ([]map[string]string, error) {
	if len(words) != 2 { return nil, errors.New("unsupported RouterOS request") }
	allowed := map[string]bool{
		"/system/resource/print": true,
		"/system/identity/print": true,
		"/interface/print": true,
	}
	if !allowed[words[0]] || !validPropertyList(words[0], words[1]) {
		return nil, errors.New("unsupported RouterOS request")
	}
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	reply, err := t.client.RunContext(ctx, words...)
	if err != nil { return nil, err }
	if reply == nil { return nil, ErrUnexpectedResponse }
	rows := make([]map[string]string, 0, len(reply.Re))
	for _, sentence := range reply.Re {
		row := make(map[string]string, len(sentence.Map))
		for key, value := range sentence.Map { row[key] = value }
		rows = append(rows, row)
	}
	return rows, nil
}

func validPropertyList(command, argument string) bool {
	if !strings.HasPrefix(argument, "=.proplist=") { return false }
	allowed := map[string]map[string]bool{
		"/system/resource/print": {"uptime":true,"version":true,"board-name":true,"architecture-name":true,"cpu":true,"cpu-count":true,"total-memory":true,"free-memory":true},
		"/system/identity/print": {"name":true},
		"/interface/print": {"name":true,"type":true,"running":true,"disabled":true},
	}
	fields := strings.Split(strings.TrimPrefix(argument, "=.proplist="), ",")
	if len(fields)==0 || len(fields)>8 { return false }
	seen := map[string]bool{}
	for _, field := range fields {
		if !allowed[command][field] || seen[field] { return false }
		seen[field]=true
	}
	return true
}
