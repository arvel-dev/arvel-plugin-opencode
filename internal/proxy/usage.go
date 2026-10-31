// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"errors"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

// perTurnUsage computes the per-turn TurnUsage and vendor currency from the
// prompt result and the cost captured during the turn. Tokens and cost are
// deltas against the previous turn in the same child (FINDINGS V6: OpenCode
// reports cumulative session usage), with a monotonic guard. It also updates
// the session baseline so the next turn computes its own delta.
func (s *Session) perTurnUsage(res acp.PromptResponse, sink *outputSink) (protocol.TurnUsage, string) {
	s.mu.Lock()
	prevInput, prevOutput, prevCache := s.lastInput, s.lastOutput, s.lastCacheRead
	prevCostAmount, prevCurrency, prevHas := s.lastCostAmount, s.lastCostCurrency, s.hasCost
	s.mu.Unlock()

	tu := protocol.TurnUsage{}
	if u := res.Usage; u != nil {
		tu.InputTokens = delta64(int64(u.InputTokens), prevInput)
		tu.OutputTokens = delta64(int64(u.OutputTokens), prevOutput)
		if u.CachedReadTokens != nil {
			tu.CacheReadTokens = delta64(int64(*u.CachedReadTokens), prevCache)
		}
	}

	currency := ""
	cost := sink.capturedCost()
	if cost != nil {
		currency = cost.Currency
		d := cost.Amount
		if prevHas && prevCurrency == cost.Currency {
			d = cost.Amount - prevCostAmount
		}
		if d < 0 {
			// The cumulative counter reset (e.g. a respawn): the current
			// amount is this turn's cost.
			d = cost.Amount
		}
		tu.CostUSD = d
		tu.CostSource = protocol.UsageSourceEstimated
	}

	s.mu.Lock()
	if res.Usage != nil {
		s.lastInput = int64(res.Usage.InputTokens)
		s.lastOutput = int64(res.Usage.OutputTokens)
		if res.Usage.CachedReadTokens != nil {
			s.lastCacheRead = int64(*res.Usage.CachedReadTokens)
		}
	}
	if cost != nil {
		s.lastCostAmount = cost.Amount
		s.lastCostCurrency = cost.Currency
		s.hasCost = true
	}
	s.mu.Unlock()

	return tu, currency
}

// delta64 returns cur-prev when cur >= prev, else cur (monotonic guard).
func delta64(cur, prev int64) int64 {
	if cur >= prev {
		return cur - prev
	}
	return cur
}

// tuEmpty reports whether a TurnUsage carries no information. A non-zero cost
// or any non-zero token count is information; the CostSource alone is not.
func tuEmpty(tu protocol.TurnUsage) bool {
	return tu.InputTokens == 0 && tu.OutputTokens == 0 && tu.CacheReadTokens == 0 &&
		tu.CostUSD == 0
}

// noteRateLimited marks the tracker exhausted after a rate_limited error.
func (s *Session) noteRateLimited() {
	if tr := s.cfg.Tracker; tr != nil {
		tr.RateLimited()
	}
}

// noteOK clears an exhausted status after a successful turn, even one that
// reported no usage (PROTOCOL §6.3).
func (s *Session) noteOK() {
	if tr := s.cfg.Tracker; tr != nil {
		tr.MarkOK()
	}
}

// isRateLimited reports whether err is an Arvel rate_limited error.
func isRateLimited(err error) bool {
	var re *protocol.RPCError
	if !errors.As(err, &re) {
		return false
	}
	d, ok := re.Arvel()
	return ok && d.Code == protocol.ErrRateLimited
}
