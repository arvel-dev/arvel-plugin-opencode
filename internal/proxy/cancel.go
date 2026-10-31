// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// Cancel asks the running turn to stop gracefully (PROTOCOL §7.6). It sends
// the upstream session/cancel notification and arms a watchdog that kills the
// child if the turn has not stopped within cancelAfter. Non-blocking and
// idempotent.
func (s *Session) Cancel() {
	s.mu.Lock()
	if s.closed || s.cancelling {
		s.mu.Unlock()
		return
	}
	s.cancelling = true
	c := s.client
	s.mu.Unlock()

	if c != nil {
		// A notify failure (child gone) is fine: the watchdog is harmless.
		_ = c.Cancel(acp.SessionId(s.id))
	}

	s.mu.Lock()
	if s.cancelTimer == nil {
		s.cancelTimer = time.AfterFunc(cancelAfter, func() {
			s.mu.Lock()
			k := s.client
			s.mu.Unlock()
			if k != nil {
				k.Kill(0)
			}
		})
	}
	s.mu.Unlock()
}
