// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"fmt"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/upstream"
)

// armIdle (re)starts the idle-close timer. It is called after New/Load and at
// the end of each turn and SetOption.
func (s *Session) armIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.idle != nil {
		s.idle.Stop()
	}
	s.idle = time.AfterFunc(idleAfter, func() {
		s.mu.Lock()
		if s.closed || s.turning {
			s.mu.Unlock()
			return // busy or already closed: re-arm on the next activity
		}
		c := s.client
		s.client = nil
		s.mu.Unlock()
		if c != nil {
			_ = c.Close()
		}
	})
}

// ensureClient returns the live child, respawning and resuming it if the idle
// timer closed it earlier. It marks the session as turning on success. The
// respawned child is tied to the session lifetime (baseCtx), never to a turn.
func (s *Session) ensureClient() (*upstream.Client, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errClosed
	}
	if s.client != nil {
		s.turning = true
		c := s.client
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	client, err := upstream.Start(s.baseCtx, upstream.Config{
		Bin:               s.cfg.Bin,
		CWD:               s.cwd,
		LoadProjectConfig: s.cfg.LoadProjectConfig,
		Proxy:             s.cfg.Proxy,
		Log:               s.cfg.Log,
		Redactor:          s.cfg.Redactor,
	})
	if err != nil {
		return nil, fmt.Errorf("proxy: resume start: %w", err)
	}
	res, err := client.LoadSession(s.baseCtx, acp.SessionId(s.id), s.cwd, s.mcp)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("proxy: resume load: %w", err)
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = client.Close()
		return nil, errClosed
	}
	s.client = client
	s.opts = res.ConfigOptions
	s.turning = true
	s.mu.Unlock()

	if err := s.applyInitial(s.baseCtx); err != nil {
		s.mu.Lock()
		s.turning = false
		if s.client == client {
			s.client = nil
		}
		s.mu.Unlock()
		_ = client.Close()
		return nil, err
	}
	return client, nil
}
