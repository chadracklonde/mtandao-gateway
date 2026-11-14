package protocol

import (
	"errors"
	"time"
)

var ErrInvalidCommand = errors.New("invalid gateway command")

type Command struct {
	CommandID string    `json:"command_id"`
	SiteID    string    `json:"site_id"`
	GatewayID string    `json:"gateway_id"`
	RouterID  string    `json:"router_id"`
	Operation string    `json:"operation"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Payload   map[string]any `json:"payload"`
}

var allowedOperations = map[string]struct{}{
	"health.check":                  {},
	"hotspot.voucher.create":        {},
	"hotspot.voucher.disable":       {},
	"hotspot.profile.list":          {},
	"hotspot.active-sessions.list":  {},
}

// Validate performs basic structural and expiry checks. Cryptographic signature
// verification and persistent replay protection are mandatory before execution.
func (c Command) Validate(now time.Time) error {
	if c.CommandID == "" || c.SiteID == "" || c.GatewayID == "" || c.RouterID == "" {
		return ErrInvalidCommand
	}
	if _, ok := allowedOperations[c.Operation]; !ok {
		return ErrInvalidCommand
	}
	if c.IssuedAt.IsZero() || c.ExpiresAt.IsZero() || !c.ExpiresAt.After(now) {
		return ErrInvalidCommand
	}
	if c.ExpiresAt.Before(c.IssuedAt) || c.Payload == nil {
		return ErrInvalidCommand
	}
	return nil
}
