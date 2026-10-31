// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// fakeAgent is a minimal in-process ACP agent over a pipe pair. It answers
// initialize/session/* with canned responses and records what the client sent.
type fakeAgent struct {
	w  io.Writer
	sc *bufio.Scanner
	mu sync.Mutex

	protocolVersion int
	sessionID       string
	configOptions   []acp.SessionConfigOption

	// observations
	lastConfigID string
	lastValue    string
	loadedID     string
	permSeen     int
	// behaviour hooks
	promptDelay  time.Duration
	noPromptResp bool
	onUpdate     func(sessionID string)
}

type rpcMsg struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// newTestClient wires a fake agent to a Client over two pipes and completes
// the initialize handshake. The client and agent share one lifecycle.
func newTestClient(t *testing.T) (*fakeAgent, *Client) {
	t.Helper()
	a := &fakeAgent{
		protocolVersion: 1,
		sessionID:       "ses_fake",
		configOptions:   testConfigOptions(),
	}
	inR, inW := io.Pipe()   // client -> agent
	outR, outW := io.Pipe() // agent -> client
	a.w = outW
	a.sc = newScanner(inR)
	go a.loop()

	c := newPipeClient(Config{}, outR, inW, inW.Close, outW.Close)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.handshake(ctx); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	return a, c
}

// newScanner returns a scanner with a 64 KiB line budget for ACP frames.
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
	return sc
}

// testConfigOptions returns model/effort/mode selects shaped like v2-session-new.
func testConfigOptions() []acp.SessionConfigOption {
	sel := func(id, current string, values ...string) acp.SessionConfigOption {
		opts := make(acp.SessionConfigSelectOptionsUngrouped, 0, len(values))
		for _, v := range values {
			opts = append(opts, acp.SessionConfigSelectOption{Name: v, Value: acp.SessionConfigValueId(v)})
		}
		return acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
			Id:           acp.SessionConfigId(id),
			Name:         id,
			Type:         "select",
			CurrentValue: acp.SessionConfigValueId(current),
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &opts},
		}}
	}
	return []acp.SessionConfigOption{
		sel("model", "a/b", "a/b", "a/c"),
		sel("effort", "default", "low", "medium", "high", "default"),
		sel("mode", "build", "build", "plan"),
	}
}

func (a *fakeAgent) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	a.mu.Lock()
	_, _ = a.w.Write(append(b, '\n'))
	a.mu.Unlock()
}

func (a *fakeAgent) result(id json.RawMessage, res any) {
	a.send(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, res})
}

func (a *fakeAgent) sendUpdate(sessionID, text string) {
	a.send(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", "session/update", acp.SessionNotification{
		SessionId: acp.SessionId(sessionID),
		Update: acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.ContentBlock{Text: &acp.ContentBlockText{Type: "text", Text: text}},
		}},
	}})
}

func (a *fakeAgent) loop() {
	for a.sc.Scan() {
		var m rpcMsg
		if json.Unmarshal(a.sc.Bytes(), &m) != nil {
			continue
		}
		a.answer(m)
	}
}

func (a *fakeAgent) answer(m rpcMsg) {
	switch m.Method {
	case "initialize":
		a.result(m.ID, acp.InitializeResponse{
			ProtocolVersion:   acp.ProtocolVersion(a.protocolVersion),
			AgentCapabilities: acp.AgentCapabilities{LoadSession: true},
			AgentInfo:         &acp.Implementation{Name: "fake-opencode", Version: "0.0.0"},
		})
	case "session/new":
		a.result(m.ID, acp.NewSessionResponse{
			SessionId:     acp.SessionId(a.sessionID),
			ConfigOptions: a.configOptions,
		})
	case "session/load":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(m.Params, &p)
		a.mu.Lock()
		a.loadedID = p.SessionID
		a.mu.Unlock()
		a.result(m.ID, acp.LoadSessionResponse{ConfigOptions: a.configOptions})
	case "session/set_config_option":
		var p struct {
			ConfigID string `json:"configId"`
			Value    string `json:"value"`
		}
		_ = json.Unmarshal(m.Params, &p)
		a.mu.Lock()
		a.lastConfigID = p.ConfigID
		a.lastValue = p.Value
		a.mu.Unlock()
		a.result(m.ID, acp.SetSessionConfigOptionResponse{ConfigOptions: a.configOptions})
	case "session/prompt":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(m.Params, &p)
		if a.promptDelay > 0 {
			time.Sleep(a.promptDelay)
		}
		if a.onUpdate != nil {
			a.onUpdate(p.SessionID)
		}
		a.sendUpdate(p.SessionID, "fake pong")
		if a.noPromptResp {
			return
		}
		a.result(m.ID, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	case "session/request_permission":
		a.mu.Lock()
		a.permSeen++
		a.mu.Unlock()
		a.result(m.ID, acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected("allow_once")})
	}
	// notifications (session/cancel, notifications/initialized) have no id and
	// are intentionally not answered.
}

// observed returns a snapshot of the recorded config option.
func (a *fakeAgent) observedConfig() (id, value string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastConfigID, a.lastValue
}

// observedLoad returns the session id sent in session/load.
func (a *fakeAgent) observedLoad() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loadedID
}
