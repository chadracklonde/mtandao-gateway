// Package executor validates Cloud commands and runs them against a router
// exactly once, using the ledger to stay idempotent across retries and restarts.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chadracklonde/mtandao-gateway/internal/protocol"
	"github.com/chadracklonde/mtandao-gateway/internal/routeros"
)

// Stable error codes reported to the Cloud (in Outcome.Code).
const (
	CodeInvalidCommand    = "invalid_command"
	CodeExpired           = "expired"
	CodeWrongTarget       = "wrong_target"
	CodeInvalidPayload    = "invalid_payload"
	CodeRouterUnavailable = "router_unavailable"
	CodeVoucherExists     = "voucher_exists"
	CodeVoucherNotFound   = "voucher_not_found"
	CodeInterrupted       = "interrupted"
	CodeInternal          = "internal_error"
	CodeResultTooLarge    = "result_too_large"
	maxListRows           = 500
	maxListBytes          = 40 << 10
	maxDialAndRunDuration = 60 * time.Second
)

// Router is what the executor needs from a connected router.
type Router interface {
	Health(ctx context.Context) (map[string]any, error)
	Profiles(ctx context.Context) ([]routeros.Profile, error)
	ActiveSessions(ctx context.Context) ([]routeros.ActiveSession, error)
	CreateVoucher(ctx context.Context, code string, opts routeros.VoucherOptions) error
	DisableVoucher(ctx context.Context, code string) (int, error)
}

// Connect opens a router connection; the returned function closes it.
type Connect func(ctx context.Context) (Router, func(), error)

// Target optionally pins the site and router this Gateway serves. Empty fields are not checked.
type Target struct{ SiteID, RouterID string }

type Executor struct {
	ledger  *Ledger
	connect Connect
	target  Target
	now     func() time.Time
}

