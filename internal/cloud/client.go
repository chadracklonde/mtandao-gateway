package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("gateway credential rejected")

type HeartbeatResponse struct {
	RouterID   string    `json:"routerId"`
	SiteID     string    `json:"siteId"`
	Status     string    `json:"status"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	RouterConnectionStatus string `json:"routerConnectionStatus"`
	RouterLastSeenAt *time.Time `json:"routerLastSeenAt"`
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid Cloud API URL")
	}
	if parsed.Scheme != "https" {
		host := parsed.Hostname()
		if parsed.Scheme != "http" || (host != "localhost" && net.ParseIP(host) == nil) {
			return nil, errors.New("Cloud API URL must use HTTPS (HTTP is allowed only for local testing)")
		}
	}
	token = strings.TrimSpace(token)
	if len(token) != 64 {
		return nil, errors.New("Gateway token must contain 64 hexadecimal characters")
	}
	for _, r := range token {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return nil, errors.New("Gateway token must be lowercase hexadecimal")
		}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		token: token,
		httpClient: httpClient,
	}, nil
}

func (c *Client) Heartbeat(ctx context.Context) (HeartbeatResponse, error) {
	return c.HeartbeatWithRouterStatus(ctx, "unknown")
}

func (c *Client) HeartbeatWithRouterStatus(ctx context.Context, routerStatus string) (HeartbeatResponse, error) {
	if routerStatus != "unknown" && routerStatus != "online" && routerStatus != "unavailable" { return HeartbeatResponse{}, errors.New("invalid RouterOS connection status") }
	body, _ := json.Marshal(map[string]string{"routerConnectionStatus":routerStatus})
	endpoint := c.baseURL + "/v1/gateway/heartbeat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return HeartbeatResponse{}, fmt.Errorf("create heartbeat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return HeartbeatResponse{}, fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return HeartbeatResponse{}, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return HeartbeatResponse{}, fmt.Errorf("Cloud heartbeat returned HTTP %d", resp.StatusCode)
	}
	var result HeartbeatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return HeartbeatResponse{}, fmt.Errorf("decode heartbeat response: %w", err)
	}
	if result.Status != "online" || result.RouterID == "" || result.SiteID == "" || result.RouterConnectionStatus != routerStatus {
		return HeartbeatResponse{}, errors.New("Cloud returned an invalid heartbeat response")
	}
	return result, nil
}
