// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"fmt"
	"log/slog"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/plugin"
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
		return plugin.TurnResult{}, fmt.Errorf("proxy: prompt: %w", err)
	}
	return plugin.TurnResult{StopReason: res.StopReason}, nil
}

// outputSink adapts the host Output to the upstream Sink.
type outputSink struct {
	out plugin.Output
	log *slog.Logger
	rd  *redact.Redactor
	s   *Session // the session whose policy the evaluator uses
}

func (o *outputSink) Update(n acp.SessionNotification) {
	if err := o.out.Update(n.Update, translateUpdateMeta(n)); err != nil && o.log != nil {
		o.log.Warn("proxy: send update", "err", o.redact(err.Error()))
	}
}

// redact runs external text through the configured redactor, if any.
func (o *outputSink) redact(s string) string {
	if o.rd == nil {
		return s
	}
	return o.rd.String(s)
}
