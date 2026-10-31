// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

func newSessionWithPolicy(t *testing.T, f detect.Flavour, p *protocol.Permissions) *Session {
	t.Helper()
	s, err := New(context.Background(), Config{Bin: fakeAgentBin, Flavour: f, Agent: "build"}, plugin.SessionParams{
		CWD:  t.TempDir(),
		Meta: protocol.SessionMeta{Permissions: p},
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

func runPermissionPrompt(t *testing.T, s *Session, out *recordingOutput, cmd string) {
	t.Helper()
	_, err := s.Prompt(context.Background(), []acp.ContentBlock{{Text: &acp.ContentBlockText{Type: "text", Text: "PERM:" + cmd}}}, out)
	if err != nil {
		t.Fatalf("Prompt(%q): %v", cmd, err)
	}
}

// TestPermissionMatrixEndToEnd drives the full pipeline (fake vendor -> proxy
// evaluator -> host) for both flavours.
func TestPermissionMatrixEndToEnd(t *testing.T) {
	policy := func() *protocol.Permissions {
		return &protocol.Permissions{Commands: &protocol.CommandPolicy{
			Default: protocol.PermissionLevelAsk,
			Allow:   []string{"go test *"},
			Deny:    []string{"git push *"},
		}}
	}
	for _, f := range []detect.Flavour{detect.V1, detect.V2} {
		t.Run(string(f), func(t *testing.T) {
			s := newSessionWithPolicy(t, f, policy())
			out := &recordingOutput{decision: plugin.PermissionDecision{OptionID: "once"}}

			runPermissionPrompt(t, s, out, "git push origin")
			if got := out.lastText(); got != "perm:reject" {
				t.Fatalf("deny outcome = %q, want perm:reject", got)
			}
			if out.callCount() != 0 {
				t.Fatalf("deny host calls = %d, want 0", out.callCount())
			}

			runPermissionPrompt(t, s, out, "go test ./...")
			if got := out.lastText(); got != "perm:once" {
				t.Fatalf("allow outcome = %q, want perm:once", got)
			}
			if out.callCount() != 0 {
				t.Fatalf("allow host calls = %d, want 0", out.callCount())
			}

			runPermissionPrompt(t, s, out, "ls -la")
			if got := out.lastText(); got != "perm:once" {
				t.Fatalf("ask outcome = %q, want perm:once", got)
			}
			if out.callCount() != 1 {
				t.Fatalf("ask host calls = %d, want 1", out.callCount())
			}
		})
	}
}
