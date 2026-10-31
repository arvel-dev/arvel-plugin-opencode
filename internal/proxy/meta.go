// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"encoding/json"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

// childSessionKey is the v2 meta key that marks a subagent child update
// (FINDINGS V7).
const childSessionKey = "opencode/child-session"

// childSessionMeta is the v2 `_meta["opencode/child-session"]` shape.
type childSessionMeta struct {
	ID       string `json:"id"`
	ParentID string `json:"parentID"`
	Title    string `json:"title"`
	Depth    int    `json:"depth"`
}

// translateUpdateMeta derives the host `_meta.arvel` for an upstream
// session/update notification. v2 marks subagent updates with
// `_meta["opencode/child-session"]`; v1 has no such meta (FINDINGS V7).
func translateUpdateMeta(n acp.SessionNotification) *protocol.UpdateMeta {
	m := n.Update.AgentMessageChunk
	if m == nil {
		return nil
	}
	raw, ok := m.Meta[childSessionKey]
	if !ok {
		return nil
	}
	// Defensive decode: a malformed child meta is not an error, just no
	// subagent attribution.
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var c childSessionMeta
	if json.Unmarshal(b, &c) != nil || c.ID == "" {
		return nil
	}
	return &protocol.UpdateMeta{Subagent: &protocol.SubagentMeta{
		ID:               c.ID,
		ParentToolCallID: c.ParentID,
		Label:            c.Title,
	}}
}
