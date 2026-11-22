package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/chadracklonde/mtandao-gateway/internal/protocol"
)

// ErrResultRefused means the Cloud will never accept this result (the command
// is unknown, already completed with another outcome, or no longer awaiting
// one). Retrying cannot help.
var ErrResultRefused = errors.New("Cloud refused the command result")

var commandIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	maxClaimResponseBytes = 4 << 20
	maxResultBytes        = 60 << 10 // the Cloud accepts at most 64 KB
)

// CommandResult is the body of POST /v1/gateway/commands/{id}/result.
type CommandResult struct {
	Success bool           `json:"success"`
	Result  map[string]any `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// ClaimCommands fetches up to limit pending commands for this Gateway's router.
// The Cloud delivers each command at most once.
func (c *Client) ClaimCommands(ctx context.Context, limit int) ([]protocol.Command, error) {
	if limit < 1 || limit > 50 {
		return nil, errors.New("claim limit must be between 1 and 50")
	}
	body, err := json.Marshal(map[string]int{"limit": limit})
	if err != nil {
		return nil, err
	}
	resp, err := c.postJSON(ctx, "/v1/gateway/commands/claim", body)
	if err != nil {
		return nil, fmt.Errorf("claim commands: %w", err)
	}
	defer resp.Body.Close()
	if err := statusError(resp.StatusCode); err != nil {
		return nil, err
	}
	var payload struct {
		Commands []protocol.Command `json:"commands"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxClaimResponseBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode claimed commands: %w", err)
	}
	return payload.Commands, nil
}

// ReportResult sends the outcome of a command. Reporting the same outcome twice
// is safe: the Cloud treats it as an idempotent retry.
func (c *Client) ReportResult(ctx context.Context, commandID string, result CommandResult) error {
	if !commandIDPattern.MatchString(commandID) {
		return errors.New("invalid command id")
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(body) > maxResultBytes {
		return errors.New("command result is too large")
	}
	resp, err := c.postJSON(ctx, "/v1/gateway/commands/"+url.PathEscape(commandID)+"/result", body)
	if err != nil {
		return fmt.Errorf("report command result: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusConflict, http.StatusBadRequest:
		return ErrResultRefused
	}
	return statusError(resp.StatusCode)
}

func (c *Client) postJSON(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return c.httpClient.Do(req)
}

func statusError(code int) error {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return ErrUnauthorized
	case code < 200 || code >= 300:
		return fmt.Errorf("Cloud returned HTTP %d", code)
	}
	return nil
}
