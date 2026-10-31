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
	mu        sync.Mutex
	updates   []acp.SessionUpdate
	metas     []*protocol.UpdateMeta
	decision  plugin.PermissionDecision
	permCalls int
}

func (r *recordingOutput) Update(u acp.SessionUpdate, meta *protocol.UpdateMeta) error {
	r.mu.Lock()
	r.updates = append(r.updates, u)
	r.metas = append(r.metas, meta)
	r.mu.Unlock()
	return nil
}

func (r *recordingOutput) RequestPermission(_ context.Context, _ plugin.PermissionRequest) (plugin.PermissionDecision, error) {
	r.mu.Lock()
	r.permCalls++
	r.mu.Unlock()
	return r.decision, nil
}

func (r *recordingOutput) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates)
}

// callCount returns how many times the host was asked for permission.
func (r *recordingOutput) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.permCalls
}

// lastText returns the text of the last agent_message_chunk update.
func (r *recordingOutput) lastText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.updates) - 1; i >= 0; i-- {
		if c := r.updates[i].AgentMessageChunk; c != nil && c.Content.Text != nil {
			return c.Content.Text.Text
		}
	}
	return ""
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

func TestNewWritesInstructions(t *testing.T) {
	dataDir := t.TempDir()
	s, err := New(context.Background(), Config{Bin: fakeAgentBin, Flavour: detect.V2, DataDir: dataDir}, plugin.SessionParams{
		CWD:  t.TempDir(),
		Meta: protocol.SessionMeta{Instructions: "Be concise", Language: "ru"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Close(c)
	})

	matches, err := filepath.Glob(filepath.Join(dataDir, "run", "*", "instructions.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("instructions matches = %v, err = %v", matches, err)
	}
	got, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	want := "Be concise\n\nReply to the user in ru.\n"
	if string(got) != want {
		t.Fatalf("instructions = %q, want %q", got, want)
	}
}

func TestNewMaterializesSkills(t *testing.T) {
	cwd := t.TempDir()
	src := t.TempDir()
	skillDir := filepath.Join(src, "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: alpha\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New(context.Background(), Config{Bin: fakeAgentBin, Flavour: detect.V2}, plugin.SessionParams{
		CWD: cwd,
		Meta: protocol.SessionMeta{Skills: []protocol.SkillRef{
			{Name: "alpha", Path: skillDir, Invocation: protocol.SkillInvocationAuto},
		}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	materialized := filepath.Join(cwd, ".opencode", "skills", "alpha", "SKILL.md")
	if _, err := os.Stat(materialized); err != nil {
		t.Fatalf("skill not materialized: %v", err)
	}

	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Close(c); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".opencode", "skills", "alpha")); !os.IsNotExist(err) {
		t.Fatalf("skill copy not removed after Close: %v", err)
	}
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
	if s.ID() == "" {
		t.Fatal("empty session id")
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
	id := s.ID()
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
	if s.ID() != id {
		t.Fatalf("id changed across resume: %q -> %q", id, s.ID())
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
	sink := &outputSink{
		out: &recordingOutput{decision: plugin.PermissionDecision{OptionID: "once"}},
		s:   newEvalSession(nil), // nil policy => ask => forward to host
	}
	resp := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{Kind: &kind},
		Options:  permissionOptions(),
	})
	if resp.Outcome.Selected == nil || string(resp.Outcome.Selected.OptionId) != "once" {
		t.Fatalf("unexpected outcome: %+v", resp.Outcome)
	}
}

func TestRequestPermission_Cancelled(t *testing.T) {
	sink := &outputSink{
		out: &recordingOutput{decision: plugin.PermissionDecision{Cancelled: true}},
		s:   newEvalSession(nil),
	}
	resp := sink.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		Options: permissionOptions(),
	})
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
