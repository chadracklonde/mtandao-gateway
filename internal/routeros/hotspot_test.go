package routeros

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeRouter simulates the subset of RouterOS used by Hotspot, enforcing the
// same allow-list as the real transport.
type fakeRouter struct {
	users    map[string]map[string]string // id -> attributes
	sessions []map[string]string
	sent     [][]string
	nextID   int
	failOn   string
}

func newFakeRouter() *fakeRouter {
	return &fakeRouter{users: map[string]map[string]string{}}
}

func (f *fakeRouter) ExecuteHotspot(_ context.Context, words []string) ([]map[string]string, error) {
	if err := ValidateHotspotWords(words); err != nil {
		return nil, err
	}
	f.sent = append(f.sent, append([]string(nil), words...))
	if f.failOn == words[0] {
		return nil, errors.New("connection reset")
	}
	attr := map[string]string{}
	query := map[string]string{}
	for _, w := range words[1:] {
		key, value, _ := strings.Cut(w[1:], "=")
		if w[0] == '?' {
			query[key] = value
		} else {
			attr[key] = value
		}
	}
	switch words[0] {
	case "/ip/hotspot/user/profile/print":
		return []map[string]string{{"name": "1h", "rate-limit": "2M/2M"}, {"name": "default"}}, nil
	case "/ip/hotspot/active/print":
		var out []map[string]string
		for _, s := range f.sessions {
			if query["user"] == "" || s["user"] == query["user"] {
				out = append(out, s)
			}
		}
		return out, nil
	case "/ip/hotspot/user/print":
		var out []map[string]string
		for id, u := range f.users {
			if query["name"] == "" || u["name"] == query["name"] {
				out = append(out, map[string]string{".id": id, "name": u["name"], "disabled": u["disabled"]})
			}
		}
		return out, nil
	case "/ip/hotspot/user/add":
		f.nextID++
		f.users[fmt.Sprintf("*%X", f.nextID)] = attr
		return nil, nil
	case "/ip/hotspot/user/set":
		u, ok := f.users[attr[".id"]]
		if !ok {
			return nil, errors.New("no such item")
		}
		u["disabled"] = attr["disabled"]
		return nil, nil
	case "/ip/hotspot/active/remove":
		var kept []map[string]string
		for _, s := range f.sessions {
			if s[".id"] != attr[".id"] {
				kept = append(kept, s)
			}
		}
		f.sessions = kept
		return nil, nil
	}
	return nil, ErrUnsupportedRequest
}

func TestCreateVoucherBuildsAllowedWordsOnly(t *testing.T) {
	f := newFakeRouter()
	h, _ := NewHotspot(f)
	err := h.CreateVoucher(context.Background(), "AB-12", VoucherOptions{
		Profile: "1h", Duration: 90 * time.Minute, DataLimitMB: 500, Comment: "vc-221126-1432 cloud",
	})
	if err != nil {
		t.Fatal(err)
	}
	last := f.sent[len(f.sent)-1]
	want := []string{
		"/ip/hotspot/user/add", "=name=AB-12", "=password=AB-12", "=profile=1h",
		"=comment=vc-221126-1432 cloud", "=limit-uptime=1h30m", "=limit-bytes-total=524288000",
	}
	if strings.Join(last, "|") != strings.Join(want, "|") {
		t.Fatalf("sent %v want %v", last, want)
	}
	if len(f.users) != 1 {
		t.Fatalf("users = %v", f.users)
	}
}

