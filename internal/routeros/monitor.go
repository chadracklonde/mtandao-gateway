package routeros

import (
	"context"
	"sync"
	"time"
)

type Snapshot struct {
	Connected bool `json:"connected"`
	Identity string `json:"identity,omitempty"`
	Version string `json:"version,omitempty"`
	Uptime string `json:"uptime,omitempty"`
	InterfaceCount int `json:"interface_count"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type Monitor struct {
	mu sync.RWMutex
	state Snapshot
}

func NewMonitor() *Monitor { return &Monitor{} }

func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Monitor) set(state Snapshot) {
	m.mu.Lock()
	m.state = state
	m.mu.Unlock()
}

// Run maintains a read-only RouterOS connection and refreshes its state.
// It reconnects after failures and never logs or stores credentials.
func (m *Monitor) Run(ctx context.Context, cfg Config, interval time.Duration) {
	if interval < 5*time.Second { interval = 30*time.Second }
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done(): return
		case <-timer.C:
		}
		transport, err := DialTLS(ctx, cfg.TLSConfig())
		if err == nil {
			client, clientErr := NewClient(transport)
			if clientErr != nil { err = clientErr } else {
				identity, idErr := client.Identity(ctx)
				resource, resourceErr := client.SystemResource(ctx)
				interfaces, interfaceErr := client.Interfaces(ctx)
				if idErr != nil { err = idErr } else if resourceErr != nil { err = resourceErr } else if interfaceErr != nil { err = interfaceErr } else {
					m.set(Snapshot{Connected:true,Identity:identity.Name,Version:resource.Version,Uptime:resource.Uptime,InterfaceCount:len(interfaces),CheckedAt:time.Now().UTC()})
				}
			}
			_ = transport.Close()
		}
		if err != nil {
			m.set(Snapshot{Connected:false,CheckedAt:time.Now().UTC(),LastError:"RouterOS unavailable"})
		}
		timer.Reset(interval)
	}
}
