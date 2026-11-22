package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/chadracklonde/mtandao-gateway/internal/protocol"
)

// Command states in the ledger.
const (
	StateReceived = "received" // claimed from the Cloud, not started
	StateStarted  = "started"  // execution began; the outcome is unknown after a crash
	StateDone     = "done"     // outcome recorded
)

const (
	ledgerVersion  = 1
	maxEntries     = 5000
	retentionAfter = 30 * 24 * time.Hour
)

// Outcome is what a command produced. It is stored so a repeated or replayed
// command returns the same result instead of running again.
type Outcome struct {
	Success bool           `json:"success"`
	Result  map[string]any `json:"result,omitempty"`
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
}

type Entry struct {
	Command     protocol.Command `json:"command"`
	State       string           `json:"state"`
	ReceivedAt  time.Time        `json:"received_at"`
	StartedAt   time.Time        `json:"started_at,omitempty"`
	CompletedAt time.Time        `json:"completed_at,omitempty"`
	Outcome     *Outcome         `json:"outcome,omitempty"`
	Reported    bool             `json:"reported"`
}

type ledgerFile struct {
	Version int               `json:"version"`
	Entries map[string]*Entry `json:"entries"`
}

// Ledger is a small durable store of every command the Gateway has received.
// It makes execution idempotent and survives restarts. The file holds voucher
// codes, so it is private (0600 in a 0700 directory).
type Ledger struct {
	mu      sync.Mutex
	path    string
	entries map[string]*Entry
	now     func() time.Time
}

// OpenLedger loads the ledger at path, creating its directory if needed.
func OpenLedger(path string) (*Ledger, error) {
	if path == "" {
		return nil, errors.New("ledger path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create ledger directory: %w", err)
	}
	l := &Ledger{path: path, entries: map[string]*Entry{}, now: func() time.Time { return time.Now().UTC() }}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return l, nil
	case err != nil:
		return nil, fmt.Errorf("inspect ledger: %w", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
		return nil, errors.New("ledger must be a regular file")
	case info.Mode().Perm()&0o077 != 0:
		return nil, errors.New("ledger permissions must be 0600 or stricter")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	var file ledgerFile
	if err := json.Unmarshal(data, &file); err != nil || file.Version != ledgerVersion {
		return nil, errors.New("ledger is corrupted or has an unsupported version")
	}
	if file.Entries != nil {
		l.entries = file.Entries
	}
	return l, nil
}

// Receive records a claimed command. If the command is already known its
// existing entry is returned unchanged, which is what makes redelivery harmless.
func (l *Ledger) Receive(cmd protocol.Command) (Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.entries[cmd.CommandID]; ok {
		return *existing, nil
	}
	entry := &Entry{Command: cmd, State: StateReceived, ReceivedAt: l.now()}
	l.entries[cmd.CommandID] = entry
	l.pruneLocked()
	if err := l.saveLocked(); err != nil {
		delete(l.entries, cmd.CommandID)
		return Entry{}, err
	}
	return *entry, nil
}

// Begin marks a command as started and persists it before anything runs.
func (l *Ledger) Begin(id string) error {
	return l.update(id, func(e *Entry) error {
		if e.State == StateDone {
			return errors.New("command already completed")
		}
		e.State, e.StartedAt = StateStarted, l.now()
		return nil
	})
}

// Complete records the outcome.
func (l *Ledger) Complete(id string, outcome Outcome) error {
	return l.update(id, func(e *Entry) error {
		e.State, e.CompletedAt, e.Outcome = StateDone, l.now(), &outcome
		return nil
	})
}

// MarkReported notes that the Cloud has the outcome (or refused it for good).
func (l *Ledger) MarkReported(id string) error {
	return l.update(id, func(e *Entry) error {
		e.Reported = true
		return nil
	})
}

// Get returns a copy of an entry.
func (l *Ledger) Get(id string) (Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[id]
	if !ok {
		return Entry{}, false
	}
	return *entry, true
}

// Unfinished returns commands that are not done, oldest first.
func (l *Ledger) Unfinished() []Entry {
	return l.collect(func(e *Entry) bool { return e.State != StateDone })
}

// Unreported returns finished commands whose outcome the Cloud has not acknowledged.
func (l *Ledger) Unreported() []Entry {
	return l.collect(func(e *Entry) bool { return e.State == StateDone && !e.Reported })
}

func (l *Ledger) collect(keep func(*Entry) bool) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	for _, entry := range l.entries {
		if keep(entry) {
			out = append(out, *entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ReceivedAt.Equal(out[j].ReceivedAt) {
			return out[i].ReceivedAt.Before(out[j].ReceivedAt)
		}
		return out[i].Command.CommandID < out[j].Command.CommandID
	})
	return out
}

func (l *Ledger) update(id string, change func(*Entry) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[id]
	if !ok {
		return errors.New("unknown command")
	}
	before := *entry
	if err := change(entry); err != nil {
		return err
	}
	if err := l.saveLocked(); err != nil {
		*entry = before
		return err
	}
	return nil
}

// pruneLocked drops old, fully reported entries. Unfinished or unreported
// entries are never dropped, so the idempotence guarantee holds while they matter.
func (l *Ledger) pruneLocked() {
	if len(l.entries) <= maxEntries {
		return
	}
	cutoff := l.now().Add(-retentionAfter)
	for id, entry := range l.entries {
		if entry.State == StateDone && entry.Reported && entry.CompletedAt.Before(cutoff) {
			delete(l.entries, id)
		}
	}
}

func (l *Ledger) saveLocked() error {
	data, err := json.Marshal(ledgerFile{Version: ledgerVersion, Entries: l.entries})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".ledger-*")
	if err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write ledger: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync ledger: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), l.path)
}
