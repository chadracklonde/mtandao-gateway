package protocol

import (
	"errors"
	"strings"
	"testing"
)

func TestParseVoucherCreate(t *testing.T) {
	got, err := ParseVoucherCreate(map[string]any{"code": "AB-12", "profile": "1 heure", "duration_minutes": float64(60)})
	if err != nil || got.Code != "AB-12" || got.Profile != "1 heure" || got.DurationMinutes != 60 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	minimal, err := ParseVoucherCreate(map[string]any{"code": "X"})
	if err != nil || minimal.Profile != "" || minimal.DurationMinutes != 0 {
		t.Fatalf("minimal %+v err=%v", minimal, err)
	}
}

func TestParseVoucherCreateRejectsInvalidPayloads(t *testing.T) {
	cases := map[string]map[string]any{
		"missing code":     {},
		"empty code":       {"code": ""},
		"code too long":    {"code": strings.Repeat("a", 65)},
		"code with space":  {"code": "a b"},
		"code with quote":  {"code": `a"; /system reset`},
		"code wrong type":  {"code": 12},
		"unknown field":    {"code": "a", "cmd": "/system/reboot"},
		"profile injected": {"code": "a", "profile": "x\n=disabled=no"},
		"profile type":     {"code": "a", "profile": 3},
		"fractional":       {"code": "a", "duration_minutes": 1.5},
		"zero":             {"code": "a", "duration_minutes": float64(0)},
		"too large":        {"code": "a", "duration_minutes": float64(525601)},
		"string duration":  {"code": "a", "duration_minutes": "60"},
	}
	for name, payload := range cases {
		if _, err := ParseVoucherCreate(payload); !errors.Is(err, ErrInvalidPayload) {
			t.Errorf("%s: accepted (err=%v)", name, err)
		}
	}
}

func TestParseVoucherDisableAndEmptyPayload(t *testing.T) {
	if code, err := ParseVoucherDisable(map[string]any{"code": "AB_1.x"}); err != nil || code != "AB_1.x" {
		t.Fatalf("code=%q err=%v", code, err)
	}
	for _, payload := range []map[string]any{{}, {"code": "a", "profile": "p"}, {"code": "bad code"}} {
		if _, err := ParseVoucherDisable(payload); !errors.Is(err, ErrInvalidPayload) {
			t.Errorf("accepted %v", payload)
		}
	}
	if err := RequireEmptyPayload(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := RequireEmptyPayload(map[string]any{"x": 1}); !errors.Is(err, ErrInvalidPayload) {
		t.Fatal("payload accepted for a no-argument operation")
	}
}

func TestIsWriteOperation(t *testing.T) {
	for op, want := range map[string]bool{
		"hotspot.voucher.create": true, "hotspot.voucher.disable": true,
		"health.check": false, "hotspot.profile.list": false, "hotspot.active-sessions.list": false,
	} {
		if IsWriteOperation(op) != want {
			t.Errorf("%s", op)
		}
	}
}
