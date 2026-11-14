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

type EnrollmentResponse struct {
	RouterID     string `json:"routerId"`
	SiteID       string `json:"siteId"`
	GatewayToken string `json:"gatewayToken"`
}

// RedeemEnrollment exchanges a short-lived, one-time enrollment token for the
// permanent Gateway bearer credential. The returned secret is never logged.
func RedeemEnrollment(ctx context.Context, baseURL, enrollmentToken string, httpClient *http.Client) (EnrollmentResponse, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return EnrollmentResponse{}, errors.New("invalid Cloud API URL")
	}
	if parsed.Scheme != "https" {
		host := parsed.Hostname()
		if parsed.Scheme != "http" || (host != "localhost" && net.ParseIP(host) == nil) {
			return EnrollmentResponse{}, errors.New("Cloud API URL must use HTTPS (HTTP is allowed only for local testing)")
		}
	}
	enrollmentToken = strings.TrimSpace(enrollmentToken)
	if !validHexToken(enrollmentToken) {
		return EnrollmentResponse{}, errors.New("enrollment token must contain 64 lowercase hexadecimal characters")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	body, err := json.Marshal(struct {
		EnrollmentToken string `json:"enrollmentToken"`
	}{EnrollmentToken: enrollmentToken})
	if err != nil {
		return EnrollmentResponse{}, fmt.Errorf("encode enrollment request: %w", err)
	}
	endpoint := strings.TrimRight(parsed.String(), "/") + "/v1/gateway/enroll"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return EnrollmentResponse{}, fmt.Errorf("create enrollment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return EnrollmentResponse{}, fmt.Errorf("send enrollment request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return EnrollmentResponse{}, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EnrollmentResponse{}, fmt.Errorf("Cloud enrollment returned HTTP %d", resp.StatusCode)
	}

	var result EnrollmentResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return EnrollmentResponse{}, fmt.Errorf("decode enrollment response: %w", err)
	}
	if result.RouterID == "" || result.SiteID == "" || !validHexToken(result.GatewayToken) {
		return EnrollmentResponse{}, errors.New("Cloud returned an invalid enrollment response")
	}
	return result, nil
}

func validHexToken(token string) bool {
	if len(token) != 64 {
		return false
	}
	for _, r := range token {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
