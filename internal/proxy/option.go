// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"fmt"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
)

// SetOption applies a config option from the next turn (PROTOCOL §7.5). The
// model and effort are translated per flavour; anything else is passed through
// unchanged. On an idle-closed session the value is stored and applied on
// resume.
func (s *Session) SetOption(ctx context.Context, id, value string) ([]acp.SessionConfigOption, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errClosed
	}
	switch id {
	case "model":
		s.model = value
	case "effort":
		s.effort = value
	}
	c := s.client
	model, effort := s.model, s.effort
	s.mu.Unlock()

	if c == nil {
		// Stored and applied on resume; keep reporting the last known options so
		// the host does not see an empty option set.
		s.mu.Lock()
		opts := s.opts
		s.mu.Unlock()
		return opts, nil
	}

	var configID, upstreamValue string
	switch id {
	case "model":
		configID, upstreamValue = "model", modelValue(s.cfg.Flavour, model, effort)
	case "effort":
		if s.cfg.Flavour == detect.V2 {
			configID, upstreamValue = "effort", effort
		} else {
			if model == "" {
				return nil, nil
			}
			configID, upstreamValue = "model", modelValue(detect.V1, model, effort)
		}
	default:
		configID, upstreamValue = id, value
	}

	opts, err := c.SetOption(ctx, acp.SessionId(s.id), configID, upstreamValue)
	if err != nil {
		return nil, fmt.Errorf("proxy: set %s: %w", id, err)
	}
	s.setOpts(opts)
	s.armIdle()
	return opts, nil
}
