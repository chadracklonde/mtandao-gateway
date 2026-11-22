package commands

import (
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/chadracklonde/mtandao-gateway/internal/cloud"
	"github.com/chadracklonde/mtandao-gateway/internal/executor"
	"github.com/chadracklonde/mtandao-gateway/internal/protocol"
	"github.com/chadracklonde/mtandao-gateway/internal/routeros"
)

type fakeCloud struct {
	queue     [][]protocol.Command // one batch per claim
	claimErr  error
	reportErr []error // consumed one per report call
	reported  map[string]cloud.CommandResult
	reports   int
}

func (f *fakeCloud) ClaimCommands(context.Context, int) ([]protocol.Command, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if len(f.queue) == 0 {
		return nil, nil
	}
	batch := f.queue[0]
	f.queue = f.queue[1:]
	return batch, nil
}

func (f *fakeCloud) ReportResult(_ context.Context, id string, r cloud.CommandResult) error {
	f.reports++
	if len(f.reportErr) > 0 {
		err := f.reportErr[0]
		f.reportErr = f.reportErr[1:]
		if err != nil {
			return err
		}
	}
	if f.reported == nil {
		f.reported = map[string]cloud.CommandResult{}
	}
	f.reported[id] = r
	return nil
}

type fakeRouter struct {
	created map[string]int
	down    bool
}

func (f *fakeRouter) Health(context.Context) (map[string]any, error) {
	return map[string]any{"identity": "MikroTik"}, nil
}
func (f *fakeRouter) Profiles(context.Context) ([]routeros.Profile, error) { return nil, nil }
func (f *fakeRouter) ActiveSessions(context.Context) ([]routeros.ActiveSession, error) {
	return nil, nil
}
func (f *fakeRouter) CreateVoucher(_ context.Context, code, _ string, _ time.Duration) error {
	f.created[code]++
	return nil
}
func (f *fakeRouter) DisableVoucher(context.Context, string) (int, error) { return 0, nil }

func setup(t *testing.T, api *fakeCloud) (*Runner, *executor.Ledger, *fakeRouter) {
	t.Helper()
	ledger, err := executor.OpenLedger(filepath.Join(t.TempDir(), "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	router := &fakeRouter{created: map[string]int{}}
	exec, _ := executor.New(ledger, func(context.Context) (executor.Router, func(), error) {
		if router.down {
			return nil, nil, errors.New("down")
		}
		return router, func() {}, nil
	}, executor.Target{})
	return NewRunner(api, exec, ledger, time.Second, log.New(io.Discard, "", 0)), ledger, router
}

func cmd(id, op string, payload map[string]any) protocol.Command {
	now := time.Now().UTC()
	return protocol.Command{CommandID: id, SiteID: "s", GatewayID: "g", RouterID: "r", Operation: op,
		IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(5 * time.Minute), Payload: payload}
}

func TestCycleExecutesAndReports(t *testing.T) {
	api := &fakeCloud{queue: [][]protocol.Command{{
		cmd("c1", "hotspot.voucher.create", map[string]any{"code": "V1"}),
		cmd("c2", "health.check", map[string]any{}),
	}}}
	r, ledger, router := setup(t, api)
	claimed, err := r.Cycle(context.Background())
	if err != nil || claimed != 2 {
		t.Fatalf("claimed=%d err=%v", claimed, err)
	}
	if router.created["V1"] != 1 || !api.reported["c1"].Success || api.reported["c2"].Result["identity"] != "MikroTik" {
		t.Fatalf("created=%v reported=%+v", router.created, api.reported)
	}
	if len(ledger.Unreported()) != 0 || len(ledger.Unfinished()) != 0 {
		t.Fatal("ledger not settled")
	}
}

func TestRedeliveredCommandIsNotExecutedTwice(t *testing.T) {
	c := cmd("c1", "hotspot.voucher.create", map[string]any{"code": "V1"})
	api := &fakeCloud{queue: [][]protocol.Command{{c}, {c}}}
	r, _, router := setup(t, api)
	for i := 0; i < 2; i++ {
		if _, err := r.Cycle(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if router.created["V1"] != 1 {
		t.Fatalf("voucher created %d times", router.created["V1"])
	}
}

func TestUnreportedResultIsRetriedWithoutReexecution(t *testing.T) {
	api := &fakeCloud{
		queue:     [][]protocol.Command{{cmd("c1", "hotspot.voucher.create", map[string]any{"code": "V1"})}},
		reportErr: []error{errors.New("network down")},
	}
	r, ledger, router := setup(t, api)
	if _, err := r.Cycle(context.Background()); err == nil {
		t.Fatal("report failure must surface so the loop backs off")
	}
	if len(ledger.Unreported()) != 1 || router.created["V1"] != 1 {
		t.Fatalf("unreported=%d created=%v", len(ledger.Unreported()), router.created)
	}
	if _, err := r.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !api.reported["c1"].Success || router.created["V1"] != 1 || len(ledger.Unreported()) != 0 {
		t.Fatalf("reported=%+v created=%v", api.reported, router.created)
	}
}

func TestRefusedResultIsDroppedNotRetriedForever(t *testing.T) {
	api := &fakeCloud{
		queue:     [][]protocol.Command{{cmd("c1", "health.check", map[string]any{})}},
		reportErr: []error{cloud.ErrResultRefused},
	}
	r, ledger, _ := setup(t, api)
	if _, err := r.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ledger.Unreported()) != 0 || api.reports != 1 {
		t.Fatalf("unreported=%d reports=%d", len(ledger.Unreported()), api.reports)
	}
}

func TestClaimedCommandsSurviveACrashBeforeExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.json")
	ledger, _ := executor.OpenLedger(path)
	// The Gateway claimed the command, persisted it, then crashed.
	if _, err := ledger.Receive(cmd("c1", "hotspot.voucher.create", map[string]any{"code": "V7"})); err != nil {
		t.Fatal(err)
	}

	api := &fakeCloud{}
	reopened, err := executor.OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	router := &fakeRouter{created: map[string]int{}}
	exec, _ := executor.New(reopened, func(context.Context) (executor.Router, func(), error) { return router, func() {}, nil }, executor.Target{})
	r := NewRunner(api, exec, reopened, time.Second, log.New(io.Discard, "", 0))
	if _, err := r.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if router.created["V7"] != 1 || !api.reported["c1"].Success {
		t.Fatalf("created=%v reported=%+v", router.created, api.reported)
	}
}

func TestRouterDownProducesAFailureReportNotAHang(t *testing.T) {
	api := &fakeCloud{queue: [][]protocol.Command{{cmd("c1", "health.check", map[string]any{})}}}
	r, _, router := setup(t, api)
	router.down = true
	if _, err := r.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := api.reported["c1"]
	if got.Success || got.Error != "router_unavailable: RouterOS is unavailable" {
		t.Fatalf("reported = %+v", got)
	}
}

func TestClaimErrorsAreReturned(t *testing.T) {
	api := &fakeCloud{claimErr: cloud.ErrUnauthorized}
	r, _, _ := setup(t, api)
	if _, err := r.Cycle(context.Background()); !errors.Is(err, cloud.ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	api := &fakeCloud{}
	r, _, _ := setup(t, api)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}
