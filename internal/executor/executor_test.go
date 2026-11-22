package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chadracklonde/mtandao-gateway/internal/protocol"
	"github.com/chadracklonde/mtandao-gateway/internal/routeros"
)

type fakeRouter struct {
	calls      []string
	vouchers   map[string]bool
	sessions   []routeros.ActiveSession
	failHealth bool
}

func (f *fakeRouter) Health(context.Context) (map[string]any, error) {
	f.calls = append(f.calls, "health")
	if f.failHealth {
		return nil, errors.New("timeout")
	}
	return map[string]any{"identity": "MikroTik", "version": "7.15"}, nil
}
func (f *fakeRouter) Profiles(context.Context) ([]routeros.Profile, error) {
	f.calls = append(f.calls, "profiles")
	return []routeros.Profile{{Name: "1h"}}, nil
}
func (f *fakeRouter) ActiveSessions(context.Context) ([]routeros.ActiveSession, error) {
	f.calls = append(f.calls, "sessions")
	return f.sessions, nil
}
func (f *fakeRouter) CreateVoucher(_ context.Context, code, profile string, d time.Duration) error {
	f.calls = append(f.calls, "create:"+code)
	if f.vouchers[code] {
		return routeros.ErrVoucherExists
	}
	f.vouchers[code] = true
	return nil
}
func (f *fakeRouter) DisableVoucher(_ context.Context, code string) (int, error) {
	f.calls = append(f.calls, "disable:"+code)
	if !f.vouchers[code] {
		return 0, routeros.ErrVoucherNotFound
	}
	return 2, nil
}

