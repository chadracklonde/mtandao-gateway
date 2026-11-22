package routeros

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrVoucherExists   = errors.New("voucher already exists")
	ErrVoucherNotFound = errors.New("voucher not found")
)

// HotspotExecutor is the narrow boundary for Hotspot requests. Implementations
// must refuse anything ValidateHotspotWords refuses.
type HotspotExecutor interface {
	ExecuteHotspot(ctx context.Context, words []string) ([]map[string]string, error)
}

type Profile struct {
	Name           string `json:"name"`
	RateLimit      string `json:"rate_limit,omitempty"`
	SessionTimeout string `json:"session_timeout,omitempty"`
	IdleTimeout    string `json:"idle_timeout,omitempty"`
	SharedUsers    string `json:"shared_users,omitempty"`
}

type ActiveSession struct {
	User            string `json:"user"`
	Address         string `json:"address,omitempty"`
	MACAddress      string `json:"mac_address,omitempty"`
	Uptime          string `json:"uptime,omitempty"`
	SessionTimeLeft string `json:"session_time_left,omitempty"`
	BytesIn         string `json:"bytes_in,omitempty"`
	BytesOut        string `json:"bytes_out,omitempty"`
}

// Hotspot manages Hotspot users (vouchers) and reads Hotspot state.
type Hotspot struct{ exec HotspotExecutor }

func NewHotspot(exec HotspotExecutor) (*Hotspot, error) {
	if exec == nil {
		return nil, ErrUnavailable
	}
	return &Hotspot{exec: exec}, nil
}

func (h *Hotspot) run(ctx context.Context, words ...string) ([]map[string]string, error) {
	if err := ValidateHotspotWords(words); err != nil {
		return nil, err
	}
	return h.exec.ExecuteHotspot(ctx, words)
}

func (h *Hotspot) Profiles(ctx context.Context) ([]Profile, error) {
	rows, err := h.run(ctx, "/ip/hotspot/user/profile/print",
		"=.proplist=name,rate-limit,session-timeout,idle-timeout,shared-users")
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, Profile{
			Name: row["name"], RateLimit: row["rate-limit"], SessionTimeout: row["session-timeout"],
			IdleTimeout: row["idle-timeout"], SharedUsers: row["shared-users"],
		})
	}
	return out, nil
}

func (h *Hotspot) ActiveSessions(ctx context.Context) ([]ActiveSession, error) {
	rows, err := h.run(ctx, "/ip/hotspot/active/print",
		"=.proplist=user,address,mac-address,uptime,session-time-left,bytes-in,bytes-out")
	if err != nil {
		return nil, err
	}
	out := make([]ActiveSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, ActiveSession{
			User: row["user"], Address: row["address"], MACAddress: row["mac-address"], Uptime: row["uptime"],
			SessionTimeLeft: row["session-time-left"], BytesIn: row["bytes-in"], BytesOut: row["bytes-out"],
		})
	}
	return out, nil
}

// CreateVoucher adds a Hotspot user whose name and password are the voucher code.
// It fails with ErrVoucherExists instead of overwriting an existing user.
func (h *Hotspot) CreateVoucher(ctx context.Context, code, profile string, duration time.Duration) error {
	existing, err := h.findUser(ctx, code)
	if err != nil {
		return err
	}
	if existing != "" {
		return ErrVoucherExists
	}
	words := []string{"/ip/hotspot/user/add", "=name=" + code, "=password=" + code}
	if profile != "" {
		words = append(words, "=profile="+profile)
	}
	if duration > 0 {
		words = append(words, "=limit-uptime="+formatUptime(duration))
	}
	_, err = h.run(ctx, words...)
	return err
}

// DisableVoucher disables the user and ends its active sessions. It returns the
// number of sessions ended. Disabling an already disabled voucher succeeds.
func (h *Hotspot) DisableVoucher(ctx context.Context, code string) (int, error) {
	id, err := h.findUser(ctx, code)
	if err != nil {
		return 0, err
	}
	if id == "" {
		return 0, ErrVoucherNotFound
	}
	if _, err := h.run(ctx, "/ip/hotspot/user/set", "=.id="+id, "=disabled=yes"); err != nil {
		return 0, err
	}
	sessions, err := h.run(ctx, "/ip/hotspot/active/print", "?user="+code, "=.proplist=.id,user")
	if err != nil {
		return 0, err
	}
	ended := 0
	for _, session := range sessions {
		sessionID := session[".id"]
		if sessionID == "" {
			continue
		}
		if _, err := h.run(ctx, "/ip/hotspot/active/remove", "=.id="+sessionID); err != nil {
			return ended, fmt.Errorf("end active session: %w", err)
		}
		ended++
	}
	return ended, nil
}

func (h *Hotspot) findUser(ctx context.Context, code string) (string, error) {
	rows, err := h.run(ctx, "/ip/hotspot/user/print", "?name="+code, "=.proplist=.id,name")
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if row["name"] == code {
			return row[".id"], nil
		}
	}
	return "", nil
}

// formatUptime renders a duration in RouterOS time notation (for example 1d2h30m).
func formatUptime(d time.Duration) string {
	minutes := int(d / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	days, hours, mins := minutes/1440, (minutes%1440)/60, minutes%60
	var b strings.Builder
	if days > 0 {
		fmt.Fprintf(&b, "%dd", days)
	}
	if hours > 0 {
		fmt.Fprintf(&b, "%dh", hours)
	}
	if mins > 0 {
		fmt.Fprintf(&b, "%dm", mins)
	}
	return b.String()
}
