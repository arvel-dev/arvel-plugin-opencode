// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// fakeAgentBin is the on-demand-built fake ACP agent shared by all proxy tests.
var fakeAgentBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeagent")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin := filepath.Join(dir, "fakeagent")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	root := repoRoot(m)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(root, "testdata", "fakeagent")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fakeagent: %v\n%s\n", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	fakeAgentBin = bin
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func repoRoot(m *testing.M) string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// recordingOutput records session/update calls and answers permissions.
type recordingOutput struct {
	mu       sync.Mutex
	updates  []acp.SessionUpdate
	metas    []*protocol.UpdateMeta
	decision plugin.PermissionDecision
}

func (r *recordingOutput) Update(u acp.SessionUpdate, meta *protocol.UpdateMeta) error {
	r.mu.Lock()
	r.updates = append(r.updates, u)
	r.metas = append(r.metas, meta)
	r.mu.Unlock()
	return nil
}

func (r *recordingOutput) RequestPermission(_ context.Context, _ plugin.PermissionRequest) (plugin.PermissionDecision, error) {
	return r.decision, nil
}

func (r *recordingOutput) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates)
}

func newSession(t *testing.T, flavour detect.Flavour, model, effort string) *Session {
	t.Helper()
	// The child's lifetime is tied to the context passed to upstream.Start, so
	// use a process-lifetime context here (as the SDK does for session/new).
	s, err := New(context.Background(), Config{Bin: fakeAgentBin, Flavour: flavour, Agent: "build"}, plugin.SessionParams{
		CWD:  t.TempDir(),
		Meta: protocol.SessionMeta{Model: model, Effort: protocol.Effort(effort)},
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

func currentValue(opts []acp.SessionConfigOption, id string) string {
	for _, o := range opts {
		if o.Select != nil && string(o.Select.Id) == id {
			return string(o.Select.CurrentValue)
		}
	}
	return ""
}

func TestNew_IDAndConfigOptions(t *testing.T) {
	s := newSession(t, detect.V2, "a/b", "high")
	if s.ID() != "ses_fake" {
		t.Fatalf("id = %q", s.ID())
	}
	opts := s.ConfigOptions()
	for _, id := range []string{"model", "effort", "mode"} {
		if !hasOption(opts, id) {
			t.Fatalf("missing config option %q", id)
		}
	}
}

func TestPrompt_StreamsUpdate(t *testing.T) {
	s := newSession(t, detect.V2, "a/b", "")
	out := &recordingOutput{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Prompt(ctx, []acp.ContentBlock{{Text: &acp.ContentBlockText{Type: "text", Text: "hello"}}}, out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if res.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("stop = %q", res.StopReason)
	}
	if out.count() != 1 {
		t.Fatalf("updates = %d", out.count())
	}
	chunk := out.updates[0].AgentMessageChunk
	if chunk == nil || chunk.Content.Text == nil || chunk.Content.Text.Text != "fake pong" {
		t.Fatalf("unexpected update")
	}
}

func TestSetOption_Model_V1(t *testing.T) {
	s := newSession(t, detect.V1, "a/b", "high")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts, err := s.SetOption(ctx, "model", "a/c")
	if err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if got := currentValue(opts, "model"); got != "a/c/high" {
		t.Fatalf("model = %q, want a/c/high", got)
	}
}

func TestSetOption_Model_V2(t *testing.T) {
	s := newSession(t, detect.V2, "a/b", "high")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts, err := s.SetOption(ctx, "model", "a/c")
	if err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if got := currentValue(opts, "model"); got != "a/c" {
		t.Fatalf("model = %q, want a/c", got)
	}
}

func TestSetOption_Effort_V1(t *testing.T) {
	s := newSession(t, detect.V1, "a/b", "high")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts, err := s.SetOption(ctx, "effort", "low")
	if err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if got := currentValue(opts, "model"); got != "a/b/low" {
		t.Fatalf("model = %q, want a/b/low", got)
	}
}

func TestSetOption_Effort_V2(t *testing.T) {
	s := newSession(t, detect.V2, "a/b", "high")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts, err := s.SetOption(ctx, "effort", "max")
	if err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if got := currentValue(opts, "effort"); got != "max" {
		t.Fatalf("effort = %q, want max", got)
	}
}

func TestIdleCloseAndResume(t *testing.T) {
	old := idleAfter
	idleAfter = 200 * time.Millisecond
	defer func() { idleAfter = old }()

	s := newSession(t, detect.V2, "a/b", "")
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		closed := s.client == nil
		s.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child was not idle-closed")
		}
		time.Sleep(20 * time.Millisecond)
	}

	out := &recordingOutput{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.Prompt(ctx, []acp.ContentBlock{{Text: &acp.ContentBlockText{Type: "text", Text: "after idle"}}}, out); err != nil {
		t.Fatalf("resume Prompt: %v", err)
	}
	if got := currentValue(s.ConfigOptions(), "model"); got != "LOADED" {
		t.Fatalf("resumed config = %q, want LOADED (session/load)", got)
	}
	if s.ID() != "ses_fake" {
		t.Fatalf("id changed across resume: %q", s.ID())
	}
}

func TestCancel_KillsStuckChild(t *testing.T) {
	old := cancelAfter
	cancelAfter = 50 * time.Millisecond
	defer func() { cancelAfter = old }()

	s := newSession(t, detect.V2, "a/b", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := s.Prompt(ctx, []acp.ContentBlock{{Text: &acp.ContentBlockText{Type: "text", Text: "please HANG"}}}, &recordingOutput{})
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	s.Cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error after cancel killed the child")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled prompt did not unblock")
	}
}

func TestRequestPermission_Selected(t *testing.T) {
	kind := acp.ToolKindExecute
	sink := &outputSink{out: &recordingOutput{decision: plugin.PermissionDecision{OptionID: "allow_once"}}}
	resp := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{Kind: &kind},
	})
	if resp.Outcome.Selected == nil || string(resp.Outcome.Selected.OptionId) != "allow_once" {
		t.Fatalf("unexpected outcome: %+v", resp.Outcome)
	}
}

func TestRequestPermission_Cancelled(t *testing.T) {
	sink := &outputSink{out: &recordingOutput{decision: plugin.PermissionDecision{Cancelled: true}}}
	resp := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{})
	if resp.Outcome.Cancelled == nil {
		t.Fatalf("want cancelled outcome, got %+v", resp.Outcome)
	}
}

func TestPrompt_ClosedSession(t *testing.T) {
	s := newSession(t, detect.V2, "a/b", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.Prompt(ctx, nil, &recordingOutput{}); err == nil {
		t.Fatal("Prompt on a closed session must fail")
	}
}

func TestCancel_Idempotent(t *testing.T) {
	s := newSession(t, detect.V2, "a/b", "")
	// Neither call may block or panic; the second is ignored.
	s.Cancel()
	s.Cancel()
}