func newExec(t *testing.T, router *fakeRouter, target Target) (*Executor, *Ledger, *int) {
	t.Helper()
	ledger, err := OpenLedger(filepath.Join(t.TempDir(), "state", "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	connects := 0
	exec, err := New(ledger, func(context.Context) (Router, func(), error) {
		connects++
		return router, func() {}, nil
	}, target)
	if err != nil {
		t.Fatal(err)
	}
	return exec, ledger, &connects
}

func command(id, op string, payload map[string]any) protocol.Command {
	now := time.Now().UTC()
	return protocol.Command{
		CommandID: id, SiteID: "site", GatewayID: "gw", RouterID: "router", Operation: op,
		IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(5 * time.Minute), Payload: payload,
	}
}

func run(t *testing.T, e *Executor, l *Ledger, cmd protocol.Command) Outcome {
	t.Helper()
	entry, err := l.Receive(cmd)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := e.Process(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestCreateVoucherRunsOnceEvenIfProcessedAgain(t *testing.T) {
	router := &fakeRouter{vouchers: map[string]bool{}}
	e, l, _ := newExec(t, router, Target{})
	cmd := command("c1", "hotspot.voucher.create", map[string]any{"code": "V1", "duration_minutes": float64(60)})
	first := run(t, e, l, cmd)
	if !first.Success || first.Result["code"] != "V1" {
		t.Fatalf("outcome = %+v", first)
	}
	// Same command delivered again: same outcome, router untouched.
	second := run(t, e, l, cmd)
	if !second.Success || len(router.calls) != 1 {
		t.Fatalf("second=%+v calls=%v", second, router.calls)
	}
}

func TestOutcomesAreMappedToStableCodes(t *testing.T) {
	router := &fakeRouter{vouchers: map[string]bool{"EXISTS": true}}
	e, l, _ := newExec(t, router, Target{})

	if o := run(t, e, l, command("c1", "hotspot.voucher.create", map[string]any{"code": "EXISTS"})); o.Code != CodeVoucherExists || o.Success {
		t.Fatalf("duplicate = %+v", o)
	}
	if o := run(t, e, l, command("c2", "hotspot.voucher.disable", map[string]any{"code": "NOPE"})); o.Code != CodeVoucherNotFound {
		t.Fatalf("missing = %+v", o)
	}
	o := run(t, e, l, command("c3", "hotspot.voucher.disable", map[string]any{"code": "EXISTS"}))
	if !o.Success || o.Result["sessions_ended"] != 2 {
		t.Fatalf("disable = %+v", o)
	}
	router.failHealth = true
	if o := run(t, e, l, command("c4", "health.check", map[string]any{})); o.Code != CodeRouterUnavailable {
		t.Fatalf("health = %+v", o)
	}
	if got := (Outcome{Code: "expired", Message: "late"}).CloudError(); got != "expired: late" {
		t.Fatalf("CloudError = %q", got)
	}
}

func TestInvalidCommandsNeverReachTheRouter(t *testing.T) {
	router := &fakeRouter{vouchers: map[string]bool{}}
	e, l, connects := newExec(t, router, Target{SiteID: "site", RouterID: "router"})

	expired := command("c1", "health.check", map[string]any{})
	expired.IssuedAt, expired.ExpiresAt = time.Now().Add(-time.Hour), time.Now().Add(-time.Minute)
	wrongRouter := command("c2", "health.check", map[string]any{})
	wrongRouter.RouterID = "other"
	cases := map[string]struct {
		cmd  protocol.Command
		code string
	}{
		"expired":        {expired, CodeExpired},
		"wrong router":   {wrongRouter, CodeWrongTarget},
		"unknown op":     {command("c3", "system.reboot", map[string]any{}), CodeInvalidCommand},
		"bad payload":    {command("c4", "hotspot.voucher.create", map[string]any{"code": "a b"}), CodeInvalidPayload},
		"extra field":    {command("c5", "hotspot.voucher.disable", map[string]any{"code": "a", "x": 1}), CodeInvalidPayload},
		"payload on get": {command("c6", "health.check", map[string]any{"x": 1}), CodeInvalidPayload},
	}
	for name, tc := range cases {
		if o := run(t, e, l, tc.cmd); o.Code != tc.code || o.Success {
			t.Errorf("%s: outcome %+v, want code %s", name, o, tc.code)
		}
	}
	if *connects != 0 || len(router.calls) != 0 {
		t.Fatalf("router was contacted: connects=%d calls=%v", *connects, router.calls)
	}
}

func TestRouterUnavailableWhenConnectionFails(t *testing.T) {
	ledger, _ := OpenLedger(filepath.Join(t.TempDir(), "c.json"))
	e, _ := New(ledger, func(context.Context) (Router, func(), error) { return nil, nil, errors.New("tls: bad certificate") }, Target{})
	o := run(t, e, ledger, command("c1", "health.check", map[string]any{}))
	if o.Code != CodeRouterUnavailable || o.Message == "" {
		t.Fatalf("outcome %+v", o)
	}
	for _, leak := range []string{"tls", "certificate"} {
		if strings.Contains(o.Message, leak) {
			t.Fatalf("internal error leaked to the Cloud: %q", o.Message)
		}
	}
}

func TestInterruptedWriteIsNeverReplayedButReadIs(t *testing.T) {
	router := &fakeRouter{vouchers: map[string]bool{}}
	e, l, _ := newExec(t, router, Target{})

	write := command("w1", "hotspot.voucher.create", map[string]any{"code": "V9"})
	if _, err := l.Receive(write); err != nil {
		t.Fatal(err)
	}
	if err := l.Begin("w1"); err != nil { // the Gateway crashed right after this
		t.Fatal(err)
	}
	read := command("r1", "hotspot.profile.list", map[string]any{})
	if _, err := l.Receive(read); err != nil {
		t.Fatal(err)
	}
	_ = l.Begin("r1")

	for _, entry := range l.Unfinished() {
		if _, err := e.Process(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	w, _ := l.Get("w1")
	if w.Outcome == nil || w.Outcome.Code != CodeInterrupted || router.vouchers["V9"] {
		t.Fatalf("write entry = %+v vouchers=%v", w.Outcome, router.vouchers)
	}
	r, _ := l.Get("r1")
	if r.Outcome == nil || !r.Outcome.Success {
		t.Fatalf("read entry = %+v", r.Outcome)
	}
}

func TestListResultsAreCapped(t *testing.T) {
	router := &fakeRouter{vouchers: map[string]bool{}, sessions: make([]routeros.ActiveSession, 600)}
	e, l, _ := newExec(t, router, Target{})
	o := run(t, e, l, command("c1", "hotspot.active-sessions.list", map[string]any{}))
	rows := o.Result["sessions"].([]routeros.ActiveSession)
	if len(rows) != maxListRows || o.Result["truncated"] != true || o.Result["count"] != 600 {
		t.Fatalf("rows=%d result=%v", len(rows), o.Result["truncated"])
	}
}

func TestLedgerSurvivesRestartAndIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "commands.json")
	l1, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := command("c1", "hotspot.voucher.create", map[string]any{"code": "SECRET1"})
	if _, err := l1.Receive(cmd); err != nil {
		t.Fatal(err)
	}
	_ = l1.Begin("c1")
	_ = l1.Complete("c1", Outcome{Success: true, Result: map[string]any{"code": "SECRET1"}})

	l2, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := l2.Get("c1")
	if !ok || entry.State != StateDone || entry.Outcome == nil || !entry.Outcome.Success {
		t.Fatalf("entry after restart = %+v", entry)
	}
	if len(l2.Unreported()) != 1 {
		t.Fatal("completed but unreported command must be listed")
	}
	_ = l2.MarkReported("c1")
	if len(l2.Unreported()) != 0 {
		t.Fatal("reported command still listed")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("ledger mode = %v", info.Mode().Perm())
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm()&0o077 != 0 {
		t.Fatalf("ledger directory mode = %v", dir.Mode().Perm())
	}
}

func TestLedgerRejectsUnsafeOrCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	open := filepath.Join(dir, "open.json")
	_ = os.WriteFile(open, []byte(`{"version":1,"entries":{}}`), 0o644)
	_ = os.Chmod(open, 0o644)
	if _, err := OpenLedger(open); err == nil {
		t.Fatal("group/world readable ledger accepted")
	}
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte("{broken"), 0o600)
	if _, err := OpenLedger(bad); err == nil {
		t.Fatal("corrupt ledger accepted")
	}
	target := filepath.Join(dir, "target.json")
	_ = os.WriteFile(target, []byte(`{"version":1,"entries":{}}`), 0o600)
	link := filepath.Join(dir, "link.json")
	_ = os.Symlink(target, link)
	if _, err := OpenLedger(link); err == nil {
		t.Fatal("symlinked ledger accepted")
	}
	if _, err := OpenLedger(""); err == nil {
		t.Fatal("empty path accepted")
	}
}

func TestReceiveIsIdempotent(t *testing.T) {
	l, _ := OpenLedger(filepath.Join(t.TempDir(), "c.json"))
	cmd := command("c1", "health.check", map[string]any{})
	first, _ := l.Receive(cmd)
	_ = l.Begin("c1")
	second, err := l.Receive(cmd)
	if err != nil || second.ReceivedAt != first.ReceivedAt || second.State != StateStarted {
		t.Fatalf("second receive changed the entry: %+v err=%v", second, err)
	}
}
