package cloud

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestHeartbeatSendsBearerAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/gateway/heartbeat" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Fatalf("unexpected authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"routerId":"router-1","siteId":"site-1","status":"online","lastSeenAt":"2026-11-14T10:00:00Z","routerConnectionStatus":"unknown","routerLastSeenAt":null}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testToken, server.Client())
	if err != nil { t.Fatal(err) }
	got, err := client.Heartbeat(context.Background())
	if err != nil { t.Fatal(err) }
	if got.RouterID != "router-1" || got.SiteID != "site-1" || got.Status != "online" || got.RouterConnectionStatus != "unknown" {
		t.Fatalf("unexpected heartbeat response: %+v", got)
	}
}

func TestHeartbeatRejectsUnauthorizedCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, testToken, server.Client())
	if err != nil { t.Fatal(err) }
	_, err = client.Heartbeat(context.Background())
	if !errors.Is(err, ErrUnauthorized) { t.Fatalf("expected ErrUnauthorized, got %v", err) }
}

func TestNewClientRequiresSecureURLAndValidToken(t *testing.T) {
	if _, err := NewClient("http://example.com", testToken, nil); err == nil {
		t.Fatal("expected non-local HTTP URL to be rejected")
	}
	if _, err := NewClient("https://cloud.example.com", strings.Repeat("z", 64), nil); err == nil {
		t.Fatal("expected invalid token to be rejected")
	}
	if _, err := NewClient("https://cloud.example.com", testToken, nil); err != nil {
		t.Fatalf("valid HTTPS config rejected: %v", err)
	}
}

func TestHeartbeatHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewClient(server.URL, testToken, &http.Client{Timeout: time.Second})
	if err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Heartbeat(ctx)
	if err == nil { t.Fatal("expected canceled request to fail") }
}
