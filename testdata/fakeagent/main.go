// SPDX-License-Identifier: Apache-2.0

// Command fakeagent is a minimal ACP agent used by the plugin's tests in place
// of `opencode acp`. It is built on demand by the tests and is never shipped.
// A prompt whose text contains "HANG" is deliberately left unanswered so the
// cancel/kill path can be exercised. A prompt whose text starts with "PERM:"
// makes the agent ask the client for permission to run the given command; the
// chosen option id is echoed back as "perm:<optionId>" in the turn's text.
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
	w := bufio.NewWriter(os.Stdout)
	state := map[string]string{"model": "a/b", "effort": "default", "mode": "build"}

	var nextReqID int64 = 1000
	var pendingPromptID json.RawMessage
	var pendingSession string

	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write(b)
		_ = w.WriteByte('\n')
		_ = w.Flush()
	}
	result := func(id json.RawMessage, res any) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": res})
	}
	updateText := func(session, text string) {
		send(map[string]any{
			"jsonrpc": "2.0",
			"method":  "session/update",
			"params": map[string]any{
				"sessionId": session,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": text},
				},
			},
		})
	}

	for sc.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if m.Method == "" {
			// A response to a permission request we sent: echo the chosen option
			// id and finish the waiting turn.
			if pendingPromptID != nil {
				var r struct {
					Result struct {
						Outcome struct {
							OptionID string `json:"optionId"`
						} `json:"outcome"`
					} `json:"result"`
				}
				_ = json.Unmarshal(sc.Bytes(), &r)
				updateText(pendingSession, "perm:"+r.Result.Outcome.OptionID)
				result(pendingPromptID, map[string]any{"stopReason": "end_turn"})
				pendingPromptID = nil
			}
			continue
		}
		switch m.Method {
		case "initialize":
			result(m.ID, map[string]any{
				"protocolVersion":   1,
				"agentCapabilities": map[string]any{"loadSession": true},
				"agentInfo":         map[string]any{"name": "fake-opencode", "version": "0.0.0"},
			})
		case "session/new":
			result(m.ID, map[string]any{"sessionId": "ses_fake", "configOptions": configOptions(state)})
		case "session/load":
			saved := state["model"]
			state["model"] = "LOADED"
			result(m.ID, map[string]any{"configOptions": configOptions(state)})
			state["model"] = saved
		case "session/set_config_option":
			var p struct {
				ConfigID string `json:"configId"`
				Value    string `json:"value"`
			}
			_ = json.Unmarshal(m.Params, &p)
			if p.ConfigID != "" {
				state[p.ConfigID] = p.Value
			}
			result(m.ID, map[string]any{"configOptions": configOptions(state)})
		case "session/prompt":
			var p struct {
				SessionID string `json:"sessionId"`
				Prompt    []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(m.Params, &p)
			var text strings.Builder
			for _, b := range p.Prompt {
				text.WriteString(b.Text)
			}
			trimmed := strings.TrimSpace(text.String())
			if strings.Contains(trimmed, "HANG") {
				continue // leave the turn unanswered
			}
			if strings.HasPrefix(trimmed, "PERM:") {
				cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "PERM:"))
				pendingPromptID = append(json.RawMessage(nil), m.ID...)
				pendingSession = p.SessionID
				id := nextReqID
				nextReqID++
				send(map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"method":  "session/request_permission",
					"params": map[string]any{
						"sessionId": p.SessionID,
						"toolCall": map[string]any{
							"toolCallId": "t_perm",
							"title":      cmd,
							"kind":       "execute",
							"rawInput":   map[string]any{"command": cmd},
						},
						"options": []map[string]any{
							{"optionId": "once", "name": "Allow once", "kind": "allow_once"},
							{"optionId": "always", "name": "Always allow", "kind": "allow_always"},
							{"optionId": "reject", "name": "Reject", "kind": "reject_once"},
						},
					},
				})
				continue // wait for the permission response before finishing
			}
			updateText(p.SessionID, "fake pong")
			result(m.ID, map[string]any{"stopReason": "end_turn"})
		case "session/request_permission":
			result(m.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow_once"}})
		}
		// Notifications (session/cancel, notifications/initialized) have no id
		// and are intentionally not answered.
	}
}

func configOptions(state map[string]string) []any {
	sel := func(id, category string, values ...string) any {
		opts := make([]any, 0, len(values))
		for _, v := range values {
			opts = append(opts, map[string]any{"name": v, "value": v})
		}
		return map[string]any{
			"type":         "select",
			"id":           id,
			"name":         id,
			"category":     category,
			"currentValue": state[id],
			"options":      opts,
		}
	}
	return []any{
		sel("model", "model", "a/b", "a/c", "a/b/high", "a/c/high", "a/b/low"),
		sel("effort", "thought_level", "default", "low", "medium", "high", "max"),
		sel("mode", "mode", "build", "plan"),
	}
}
