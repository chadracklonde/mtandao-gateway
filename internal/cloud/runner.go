package cloud

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"
)

// RunHeartbeat sends an immediate heartbeat and then retries with bounded
// exponential backoff whenever the Cloud cannot be reached.
func RunHeartbeat(ctx context.Context, client *Client, interval time.Duration, logger *log.Logger) {
	RunHeartbeatWithRouterStatus(ctx, client, interval, logger, func() string { return "unknown" })
}

func RunHeartbeatWithRouterStatus(ctx context.Context, client *Client, interval time.Duration, logger *log.Logger, routerStatus func() string) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if logger == nil {
		logger = log.Default()
	}
	backoff := 5 * time.Second
	const maxBackoff = 5 * time.Minute

	for {
		requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		status := "unknown"
		if routerStatus != nil {
			status = routerStatus()
		}
		result, err := client.HeartbeatWithRouterStatus(requestCtx, status)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			logger.Printf("Cloud heartbeat accepted: router=%s status=%s", result.RouterID, result.Status)
			backoff = 5 * time.Second
			if !wait(ctx, interval+jitter(interval/10)) {
				return
			}
			continue
		}
		if errors.Is(err, ErrUnauthorized) {
			logger.Printf("Cloud rejected Gateway credential; heartbeat paused until credential is replaced")
			if !wait(ctx, maxBackoff) {
				return
			}
			continue
		}
		logger.Printf("Cloud heartbeat failed: %v; retrying", fmt.Errorf("%w", err))
		delay := backoff + jitter(backoff/4)
		if !wait(ctx, delay) {
			return
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
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
