// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// fakeTracker records the usage and limit events a session reports.
type fakeTracker struct {
	mu         sync.Mutex
	adds       []protocol.TurnUsage
	currencies []string
	limited    int
	ok         int
}

func (f *fakeTracker) Add(tu protocol.TurnUsage, currency string) {
	f.mu.Lock()
	f.adds = append(f.adds, tu)
	f.currencies = append(f.currencies, currency)
	f.mu.Unlock()
}

func (f *fakeTracker) RateLimited() {
	f.mu.Lock()
	f.limited++
	f.mu.Unlock()
}

func (f *fakeTracker) MarkOK() {
	f.mu.Lock()
	f.ok++
	f.mu.Unlock()
}

func (f *fakeTracker) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.adds)
}

func (f *fakeTracker) limitedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.limited
}

func (f *fakeTracker) okCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ok
}

// newSessionWithConfig is newSession with a caller-supplied Config; it fills in
// the fake binary and default agent so tests only set what they care about.
func newSessionWithConfig(t *testing.T, cfg Config, model, effort string) *Session {
	t.Helper()
	if cfg.Bin == "" {
		cfg.Bin = fakeAgentBin
	}
	if cfg.Agent == "" {
		cfg.Agent = "build"
	}
	s, err := New(context.Background(), cfg, plugin.SessionParams{
		CWD:  t.TempDir(),
		Meta: protocol.SessionMeta{Model: model, Effort: protocol.Effort(effort)},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Close(c)
	})
	return s
}

func promptText(text string) []acp.ContentBlock {
	return []acp.ContentBlock{{Text: &acp.ContentBlockText{Type: "text", Text: text}}}
}

