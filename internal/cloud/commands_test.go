package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const cmdTestToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const testCommandID = "11111111-2222-3333-4444-555555555555"

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, cmdTestToken, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClaimCommandsSendsBearerAndDecodesBatch(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/gateway/commands/claim" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+cmdTestToken {
			t.Error("missing gateway bearer credential")
		}
		var body map[string]int
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["limit"] != 5 {
			t.Errorf("limit = %d", body["limit"])
		}
		_, _ = w.Write([]byte(`{"commands":[{"command_id":"` + testCommandID + `","site_id":"s","gateway_id":"g","router_id":"r","operation":"health.check","issued_at":"2026-11-22T10:00:00Z","expires_at":"2026-11-22T10:05:00Z","payload":{}}]}`))
	})
	commands, err := client.ClaimCommands(context.Background(), 5)
	if err != nil || len(commands) != 1 {
		t.Fatalf("commands=%v err=%v", commands, err)
	}
	if commands[0].CommandID != testCommandID || commands[0].Operation != "health.check" || commands[0].ExpiresAt.IsZero() {
		t.Fatalf("decoded command = %+v", commands[0])
	}
}

func TestClaimCommandsMapsErrors(t *testing.T) {
	cases := map[int]func(error) bool{
		http.StatusUnauthorized: func(err error) bool { return errors.Is(err, ErrUnauthorized) },
		http.StatusForbidden:    func(err error) bool { return errors.Is(err, ErrUnauthorized) },
		http.StatusBadGateway:   func(err error) bool { return err != nil && !errors.Is(err, ErrUnauthorized) },
	}
	for status, check := range cases {
		client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		if _, err := client.ClaimCommands(context.Background(), 1); !check(err) {
			t.Fatalf("status %d: got %v", status, err)
		}
	}
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) })
	if _, err := client.ClaimCommands(context.Background(), 1); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := client.ClaimCommands(context.Background(), 0); err == nil {
		t.Fatal("limit 0 accepted")
	}
}

func TestReportResultSendsOutcome(t *testing.T) {
	var got CommandResult
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/gateway/commands/"+testCommandID+"/result" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"commandId":"` + testCommandID + `","status":"failed"}`))
	})
	err := client.ReportResult(context.Background(), testCommandID, CommandResult{Success: false, Error: "router_unavailable: boom"})
	if err != nil || got.Success || got.Error != "router_unavailable: boom" {
		t.Fatalf("err=%v got=%+v", err, got)
	}
}

func TestReportResultClassifiesRefusalAndAuth(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusNotFound, http.StatusBadRequest} {
		client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		if err := client.ReportResult(context.Background(), testCommandID, CommandResult{Success: true}); !errors.Is(err, ErrResultRefused) {
			t.Fatalf("status %d: got %v", status, err)
		}
	}
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	if err := client.ReportResult(context.Background(), testCommandID, CommandResult{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
	client = newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	err := client.ReportResult(context.Background(), testCommandID, CommandResult{})
	if err == nil || errors.Is(err, ErrResultRefused) || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("server error must be retryable, got %v", err)
	}
}

func TestReportResultValidatesInput(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { t.Error("request must not be sent") })
	if err := client.ReportResult(context.Background(), "../etc/passwd", CommandResult{}); err == nil {
		t.Fatal("invalid command id accepted")
	}
	huge := CommandResult{Success: true, Result: map[string]any{"blob": strings.Repeat("x", 70<<10)}}
	if err := client.ReportResult(context.Background(), testCommandID, huge); err == nil {
		t.Fatal("oversized result accepted")
	}
}
