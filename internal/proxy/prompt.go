// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
	"github.com/arvel-dev/arvel-sdk/redact"
)

// Prompt runs one turn: ensure the child is alive, install the output sink,
// call upstream session/prompt, stream updates to the host and return the
// upstream stop reason (SPEC §7, PROTOCOL §7.2).
func (s *Session) Prompt(ctx context.Context, blocks []acp.ContentBlock, out plugin.Output) (plugin.TurnResult, error) {
	c, err := s.ensureClient()
	if err != nil {
		return plugin.TurnResult{}, err
	}
	defer func() {
		s.mu.Lock()
		s.turning = false
		if s.cancelTimer != nil {
			s.cancelTimer.Stop()
			s.cancelTimer = nil
		}
		s.cancelling = false
		s.mu.Unlock()
		s.armIdle()
	}()

	sink := &outputSink{out: out, log: s.cfg.Log, rd: s.cfg.Redactor, s: s}
	res, err := c.Prompt(ctx, acp.SessionId(s.id), blocks, sink)
	if err != nil {
		if isRateLimited(err) {
			s.noteRateLimited()
		}
		return plugin.TurnResult{}, fmt.Errorf("proxy: prompt: %w", err)
	}

	tu, currency := s.perTurnUsage(res, sink)
	var usage *protocol.TurnUsage
	if tuEmpty(tu) {
		// A successful turn with no usage still clears a prior exhausted status
		// (PROTOCOL §6.3); with usage, Add clears it.
		s.noteOK()
	} else {
		usage = &tu
		if tr := s.cfg.Tracker; tr != nil {
			tr.Add(tu, currency)
		}
	}
	return plugin.TurnResult{StopReason: res.StopReason, Usage: usage}, nil
}

// outputSink adapts the host Output to the upstream Sink.
type outputSink struct {
	out plugin.Output
	log *slog.Logger
	rd  *redact.Redactor
	s   *Session // the session whose policy the evaluator uses

	mu       sync.Mutex
	lastCost *acp.Cost // latest cumulative session cost from usage_update
}

func (o *outputSink) Update(n acp.SessionNotification) {
	if u := n.Update.UsageUpdate; u != nil {
		if u.Cost != nil {
			o.mu.Lock()
			o.lastCost = u.Cost
			o.mu.Unlock()
		}
		if o.log != nil {
			o.log.Debug("proxy: captured usage_update")
		}
		return // usage_update is OpenCode-specific: captured, not forwarded (SPEC §8)
	}
	if err := o.out.Update(n.Update, translateUpdateMeta(n)); err != nil && o.log != nil {
		o.log.Warn("proxy: send update", "err", o.redact(err.Error()))
	}
}

// capturedCost returns the latest cumulative cost seen during the turn.
func (o *outputSink) capturedCost() *acp.Cost {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastCost
}

// redact runs external text through the configured redactor, if any.
func (o *outputSink) redact(s string) string {
	if o.rd == nil {
		return s
	}
	return o.rd.String(s)
}