func TestPrompt_ReportsPerTurnUsage(t *testing.T) {
	ft := &fakeTracker{}
	s := newSessionWithConfig(t, Config{Flavour: detect.V2, Tracker: ft}, "a/b", "")
	out := &recordingOutput{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := s.Prompt(ctx, promptText("USAGE"), out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if res.Usage == nil {
		t.Fatal("res.Usage is nil, want per-turn usage")
	}
	if res.Usage.InputTokens != 100 || res.Usage.OutputTokens != 50 || res.Usage.CacheReadTokens != 25 {
		t.Fatalf("tokens = %+v, want 100/50/25", res.Usage)
	}
	if res.Usage.CostUSD != 0.42 || res.Usage.CostSource != protocol.UsageSourceEstimated {
		t.Fatalf("cost = %+v, want 0.42 estimated", res.Usage)
	}
	if ft.count() != 1 {
		t.Fatalf("tracker adds = %d, want 1", ft.count())
	}
	if ft.currencies[0] != "USD" {
		t.Fatalf("currency = %q, want USD", ft.currencies[0])
	}
}

func TestPrompt_NoUsageWhenAbsent(t *testing.T) {
	ft := &fakeTracker{}
	s := newSessionWithConfig(t, Config{Flavour: detect.V2, Tracker: ft}, "a/b", "")
	out := &recordingOutput{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := s.Prompt(ctx, promptText("hello"), out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if res.Usage != nil {
		t.Fatalf("res.Usage = %+v, want nil", res.Usage)
	}
	if ft.count() != 0 {
		t.Fatalf("tracker adds = %d, want 0", ft.count())
	}
}

func TestPrompt_DropsUsageUpdateFromStream(t *testing.T) {
	s := newSessionWithConfig(t, Config{Flavour: detect.V2}, "a/b", "")
	out := &recordingOutput{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := s.Prompt(ctx, promptText("USAGE"), out); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if out.count() != 1 {
		t.Fatalf("forwarded updates = %d, want 1 (usage_update dropped)", out.count())
	}
	if out.updates[0].UsageUpdate != nil {
		t.Fatal("usage_update leaked to the host stream")
	}
}

func TestPrompt_MultiTurnDeltas(t *testing.T) {
	ft := &fakeTracker{}
	s := newSessionWithConfig(t, Config{Flavour: detect.V2, Tracker: ft}, "a/b", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Turn 1: cumulative 100/50/25 tokens and 0.42 USD.
	r1, err := s.Prompt(ctx, promptText("USAGE"), &recordingOutput{})
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if r1.Usage == nil || r1.Usage.InputTokens != 100 || r1.Usage.CostUSD != 0.42 {
		t.Fatalf("turn 1 usage = %+v, want 100 tokens / 0.42", r1.Usage)
	}

	// Turn 2: cumulative 200/100/50 tokens and 0.84 USD; the delta is the
	// first turn's amount again, not the cumulative total.
	r2, err := s.Prompt(ctx, promptText("USAGE2"), &recordingOutput{})
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if r2.Usage == nil {
		t.Fatal("turn 2 usage is nil")
	}
	if r2.Usage.InputTokens != 100 || r2.Usage.OutputTokens != 50 || r2.Usage.CacheReadTokens != 25 {
		t.Fatalf("turn 2 tokens = %+v, want a 100/50/25 delta", r2.Usage)
	}
	if r2.Usage.CostUSD != 0.42 {
		t.Fatalf("turn 2 cost = %v, want a 0.42 delta", r2.Usage.CostUSD)
	}
	if ft.count() != 2 {
		t.Fatalf("tracker adds = %d, want 2", ft.count())
	}
}

func TestPrompt_ClearsExhaustedWithoutUsage(t *testing.T) {
	ft := &fakeTracker{}
	s := newSessionWithConfig(t, Config{Flavour: detect.V2, Tracker: ft}, "a/b", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.noteRateLimited()
	if _, err := s.Prompt(ctx, promptText("hello"), &recordingOutput{}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if ft.okCount() != 1 {
		t.Fatalf("MarkOK count = %d, want 1 after a usage-less successful turn", ft.okCount())
	}
}

func TestDelta64(t *testing.T) {
	cases := []struct {
		cur, prev, want int64
	}{
		{100, 0, 100},
		{200, 100, 100},
		{50, 100, 50}, // a counter reset: report the new value as-is
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := delta64(c.cur, c.prev); got != c.want {
			t.Errorf("delta64(%d, %d) = %d, want %d", c.cur, c.prev, got, c.want)
		}
	}
}

func TestTuEmpty(t *testing.T) {
	if !tuEmpty(protocol.TurnUsage{}) {
		t.Fatal("zero TurnUsage must be empty")
	}
	if !tuEmpty(protocol.TurnUsage{CostSource: protocol.UsageSourceEstimated}) {
		t.Fatal("a cost source alone must be empty")
	}
	if tuEmpty(protocol.TurnUsage{InputTokens: 1}) {
		t.Fatal("a non-zero token count must not be empty")
	}
	if tuEmpty(protocol.TurnUsage{CostUSD: 0.01}) {
		t.Fatal("a non-zero cost must not be empty")
	}
}

func TestPrompt_MapsRateLimited(t *testing.T) {
	ft := &fakeTracker{}
	s := newSessionWithConfig(t, Config{Flavour: detect.V2, Tracker: ft}, "a/b", "")
	out := &recordingOutput{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.Prompt(ctx, promptText("ERR:provider.free-limit"), out)
	if err == nil {
		t.Fatal("Prompt returned nil, want a rate_limited error")
	}
	var re *protocol.RPCError
	if !errors.As(err, &re) {
		t.Fatalf("error = %T, want *protocol.RPCError", err)
	}
	d, ok := re.Arvel()
	if !ok || d.Code != protocol.ErrRateLimited {
		t.Fatalf("arvel error = %+v, want rate_limited", d)
	}
	if ft.limitedCount() != 1 {
		t.Fatalf("rate-limited count = %d, want 1", ft.limitedCount())
	}
}
