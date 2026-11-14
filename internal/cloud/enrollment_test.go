package cloud

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedeemEnrollmentExchangesOneTimeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/gateway/enroll" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("missing JSON content type")
		}
		var body struct { EnrollmentToken string `json:"enrollmentToken"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { t.Fatal(err) }
		if body.EnrollmentToken != testToken { t.Fatal("wrong enrollment token") }
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"routerId":"router-1","siteId":"site-1","gatewayToken":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`))
	}))
	defer server.Close()

	got, err := RedeemEnrollment(context.Background(), server.URL, testToken, server.Client())
	if err != nil { t.Fatal(err) }
	if got.RouterID != "router-1" || got.SiteID != "site-1" || got.GatewayToken != testToken {
		t.Fatalf("unexpected enrollment response: %+v", got)
	}
}

func TestRedeemEnrollmentRejectsInvalidTokenAndUnauthorized(t *testing.T) {
	if _, err := RedeemEnrollment(context.Background(), "https://cloud.example.com", strings.Repeat("z", 64), nil); err == nil {
		t.Fatal("expected invalid token to be rejected")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := RedeemEnrollment(context.Background(), server.URL, testToken, server.Client())
	if !errors.Is(err, ErrUnauthorized) { t.Fatalf("expected ErrUnauthorized, got %v", err) }
}
