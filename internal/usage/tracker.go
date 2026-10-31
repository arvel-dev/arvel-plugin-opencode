// SPDX-License-Identifier: Apache-2.0

// Package usage aggregates best-effort spend from OpenCode's upstream usage
// stream (SPEC §8, FINDINGS V6). A Tracker accumulates spend.sinceStart in
// memory and persists a per-day ledger file; the provider reads Snapshot for
// _arvel/usage.
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

// dayFormat is the ledger filename per UTC day (deterministic for tests).
const dayFormat = "2006-01-02"

// ledger is the on-disk per-day record.
type ledger struct {
	Date            string  `json:"date"`
	Currency        string  `json:"currency"`
	SpendUSD        float64 `json:"spendUsd"`
	Turns           int     `json:"turns"`
	InputTokens     int64   `json:"inputTokens"`
	OutputTokens    int64   `json:"outputTokens"`
	CacheReadTokens int64   `json:"cacheReadTokens"`
}

// Tracker aggregates usage for one plugin process. It is safe for concurrent
// use: every session goroutine may call Add.
type Tracker struct {
	mu         sync.Mutex
	dir        string // <DataDir>/usage; "" disables the ledger
	now        func() time.Time
	currency   string
	sinceStart float64
	status     protocol.UsageStatus
	updatedAt  time.Time
	hasData    bool
}

// New returns a tracker that writes its daily ledger under dataDir/usage.
// An empty dataDir disables the ledger (in-memory only).
func New(dataDir string) *Tracker {
	dir := ""
	if dataDir != "" {
		dir = filepath.Join(dataDir, "usage")
	}
	return &Tracker{dir: dir, now: time.Now, status: protocol.UsageStatusOK}
}

// Add records one finished turn. currency is the vendor-reported currency
// ("" falls back to "USD"). A turn with no tokens and no cost is a no-op (the
// CostSource alone is not enough to record: it is a modifier, not a trigger).
// Add returns the ledger write error (nil when the ledger is disabled); a
// ledger failure never fails the turn.
func (t *Tracker) Add(tu protocol.TurnUsage, currency string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tu.InputTokens == 0 && tu.OutputTokens == 0 && tu.CacheReadTokens == 0 &&
		tu.CostUSD == 0 {
		return nil
	}
	if currency == "" {
		currency = "USD"
	}
	if t.currency == "" {
		t.currency = currency
	}
	t.sinceStart += tu.CostUSD
	t.status = protocol.UsageStatusOK // a successful turn clears a prior limit
	t.updatedAt = t.now()
	t.hasData = true
	if t.dir == "" {
		return nil
	}
	return t.writeLedger(tu, currency)
}

// MarkRateLimited flips the aggregate status to exhausted (PROTOCOL §6.3:
// "exhausted when the vendor rejects requests").
func (t *Tracker) MarkRateLimited() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status = protocol.UsageStatusExhausted
	t.updatedAt = t.now()
}

// MarkOK clears an exhausted status without recording spend.
func (t *Tracker) MarkOK() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status = protocol.UsageStatusOK
	t.updatedAt = t.now()
}

// Snapshot returns the current _arvel/usage body. Windows are always empty
// (OpenCode has no subscription quota windows, SPEC §8). Spend is reported as
// estimated (OpenCode cost comes from the vendor's own accounting, SPEC §8).
func (t *Tracker) Snapshot() protocol.Usage {
	t.mu.Lock()
	defer t.mu.Unlock()
	u := protocol.Usage{
		Source:    protocol.UsageSourceUnknown,
		Status:    t.status,
		UpdatedAt: t.updatedAt,
	}
	if t.hasData {
		u.Source = protocol.UsageSourceEstimated
		cur := t.currency
		if cur == "" {
			cur = "USD"
		}
		u.Spend = &protocol.Spend{
			Currency:   cur,
			SinceStart: t.sinceStart,
			Source:     protocol.UsageSourceEstimated,
		}
	}
	return u
}

// writeLedger loads today's ledger (if any), folds in the turn, and writes it
// back atomically. Best effort: it must not lose an existing file on error.
func (t *Tracker) writeLedger(tu protocol.TurnUsage, currency string) error {
	day := t.now().UTC().Format(dayFormat)
	path := filepath.Join(t.dir, day+".json")
	l := ledger{Date: day, Currency: currency}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &l) // ignore a malformed file; start fresh
	}
	if l.Currency == "" {
		l.Currency = currency
	}
	l.SpendUSD += tu.CostUSD
	l.Turns++
	l.InputTokens += tu.InputTokens
	l.OutputTokens += tu.OutputTokens
	l.CacheReadTokens += tu.CacheReadTokens
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(t.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(t.dir, day+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