func TestCreateVoucherRefusesDuplicates(t *testing.T) {
	f := newFakeRouter()
	h, _ := NewHotspot(f)
	if err := h.CreateVoucher(context.Background(), "DUP", VoucherOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.CreateVoucher(context.Background(), "DUP", VoucherOptions{}); !errors.Is(err, ErrVoucherExists) {
		t.Fatalf("got %v", err)
	}
	if len(f.users) != 1 {
		t.Fatal("duplicate voucher was created")
	}
}

func TestDisableVoucherEndsSessions(t *testing.T) {
	f := newFakeRouter()
	h, _ := NewHotspot(f)
	_ = h.CreateVoucher(context.Background(), "V1", VoucherOptions{})
	f.sessions = []map[string]string{{".id": "*A", "user": "V1"}, {".id": "*B", "user": "OTHER"}}
	ended, err := h.DisableVoucher(context.Background(), "V1")
	if err != nil || ended != 1 {
		t.Fatalf("ended=%d err=%v", ended, err)
	}
	if len(f.sessions) != 1 || f.sessions[0]["user"] != "OTHER" {
		t.Fatalf("sessions = %v", f.sessions)
	}
	for _, u := range f.users {
		if u["disabled"] != "yes" {
			t.Fatal("user not disabled")
		}
	}
	if _, err := h.DisableVoucher(context.Background(), "V1"); err != nil {
		t.Fatalf("second disable must succeed: %v", err)
	}
	if _, err := h.DisableVoucher(context.Background(), "NOPE"); !errors.Is(err, ErrVoucherNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestListings(t *testing.T) {
	f := newFakeRouter()
	f.sessions = []map[string]string{{"user": "V1", "address": "10.0.0.5", "mac-address": "AA:BB"}}
	h, _ := NewHotspot(f)
	profiles, err := h.Profiles(context.Background())
	if err != nil || len(profiles) != 2 || profiles[0].RateLimit != "2M/2M" {
		t.Fatalf("profiles=%v err=%v", profiles, err)
	}
	sessions, err := h.ActiveSessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].MACAddress != "AA:BB" {
		t.Fatalf("sessions=%v err=%v", sessions, err)
	}
}

func TestRouterErrorsPropagate(t *testing.T) {
	f := newFakeRouter()
	f.failOn = "/ip/hotspot/user/print"
	h, _ := NewHotspot(f)
	if err := h.CreateVoucher(context.Background(), "X", VoucherOptions{}); err == nil || errors.Is(err, ErrVoucherExists) {
		t.Fatalf("got %v", err)
	}
	if len(f.users) != 0 {
		t.Fatal("voucher created despite lookup failure")
	}
}

func TestValidateHotspotWords(t *testing.T) {
	valid := [][]string{
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1"},
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=profile=1h", "=limit-uptime=1d2h30m"},
		{"/ip/hotspot/user/set", "=.id=*1F", "=disabled=yes"},
		{"/ip/hotspot/active/remove", "=.id=*A"},
		{"/ip/hotspot/user/print", "?name=A1", "=.proplist=.id,name"},
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=comment=vc-221126-1432", "=limit-bytes-total=524288000"},
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=comment=up-010126-0905 Client Jean"},
	}
	for _, words := range valid {
		if err := ValidateHotspotWords(words); err != nil {
			t.Errorf("%v rejected: %v", words, err)
		}
	}
	invalid := [][]string{
		nil,
		{"/system/reboot"},
		{"/ip/hotspot/user/remove", "=.id=*1"},
		{"/ip/hotspot/user/add", "=name=A1"}, // missing password
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=comment=x"}, // attribute not allowed
		{"/ip/hotspot/user/add", "=name=A1", "=name=A2", "=password=x"},    // duplicate attribute
		{"/ip/hotspot/user/add", "=name=A 1;/x", "=password=A1"},           // unsafe value
		{"/ip/hotspot/user/add", "=name=A1\n=password=x", "=password=A1"},  // newline
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=limit-uptime=forever"},
		{"/ip/hotspot/user/set", "=.id=1", "=disabled=yes"}, // malformed id
		{"/ip/hotspot/user/set", "=.id=*1", "=disabled=maybe"},
		{"/ip/hotspot/user/set", "=.id=*1"},       // missing disabled
		{"/ip/hotspot/user/print", "?password=x"}, // query not allowed
		{"/ip/hotspot/user/print", "=.proplist=password"},
		{"/ip/hotspot/user/print", "=.proplist=name,name"},
		{"/ip/hotspot/active/remove", "=.id=*A", "=.id=*B"},
		{"/ip/hotspot/user/print", "name=x"},                                                          // no prefix
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=comment=free text"},                    // not the ticket format
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=comment=vc-221126-1432;/system reset"}, // injection attempt
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=comment=vc-22112-1432"},                // malformed date
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=limit-bytes-total=0"},
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=limit-bytes-total=-5"},
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=limit-bytes-total=10G"},
		{"/ip/hotspot/user/add", "=name=A1", "=password=A1", "=limit-bytes-total=1", "=limit-bytes-total=2"},
	}
	for _, words := range invalid {
		if err := ValidateHotspotWords(words); !errors.Is(err, ErrUnsupportedRequest) {
			t.Errorf("%q accepted", words)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Minute: "1m", 60 * time.Minute: "1h", 90 * time.Minute: "1h30m",
		24 * time.Hour: "1d", 26*time.Hour + 5*time.Minute: "1d2h5m", 10 * time.Second: "1m",
	} {
		if got := formatUptime(d); got != want {
			t.Errorf("%v: got %q want %q", d, got, want)
		}
	}
}
