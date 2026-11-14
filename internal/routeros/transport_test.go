package routeros

import (
	"context"
	"errors"
	"testing"

	ros "github.com/go-routeros/routeros/v3"
	"github.com/go-routeros/routeros/v3/proto"
)

type apiFake struct { words []string; reply *ros.Reply; err error; closed bool }
func (f *apiFake) RunContext(_ context.Context, words ...string) (*ros.Reply,error) { f.words=append([]string(nil),words...); return f.reply,f.err }
func (f *apiFake) Close() error { f.closed=true; return nil }

func TestTransportAllowsOnlyFixedReadCommands(t *testing.T) {
	f:=&apiFake{reply:&ros.Reply{Re:[]*proto.Sentence{{Map:map[string]string{"name":"ether1"}}}}}
	tr,err:=NewTransport(f,0); if err!=nil { t.Fatal(err) }
	rows,err:=tr.Execute(context.Background(),[]string{"/interface/print","=.proplist=name,type"})
	if err!=nil || len(rows)!=1 || rows[0]["name"]!="ether1" { t.Fatalf("rows=%v err=%v",rows,err) }
	if _,err=tr.Execute(context.Background(),[]string{"/system/reboot"}); err==nil { t.Fatal("write command unexpectedly allowed") }
	if len(f.words)!=2 || f.words[0]!="/interface/print" { t.Fatalf("unexpected command sent: %v",f.words) }
}

func TestTransportRejectsMalformedRequest(t *testing.T) {
	f:=&apiFake{}
	tr,_:=NewTransport(f,0)
	if _,err:=tr.Execute(context.Background(),[]string{"/interface/print"}); err==nil { t.Fatal("expected error") }
	if len(f.words)!=0 { t.Fatal("invalid command reached client") }
}

func TestTransportPropagatesAPIError(t *testing.T) {
	f:=&apiFake{err:errors.New("offline")}
	tr,_:=NewTransport(f,0)
	_,err:=tr.Execute(context.Background(),[]string{"/system/identity/print","=.proplist=name"})
	if err==nil || err.Error()!="offline" { t.Fatalf("got %v",err) }
}

func TestDialRejectsPublicAndHostnameTargets(t *testing.T) {
	for _,addr:=range []string{"8.8.8.8:8729","router.local:8729"} {
		_,err:=DialTLS(context.Background(),TLSConfig{Address:addr,Username:"u",Password:"p"})
		if !errors.Is(err,ErrNonLocalTarget) { t.Fatalf("%s: got %v",addr,err) }
	}
}
