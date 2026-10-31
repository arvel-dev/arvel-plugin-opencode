// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// recordingHost is a plugin.Output that counts permission round-trips.
type recordingHost struct {
	calls  int
	meta   protocol.PermissionMeta
	answer plugin.PermissionDecision
}

func (r *recordingHost) Update(acp.SessionUpdate, *protocol.UpdateMeta) error { return nil }
func (r *recordingHost) RequestPermission(_ context.Context, req plugin.PermissionRequest) (plugin.PermissionDecision, error) {
	r.calls++
	r.meta = req.Meta
	return r.answer, nil
}

func permissionOptions() []acp.PermissionOption {
	return []acp.PermissionOption{
		{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow once", OptionId: "once"},
		{Kind: acp.PermissionOptionKindAllowAlways, Name: "Always allow", OptionId: "always"},
		{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: "reject"},
	}
}

func outcomeID(r acp.RequestPermissionResponse) string {
	if r.Outcome.Selected != nil {
		return string(r.Outcome.Selected.OptionId)
	}
	return ""
}

func execTool(cmd string) acp.ToolCallUpdate {
	k := acp.ToolKindExecute
	return acp.ToolCallUpdate{ToolCallId: "t1", Kind: &k, RawInput: map[string]any{"command": cmd}}
}

func editTool(path string) acp.ToolCallUpdate {
	k := acp.ToolKindEdit
	return acp.ToolCallUpdate{ToolCallId: "t2", Kind: &k, Locations: []acp.ToolCallLocation{{Path: path}}}
}

func newEvalSession(p *protocol.Permissions) *Session {
	return &Session{meta: protocol.SessionMeta{Permissions: p}}
}

func TestEvaluatorDenyWithoutHost(t *testing.T) {
	s := newEvalSession(&protocol.Permissions{Commands: &protocol.CommandPolicy{
		Default: protocol.PermissionLevelAsk,
		Deny:    []string{"git push *"},
	}})
	host := &recordingHost{}
	sink := &outputSink{out: host, s: s}

	got := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{ToolCall: execTool("git push origin main"), Options: permissionOptions()})
	if outcomeID(got) != "reject" {
		t.Fatalf("outcome = %q, want reject", outcomeID(got))
	}
	if host.calls != 0 {
		t.Fatalf("host calls = %d, want 0 (deny must not round-trip)", host.calls)
	}
}

func TestEvaluatorAllowWithoutHost(t *testing.T) {
	s := newEvalSession(&protocol.Permissions{Commands: &protocol.CommandPolicy{
		Default: protocol.PermissionLevelAsk,
		Allow:   []string{"go test *"},
	}})
	host := &recordingHost{}
	sink := &outputSink{out: host, s: s}

	got := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{ToolCall: execTool("go test ./..."), Options: permissionOptions()})
	if outcomeID(got) != "once" {
		t.Fatalf("outcome = %q, want once", outcomeID(got))
	}
	if host.calls != 0 {
		t.Fatalf("host calls = %d, want 0 (allow must not round-trip)", host.calls)
	}
}

func TestEvaluatorAskForwardsMeta(t *testing.T) {
	s := newEvalSession(&protocol.Permissions{Commands: &protocol.CommandPolicy{Default: protocol.PermissionLevelAsk}})
	host := &recordingHost{answer: plugin.PermissionDecision{OptionID: "once"}}
	sink := &outputSink{out: host, s: s}

	got := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{ToolCall: execTool("ls -la"), Options: permissionOptions()})
	if outcomeID(got) != "once" {
		t.Fatalf("outcome = %q, want once", outcomeID(got))
	}
	if host.calls != 1 {
		t.Fatalf("host calls = %d, want 1", host.calls)
	}
	if host.meta.Category != protocol.PermissionCategoryCommand || host.meta.Command != "ls -la" {
		t.Fatalf("meta = %+v", host.meta)
	}
}

func TestEvaluatorAlwaysIsSessionLocal(t *testing.T) {
	s := newEvalSession(&protocol.Permissions{Commands: &protocol.CommandPolicy{Default: protocol.PermissionLevelAsk}})
	host := &recordingHost{answer: plugin.PermissionDecision{OptionID: "always"}}
	sink := &outputSink{out: host, s: s}
	req := acp.RequestPermissionRequest{ToolCall: execTool("make deploy"), Options: permissionOptions()}

	if got := sink.RequestPermission(context.Background(), req); outcomeID(got) != "once" {
		t.Fatalf("first outcome = %q, want once (never forward always)", outcomeID(got))
	}
	if host.calls != 1 {
		t.Fatalf("host calls = %d, want 1", host.calls)
	}
	if got := sink.RequestPermission(context.Background(), req); outcomeID(got) != "once" {
		t.Fatalf("second outcome = %q, want once", outcomeID(got))
	}
	if host.calls != 1 {
		t.Fatalf("host calls = %d, want 1 (second request is session-local)", host.calls)
	}
}

func TestEvaluatorEditDeny(t *testing.T) {
	s := newEvalSession(&protocol.Permissions{Edit: protocol.PermissionLevelDeny})
	host := &recordingHost{}
	sink := &outputSink{out: host, s: s}

	got := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{ToolCall: editTool("/x/y.go"), Options: permissionOptions()})
	if outcomeID(got) != "reject" {
		t.Fatalf("outcome = %q, want reject", outcomeID(got))
	}
	if host.calls != 0 {
		t.Fatalf("host calls = %d, want 0", host.calls)
	}
}

func TestAllowKeyPathScoped(t *testing.T) {
	a := allowKey(protocol.PermissionCategoryEdit, editTool("/a/x.go"))
	b := allowKey(protocol.PermissionCategoryEdit, editTool("/b/y.go"))
	if a == b {
		t.Fatalf("edit allow-keys must differ by path, both %q", a)
	}
}

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "", true},
		{"*", "anything at all", true},
		{"go test *", "go test ./...", true},
		{"go test *", "go build", false},
		{"git push *", "git push origin", true},
		{"git push *", "git pull", false},
		{"rm ?", "rm x", true},
		{"rm ?", "rm xy", false},
	}
	for _, c := range cases {
		if got := matchPattern(c.pattern, c.s); got != c.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}
