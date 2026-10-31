// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// recSink records updates and answers permission requests with a fixed outcome.
type recSink struct {
	mu      sync.Mutex
	updates []acp.SessionNotification
	outcome acp.RequestPermissionResponse
}

func (r *recSink) Update(n acp.SessionNotification) {
	r.mu.Lock()
	r.updates = append(r.updates, n)
	r.mu.Unlock()
}

func (r *recSink) RequestPermission(_ context.Context, _ acp.RequestPermissionRequest) acp.RequestPermissionResponse {
	return r.outcome
}

func (r *recSink) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates)
}

func TestFakeAgent_Handshake(t *testing.T) {
	_, c := newTestClient(t)
	if c.Done() != nil {
		t.Fatal("pipe client must report a nil Done channel")
	}
}

func TestClient_NewPromptLoadSetCancel(t *testing.T) {
	agent, c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.NewSession(ctx, "/tmp/work", nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if string(res.SessionId) != "ses_fake" {
		t.Fatalf("session id = %q", res.SessionId)
	}

	sink := &recSink{outcome: acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected("allow_once")}}
	pr, err := c.Prompt(ctx, res.SessionId, []acp.ContentBlock{{Text: &acp.ContentBlockText{Type: "text", Text: "hi"}}}, sink)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if pr.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("stop = %q", pr.StopReason)
	}
	if sink.count() != 1 {
		t.Fatalf("updates = %d, want 1", sink.count())
	}
	chunk := sink.updates[0].Update.AgentMessageChunk
	if chunk == nil || chunk.Content.Text == nil || chunk.Content.Text.Text != "fake pong" {
		t.Fatalf("unexpected update: %+v", sink.updates[0])
	}

	if _, err := c.SetOption(ctx, res.SessionId, "model", "a/c"); err != nil {
		t.Fatalf("set option: %v", err)
	}
	if id, val := agent.observedConfig(); id != "model" || val != "a/c" {
		t.Fatalf("config = %q=%q", id, val)
	}

	lr, err := c.LoadSession(ctx, res.SessionId, "/tmp/work", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(lr.ConfigOptions) != len(agent.configOptions) {
		t.Fatalf("load config options = %d", len(lr.ConfigOptions))
	}
	if agent.observedLoad() != "ses_fake" {
		t.Fatalf("load id = %q", agent.observedLoad())
	}

	if err := c.Cancel(res.SessionId); err != nil {
		t.Fatalf("cancel: %v", err)
	}
}

func TestClient_StaleUpdateDropped(t *testing.T) {
	agent, _ := newTestClient(t)
	// No prompt is running: the client must drop the update without panicking.
	agent.sendUpdate("ses_fake", "stale")
	time.Sleep(50 * time.Millisecond)
}

func TestHandshake_UnsupportedVersion(t *testing.T) {
	agent := &fakeAgent{protocolVersion: 2, sessionID: "ses_fake"}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	agent.w = outW
	agent.sc = newScanner(inR)
	go agent.loop()
	c := newPipeClient(Config{Log: slog.New(slog.DiscardHandler)}, outR, inW, inW.Close, outW.Close)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.handshake(ctx); err == nil {
		t.Fatal("want unsupported protocol version error")
	}
}

func TestTeardown_UnblocksCall(t *testing.T) {
	agent, c := newTestClient(t)
	agent.noPromptResp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.NewSession(ctx, "/tmp/work", nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := c.Prompt(ctx, res.SessionId, nil, &recSink{})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	c.Kill(0)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error after kill")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not unblock after kill")
	}
}

// TestStart_Handshake exercises the real procx spawn path against an installed
// opencode. It is opt-in (ARVEL_E2E=1) per .ai-factory/RULES.md.
func TestStart_Handshake(t *testing.T) {
	if os.Getenv("ARVEL_E2E") != "1" {
		t.Skip("real opencode spawn: set ARVEL_E2E=1")
	}
	bin, err := exec.LookPath("opencode")
	if err != nil {
		t.Skip("opencode not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := Start(ctx, Config{Bin: bin})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = c.Close()
}
