package routeros

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

var (
	ErrUnavailable = errors.New("RouterOS transport is not configured")
	ErrUnexpectedResponse = errors.New("unexpected RouterOS response")
)

// Executor is the narrow boundary implemented by the RouterOS wire client.
// It intentionally exposes no arbitrary command endpoint to callers.
type Executor interface {
	Execute(ctx context.Context, words []string) ([]map[string]string, error)
}

type Client struct {
	executor Executor
}

func NewClient(executor Executor) (*Client, error) {
	if executor == nil {
		return nil, ErrUnavailable
	}
	return &Client{executor: executor}, nil
}

type Resource struct {
	Uptime        string
	Version       string
	BoardName     string
	Architecture  string
	CPU           string
	CPUCount      string
	TotalMemory   string
	FreeMemory    string
}

type Identity struct {
	Name string
}

type Interface struct {
	Name    string
	Type    string
	Running bool
	Disabled bool
}

func (c *Client) SystemResource(ctx context.Context) (Resource, error) {
	rows, err := c.executor.Execute(ctx, []string{"/system/resource/print", "=.proplist=uptime,version,board-name,architecture-name,cpu,cpu-count,total-memory,free-memory"})
	if err != nil { return Resource{}, err }
	if len(rows) != 1 { return Resource{}, ErrUnexpectedResponse }
	r := rows[0]
	return Resource{
		Uptime: r["uptime"], Version: r["version"], BoardName: r["board-name"],
		Architecture: r["architecture-name"], CPU: r["cpu"], CPUCount: r["cpu-count"],
		TotalMemory: r["total-memory"], FreeMemory: r["free-memory"],
	}, nil
}

func (c *Client) Identity(ctx context.Context) (Identity, error) {
	rows, err := c.executor.Execute(ctx, []string{"/system/identity/print", "=.proplist=name"})
	if err != nil { return Identity{}, err }
	if len(rows) != 1 { return Identity{}, ErrUnexpectedResponse }
	name := strings.TrimSpace(rows[0]["name"])
	if name == "" { return Identity{}, ErrUnexpectedResponse }
	return Identity{Name: name}, nil
}

func (c *Client) Interfaces(ctx context.Context) ([]Interface, error) {
	rows, err := c.executor.Execute(ctx, []string{"/interface/print", "=.proplist=name,type,running,disabled"})
	if err != nil { return nil, err }
	out := make([]Interface, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row["name"])
		if name == "" { return nil, ErrUnexpectedResponse }
		out = append(out, Interface{
			Name: name, Type: row["type"],
			Running: parseFlag(row["running"]), Disabled: parseFlag(row["disabled"]),
		})
	}
	return out, nil
}

func parseFlag(value string) bool {
	return value == "true" || value == "yes"
}

// ValidateLocalAddress rejects obvious non-local targets. The network adapter
// must additionally resolve and verify the destination before dialing.
func ValidateLocalAddress(host string) error {
	host = strings.TrimSpace(host)
	if host == "" { return fmt.Errorf("RouterOS host is required") }
	name, port, err := net.SplitHostPort(host)
	if err != nil || port == "" { return ErrNonLocalTarget }
	ip := net.ParseIP(name)
	if ip == nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) { return ErrNonLocalTarget }
	return nil
}