func New(ledger *Ledger, connect Connect, target Target) (*Executor, error) {
	if ledger == nil || connect == nil {
		return nil, errors.New("ledger and router connection are required")
	}
	return &Executor{ledger: ledger, connect: connect, target: target, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Process runs one ledger entry to completion and returns its outcome.
// A command that is already done is never executed again.
func (e *Executor) Process(ctx context.Context, entry Entry) (Outcome, error) {
	if entry.State == StateDone && entry.Outcome != nil {
		return *entry.Outcome, nil
	}
	cmd := entry.Command
	id := cmd.CommandID

	if entry.State == StateStarted && protocol.IsWriteOperation(cmd.Operation) {
		// The router may or may not have applied it. Never run a write twice.
		outcome := failure(CodeInterrupted, "the Gateway stopped while this command was running; check the router before retrying")
		return outcome, e.ledger.Complete(id, outcome)
	}

	if outcome, rejected := e.precheck(cmd, entry.State == StateReceived); rejected {
		return outcome, e.ledger.Complete(id, outcome)
	}

	if err := e.ledger.Begin(id); err != nil {
		return Outcome{}, fmt.Errorf("record command start: %w", err)
	}
	outcome := e.execute(ctx, cmd)
	if err := e.ledger.Complete(id, outcome); err != nil {
		return outcome, fmt.Errorf("record command outcome: %w", err)
	}
	return outcome, nil
}

// precheck rejects commands that must not reach the router. Expiry only applies
// to commands that have not started yet.
func (e *Executor) precheck(cmd protocol.Command, checkExpiry bool) (Outcome, bool) {
	if checkExpiry && !cmd.ExpiresAt.IsZero() && !cmd.ExpiresAt.After(e.now()) {
		return failure(CodeExpired, "command expired before it could run"), true
	}
	// Structural validation only: expiry was handled above, so evaluate it
	// just before the command's own deadline.
	if err := cmd.Validate(cmd.ExpiresAt.Add(-time.Nanosecond)); err != nil {
		return failure(CodeInvalidCommand, "command is malformed or not allowed"), true
	}
	if (e.target.SiteID != "" && cmd.SiteID != e.target.SiteID) || (e.target.RouterID != "" && cmd.RouterID != e.target.RouterID) {
		return failure(CodeWrongTarget, "command is addressed to another site or router"), true
	}
	var err error
	switch cmd.Operation {
	case "hotspot.voucher.create":
		_, err = protocol.ParseVoucherCreate(cmd.Payload)
	case "hotspot.voucher.disable":
		_, err = protocol.ParseVoucherDisable(cmd.Payload)
	default:
		err = protocol.RequireEmptyPayload(cmd.Payload)
	}
	if err != nil {
		return failure(CodeInvalidPayload, "payload does not match the operation"), true
	}
	return Outcome{}, false
}

func (e *Executor) execute(ctx context.Context, cmd protocol.Command) Outcome {
	ctx, cancel := context.WithTimeout(ctx, maxDialAndRunDuration)
	defer cancel()
	router, closeRouter, err := e.connect(ctx)
	if err != nil {
		return failure(CodeRouterUnavailable, "RouterOS is unavailable")
	}
	defer closeRouter()

	switch cmd.Operation {
	case "health.check":
		info, err := router.Health(ctx)
		if err != nil {
			return failure(CodeRouterUnavailable, "RouterOS is unavailable")
		}
		return success(info)
	case "hotspot.profile.list":
		profiles, err := router.Profiles(ctx)
		if err != nil {
			return failure(CodeRouterUnavailable, "RouterOS is unavailable")
		}
		rows, truncated := capRows(profiles)
		return success(map[string]any{"profiles": rows, "truncated": truncated})
	case "hotspot.active-sessions.list":
		sessions, err := router.ActiveSessions(ctx)
		if err != nil {
			return failure(CodeRouterUnavailable, "RouterOS is unavailable")
		}
		rows, truncated := capRows(sessions)
		return success(map[string]any{"sessions": rows, "count": len(sessions), "truncated": truncated})
	case "hotspot.voucher.create":
		payload, _ := protocol.ParseVoucherCreate(cmd.Payload)
		err := router.CreateVoucher(ctx, payload.Code, routeros.VoucherOptions{
			Profile:     payload.Profile,
			Duration:    time.Duration(payload.DurationMinutes) * time.Minute,
			DataLimitMB: payload.DataLimitMB,
			Comment:     payload.Comment,
		})
		return voucherOutcome(err, map[string]any{
			"code":                payload.Code,
			"created":             true,
			"usage_limit_minutes": payload.DurationMinutes, // 0: no time limit
			"data_limit_mb":       payload.DataLimitMB,     // 0: no data limit
		})
	case "hotspot.voucher.disable":
		code, _ := protocol.ParseVoucherDisable(cmd.Payload)
		ended, err := router.DisableVoucher(ctx, code)
		return voucherOutcome(err, map[string]any{"code": code, "disabled": true, "sessions_ended": ended})
	}
	return failure(CodeInvalidCommand, "command is malformed or not allowed")
}

func voucherOutcome(err error, result map[string]any) Outcome {
	switch {
	case err == nil:
		return success(result)
	case errors.Is(err, routeros.ErrVoucherExists):
		return failure(CodeVoucherExists, "a voucher with this code already exists")
	case errors.Is(err, routeros.ErrVoucherNotFound):
		return failure(CodeVoucherNotFound, "no voucher with this code")
	default:
		return failure(CodeRouterUnavailable, "RouterOS is unavailable")
	}
}

// capRows limits a list by row count and by encoded size, so the result always
// fits what the Cloud accepts. It reports whether rows were dropped.
func capRows[T any](rows []T) ([]T, bool) {
	truncated := false
	if len(rows) > maxListRows {
		rows, truncated = rows[:maxListRows], true
	}
	for len(rows) > 0 {
		encoded, err := json.Marshal(rows)
		if err == nil && len(encoded) <= maxListBytes {
			break
		}
		rows, truncated = rows[:len(rows)/2], true
	}
	return rows, truncated
}

func success(result map[string]any) Outcome { return Outcome{Success: true, Result: result} }

func failure(code, message string) Outcome { return Outcome{Code: code, Message: message} }

// CloudError renders a failed outcome as the stable "code: message" string sent to the Cloud.
func (o Outcome) CloudError() string {
	if o.Success {
		return ""
	}
	return o.Code + ": " + o.Message
}
