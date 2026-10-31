// SPDX-License-Identifier: Apache-2.0

package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

func TestTracker_SnapshotStartsUnknown(t *testing.T) {
	tr := New("")
	u := tr.Snapshot()
	if u.Source != protocol.UsageSourceUnknown {
		t.Fatalf("source = %q, want unknown", u.Source)
	}
	if u.Status != protocol.UsageStatusOK {
		t.Fatalf("status = %q, want ok", u.Status)
	}
	if u.Spend != nil {
		t.Fatalf("spend = %+v, want nil before any turn", u.Spend)
	}
	if u.Windows != nil {
		t.Fatalf("windows = %+v, want nil", u.Windows)
	}
}

func TestTracker_AddAccumulatesSpend(t *testing.T) {
	tr := New("")
	if err := tr.Add(protocol.TurnUsage{CostUSD: 0.40, CostSource: protocol.UsageSourceEstimated}, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Add(protocol.TurnUsage{CostUSD: 0.02, CostSource: protocol.UsageSourceEstimated}, "USD"); err != nil {
		t.Fatal(err)
	}
	u := tr.Snapshot()
	if u.Source != protocol.UsageSourceEstimated {
		t.Fatalf("source = %q, want estimated", u.Source)
	}
	if u.Spend == nil {
		t.Fatal("spend is nil, want a value")
	}
	if u.Spend.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", u.Spend.Currency)
	}
	if diff(u.Spend.SinceStart, 0.42) > 1e-9 {
		t.Fatalf("sinceStart = %v, want 0.42", u.Spend.SinceStart)
	}
	if u.Spend.Source != protocol.UsageSourceEstimated {
		t.Fatalf("spend.source = %q, want estimated", u.Spend.Source)
	}
}

func TestTracker_AddEmptyIsNoOp(t *testing.T) {
	tr := New("")
	if err := tr.Add(protocol.TurnUsage{CostSource: protocol.UsageSourceEstimated}, "USD"); err != nil {
		t.Fatal(err)
	}
	u := tr.Snapshot()
	if u.Source != protocol.UsageSourceUnknown || u.Spend != nil {
		t.Fatalf("empty add recorded data: %+v", u)
	}
}

func TestTracker_LedgerWrittenPerDay(t *testing.T) {
	dir := t.TempDir()
	tr := New(dir)
	day := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return day }

	if err := tr.Add(protocol.TurnUsage{
		InputTokens: 100, OutputTokens: 20, CacheReadTokens: 5, CostUSD: 0.5,
		CostSource: protocol.UsageSourceEstimated,
	}, "USD"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "usage", "2026-10-31.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	var l ledger
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	if l.Date != "2026-10-31" || l.SpendUSD != 0.5 || l.Turns != 1 ||
		l.InputTokens != 100 || l.OutputTokens != 20 || l.CacheReadTokens != 5 {
		t.Fatalf("ledger = %+v", l)
	}
}

func TestTracker_LedgerAccumulatesSameDay(t *testing.T) {
	dir := t.TempDir()
	tr := New(dir)
	day := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return day }

	for i := 0; i < 3; i++ {
		if err := tr.Add(protocol.TurnUsage{InputTokens: 10, CostUSD: 0.1}, "USD"); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "usage", "2026-10-31.json"))
	if err != nil {
		t.Fatal(err)
	}
	var l ledger
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatal(err)
	}
	if l.Turns != 3 || l.InputTokens != 30 || l.SpendUSD < 0.29 || l.SpendUSD > 0.31 {
		t.Fatalf("ledger = %+v, want 3 turns, 30 tokens, ~0.3 spend", l)
	}
}

func TestTracker_MarkRateLimitedSetsExhausted(t *testing.T) {
	tr := New("")
	tr.MarkRateLimited()
	if u := tr.Snapshot(); u.Status != protocol.UsageStatusExhausted {
		t.Fatalf("status = %q, want exhausted", u.Status)
	}
}

func TestTracker_AddClearsExhausted(t *testing.T) {
	tr := New("")
	tr.MarkRateLimited()
	if err := tr.Add(protocol.TurnUsage{CostUSD: 0.01}, "USD"); err != nil {
		t.Fatal(err)
	}
	if u := tr.Snapshot(); u.Status != protocol.UsageStatusOK {
		t.Fatalf("status = %q, want ok after a successful turn", u.Status)
	}
}

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func TestTracker_ConcurrentAdd(t *testing.T) {
	tr := New("")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tr.Add(protocol.TurnUsage{InputTokens: 1, CostUSD: 0.01}, "USD")
		}()
	}
	wg.Wait()
	u := tr.Snapshot()
	if u.Spend == nil || u.Spend.SinceStart < 0.19 || u.Spend.SinceStart > 0.21 {
		t.Fatalf("sinceStart = %+v, want ~0.20", u.Spend)
	}
}
