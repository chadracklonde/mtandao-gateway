package cloud

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunHeartbeatStopsOnContextCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"routerId":"r","siteId":"s","status":"online","lastSeenAt":"2026-11-14T10:00:00Z"}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, testToken, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	logger := log.New(io.Discard, "", 0)
	done := make(chan struct{})
	go func() { RunHeartbeat(ctx, client, time.Hour, logger); close(done) }()
	deadline := time.After(2 * time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("initial heartbeat was not sent")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat runner did not stop")
	}
}
