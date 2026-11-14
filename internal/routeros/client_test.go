package routeros

import (
	"context"
	"reflect"
	"testing"
)

type fakeExecutor struct {
	words []string
	rows []map[string]string
	err error
}
func (f *fakeExecutor) Execute(_ context.Context, words []string) ([]map[string]string, error) {
	f.words = append([]string(nil), words...)
	return f.rows, f.err
}

func TestSystemResourceUsesFixedReadOnlyCommand(t *testing.T) {
	f := &fakeExecutor{rows: []map[string]string{{"version":"7.16","uptime":"1d"}}}
	c, err := NewClient(f); if err != nil { t.Fatal(err) }
	got, err := c.SystemResource(context.Background()); if err != nil { t.Fatal(err) }
	if got.Version != "7.16" || got.Uptime != "1d" { t.Fatalf("unexpected resource: %+v", got) }
	if f.words[0] != "/system/resource/print" { t.Fatalf("unexpected command: %v", f.words) }
}

func TestIdentityRequiresOneNamedRow(t *testing.T) {
	f := &fakeExecutor{rows: []map[string]string{{"name":"  site-router "}}}
	c, _ := NewClient(f)
	got, err := c.Identity(context.Background()); if err != nil { t.Fatal(err) }
	if got.Name != "site-router" { t.Fatalf("got %q", got.Name) }
}

func TestInterfacesMapsFlags(t *testing.T) {
	f := &fakeExecutor{rows: []map[string]string{{"name":"ether1","type":"ether","running":"true","disabled":"false"}}}
	c, _ := NewClient(f)
	got, err := c.Interfaces(context.Background()); if err != nil { t.Fatal(err) }
	want := []Interface{{Name:"ether1", Type:"ether", Running:true, Disabled:false}}
	if !reflect.DeepEqual(got, want) { t.Fatalf("got %#v want %#v", got, want) }
}

func TestRejectUnexpectedRows(t *testing.T) {
	f := &fakeExecutor{}
	c, _ := NewClient(f)
	if _, err := c.Identity(context.Background()); err != ErrUnexpectedResponse { t.Fatalf("got %v", err) }
}

func TestNewClientRequiresExecutor(t *testing.T) {
	if _, err := NewClient(nil); err != ErrUnavailable { t.Fatalf("got %v", err) }
}
