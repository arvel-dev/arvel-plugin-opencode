// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

func TestTranslateUpdateMeta_Subagent(t *testing.T) {
	n := acp.SessionNotification{Update: acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
		Meta: map[string]any{childSessionKey: map[string]any{
			"id": "ses_child", "parentID": "ses_parent", "title": "Explore", "depth": 1,
		}},
	}}}
	meta := translateUpdateMeta(n)
	if meta == nil || meta.Subagent == nil {
		t.Fatal("want subagent meta")
	}
	if meta.Subagent.ID != "ses_child" || meta.Subagent.ParentToolCallID != "ses_parent" || meta.Subagent.Label != "Explore" {
		t.Fatalf("unexpected subagent meta: %+v", meta.Subagent)
	}
}

func TestTranslateUpdateMeta_NoMeta(t *testing.T) {
	n := acp.SessionNotification{Update: acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{}}}
	if translateUpdateMeta(n) != nil {
		t.Fatal("want nil meta")
	}
}

func TestTranslateUpdateMeta_Malformed(t *testing.T) {
	n := acp.SessionNotification{Update: acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
		Meta: map[string]any{childSessionKey: map[string]any{"parentID": "ses_parent"}},
	}}}
	if translateUpdateMeta(n) != nil {
		t.Fatal("missing id must yield nil meta")
	}
}

func TestCategoryFromKind(t *testing.T) {
	kind := func(k acp.ToolKind) *acp.ToolKind { return &k }
	cases := []struct {
		in   *acp.ToolKind
		want protocol.PermissionCategory
	}{
		{kind(acp.ToolKindExecute), protocol.PermissionCategoryCommand},
		{kind(acp.ToolKindEdit), protocol.PermissionCategoryEdit},
		{kind(acp.ToolKindDelete), protocol.PermissionCategoryEdit},
		{kind(acp.ToolKindMove), protocol.PermissionCategoryEdit},
		{kind(acp.ToolKindRead), protocol.PermissionCategoryRead},
		{kind(acp.ToolKindSearch), protocol.PermissionCategoryRead},
		{kind(acp.ToolKindFetch), protocol.PermissionCategoryNetwork},
		{kind(acp.ToolKindThink), protocol.PermissionCategoryOther},
		{nil, protocol.PermissionCategoryOther},
	}
	for _, c := range cases {
		if got := categoryFromKind(c.in); got != c.want {
			t.Fatalf("categoryFromKind(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
