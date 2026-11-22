package routeros

import (
	"testing"
	"time"
)

func TestMonitorSnapshotIsInitiallyDisconnected(t *testing.T) {
	m := NewMonitor()
	got := m.Snapshot()
	if got.Connected || got.CheckedAt != (time.Time{}) {
		t.Fatalf("unexpected initial state: %+v", got)
	}
}

func TestMonitorSnapshotReturnsCopy(t *testing.T) {
	m := NewMonitor()
	m.set(Snapshot{Connected: true, Identity: "router", InterfaceCount: 3})
	got := m.Snapshot()
	got.Identity = "changed"
	if m.Snapshot().Identity != "router" {
		t.Fatal("snapshot mutated monitor state")
	}
}
