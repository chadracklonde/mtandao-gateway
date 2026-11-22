// Package commands polls the Cloud for Gateway commands, executes them through
// the executor and reports outcomes, surviving network failures and restarts.
package commands

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"time"

	"github.com/chadracklonde/mtandao-gateway/internal/cloud"
	"github.com/chadracklonde/mtandao-gateway/internal/executor"
	"github.com/chadracklonde/mtandao-gateway/internal/protocol"
)

// CloudAPI is the part of the Cloud client the runner uses.
type CloudAPI interface {
	ClaimCommands(ctx context.Context, limit int) ([]protocol.Command, error)
	ReportResult(ctx context.Context, commandID string, result cloud.CommandResult) error
}

const (
	// BatchSize is how many commands are claimed per poll.
	BatchSize      = 10
	initialBackoff = 5 * time.Second
	maxBackoff     = 5 * time.Minute
)

type Runner struct {
	api      CloudAPI
	exec     *executor.Executor
	ledger   *executor.Ledger
	interval time.Duration
	logger   *log.Logger
}

func NewRunner(api CloudAPI, exec *executor.Executor, ledger *executor.Ledger, interval time.Duration, logger *log.Logger) *Runner {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Runner{api: api, exec: exec, ledger: ledger, interval: interval, logger: logger}
}

// Run polls until ctx is cancelled. Failures back off exponentially; a rejected
// credential pauses polling like the heartbeat does.
func (r *Runner) Run(ctx context.Context) {
	backoff := initialBackoff
	for {
		claimed, err := r.Cycle(ctx)
		if ctx.Err() != nil {
			return
		}
		var delay time.Duration
		switch {
		case err == nil && claimed >= BatchSize:
			delay = 0 // more commands are probably waiting
			backoff = initialBackoff
		case err == nil:
			delay = r.interval + jitter(r.interval/10)
			backoff = initialBackoff
		case errors.Is(err, cloud.ErrUnauthorized):
			r.logger.Printf("Cloud rejected Gateway credential; command polling paused")
			delay = maxBackoff
		default:
			r.logger.Printf("command cycle failed: %v; retrying", err)
			delay = backoff + jitter(backoff/4)
			backoff = min(backoff*2, maxBackoff)
		}
		if !wait(ctx, delay) {
			return
		}
	}
}

// Cycle performs one poll: finish interrupted work, deliver pending results,
// claim new commands, execute and report them. It returns how many were claimed.
func (r *Runner) Cycle(ctx context.Context) (int, error) {
	if err := r.finishAndReport(ctx); err != nil {
		return 0, err
	}
	commands, err := r.api.ClaimCommands(ctx, BatchSize)
	if err != nil {
		return 0, err
	}
	// Persist every claimed command before running any: the Cloud delivers each
	// one only once, so the ledger must hold them before they can be lost.
	for _, cmd := range commands {
		if _, err := r.ledger.Receive(cmd); err != nil {
			return len(commands), err
		}
	}
	return len(commands), r.finishAndReport(ctx)
}

func (r *Runner) finishAndReport(ctx context.Context) error {
	for _, entry := range r.ledger.Unfinished() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := r.exec.Process(ctx, entry); err != nil {
			return err
		}
	}
	for _, entry := range r.ledger.Unreported() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		outcome := entry.Outcome
		err := r.api.ReportResult(ctx, entry.Command.CommandID, cloud.CommandResult{
			Success: outcome.Success,
			Result:  outcome.Result,
			Error:   outcome.CloudError(),
		})
		switch {
		case err == nil:
		case errors.Is(err, cloud.ErrResultRefused):
			r.logger.Printf("Cloud no longer accepts the result of command %s; dropping it", entry.Command.CommandID)
		default:
			return err
		}
		if err := r.ledger.MarkReported(entry.Command.CommandID); err != nil {
			return err
		}
	}
	return nil
}

func wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(max) + 1))
}
