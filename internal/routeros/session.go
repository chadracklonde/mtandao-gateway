package routeros

import (
	"context"
)

// Session is one authenticated RouterOS connection exposing the read-only
// monitoring calls and the allow-listed Hotspot operations.
type Session struct {
	*Client
	*Hotspot
	transport *Transport
}

// OpenSession connects over TLS to the configured local router.
func OpenSession(ctx context.Context, cfg Config) (*Session, error) {
	transport, err := DialTLS(ctx, cfg.TLSConfig())
	if err != nil {
		return nil, err
	}
	client, err := NewClient(transport)
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	hotspot, err := NewHotspot(transport)
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	return &Session{Client: client, Hotspot: hotspot, transport: transport}, nil
}

func (s *Session) Close() error { return s.transport.Close() }

// Health summarizes the router state for health.check without secrets.
func (s *Session) Health(ctx context.Context) (map[string]any, error) {
	identity, err := s.Identity(ctx)
	if err != nil {
		return nil, err
	}
	resource, err := s.SystemResource(ctx)
	if err != nil {
		return nil, err
	}
	interfaces, err := s.Interfaces(ctx)
	if err != nil {
		return nil, err
	}
	running := 0
	for _, iface := range interfaces {
		if iface.Running && !iface.Disabled {
			running++
		}
	}
	return map[string]any{
		"identity":           identity.Name,
		"version":            resource.Version,
		"board":              resource.BoardName,
		"uptime":             resource.Uptime,
		"interfaces":         len(interfaces),
		"interfaces_running": running,
	}, nil
}
