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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// flavourEnv returns the fake flavour: v1 or v2 (default v2).
func flavourEnv() string {
	if f := os.Getenv("FAKE_OPENCODE_FLAVOUR"); f == "v1" {
		return "v1"
	}
	return "v2"
}

// printAuthList prints `opencode auth list` output in the flavour's shape
// (FINDINGS V3): v2 rows are "<name>  <id>  stored", v1 rows are "●  <name> <type>".
func printAuthList() {
	if flavourEnv() == "v2" {
		fmt.Println("DeepSeek  deepseek  stored")
		return
	}
	fmt.Println("●  DeepSeek api")
}

// printModels prints `opencode models` (v2: one provider/model per line) or
// `opencode models --verbose` (v1: header line + a JSON object per model).
func printModels(verbose bool) {
	if !verbose {
		fmt.Println("opencode/beam-free")
		fmt.Println("opencode/big-pickle")
		fmt.Println("deepseek/deepseek-v4-pro")
		return
	}
	models := []struct {
		id, name, provider string
		context            int
		image              bool
		variants           string
	}{
		{"beam-free", "Beam Free", "opencode", 1048576, true, `{"low":{"reasoningEffort":"low"},"high":{"reasoningEffort":"high"}}`},
		{"deepseek-v4-pro", "DeepSeek V4 Pro", "deepseek", 131072, true, `{"low":{"reasoningEffort":"low"},"high":{"reasoningEffort":"high"}}`},
	}
	for _, m := range models {
		fmt.Printf("%s/%s\n", m.provider, m.id)
		fmt.Printf(`{"id":%q,"providerID":%q,"name":%q,"api":{"url":"https://example.invalid/v1"},"status":"active","limit":{"context":%d,"output":131072},"capabilities":{"input":{"image":%t}},"variants":%s}`+"\n",
			m.id, m.provider, m.name, m.context, m.image, m.variants)
	}
}

func main() {
	args := os.Args[1:]
	if slices.Contains(args, "--version") {
		v := os.Getenv("FAKE_OPENCODE_VERSION")
		if v == "" {
			v = "opencode v2.2.5"
		}
		fmt.Println(v)
		return
	}
	switch {
	case len(args) == 2 && args[0] == "auth" && args[1] == "list":
		printAuthList()
		return
	case len(args) == 1 && args[0] == "models":
		printModels(false)
		return
	case len(args) == 2 && args[0] == "models" && args[1] == "--verbose":
		printModels(true)
		return
	case len(args) == 1 && args[0] == "acp":
		// fall through to the ACP loop below
	default:
		fmt.Fprintf(os.Stderr, "fakeopencode: unknown args %v\n", args)
		os.Exit(2)
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
	w := bufio.NewWriter(os.Stdout)
	state := map[string]string{"model": "a/b", "effort": "default", "mode": "build"}

	var nextReqID int64 = 1000
	var nextSessionID int
	var pendingPromptID json.RawMessage
	var pendingSession string
	var holdMode bool

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
	// reportEnv emits the proxy environment the child actually sees as one
	// NAME=VALUE line per variable in the agent_message_chunk text (the release
	// proxy probe parses these; PROTOCOL §15, SDK proxy.go checkProxyEnv).
	reportEnv := func(session string) {
		var b strings.Builder
		for _, name := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY", "NO_PROXY", "SSH_AUTH_SOCK"} {
			fmt.Fprintf(&b, "%s=%s\n", name, os.Getenv(name))
		}
		updateText(session, b.String())
	}
	// childChunk emits an agent_message_chunk whose update carries
	// _meta["opencode/child-session"], matching FINDINGS V7 and the shape
	// translateUpdateMeta decodes (internal/proxy/meta.go).
	childChunk := func(session, text string) {
		send(map[string]any{
			"jsonrpc": "2.0",
			"method":  "session/update",
			"params": map[string]any{
				"sessionId": session,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": text},
					"_meta": map[string]any{
						"opencode/child-session": map[string]any{
							"id": "ses_child", "parentID": "t_sub", "title": "Sub", "depth": 1,
						},
					},
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
		if m.Method == "session/cancel" {
			if holdMode && pendingPromptID != nil {
				result(pendingPromptID, map[string]any{"stopReason": "cancelled"})
				pendingPromptID = nil
				pendingSession = ""
				holdMode = false
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
			nextSessionID++
			result(m.ID, map[string]any{"sessionId": fmt.Sprintf("ses_%d_%d", os.Getpid(), nextSessionID), "configOptions": configOptions(state)})
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
			if strings.Contains(trimmed, "HOLD") {
				updateText(p.SessionID, "holding")
				pendingPromptID = append(json.RawMessage(nil), m.ID...)
				pendingSession = p.SessionID
				holdMode = true
				continue
			}
			if strings.HasPrefix(trimmed, "ENV:") {
				reportEnv(p.SessionID)
				result(m.ID, map[string]any{"stopReason": "end_turn"})
				continue
			}
			if strings.HasPrefix(trimmed, "SUBAGENT:") {
				childChunk(p.SessionID, "sub pong")
				updateText(p.SessionID, "parent pong")
				result(m.ID, map[string]any{"stopReason": "end_turn"})
				continue
			}
			if strings.HasPrefix(trimmed, "SKILLS:") {
				updateText(p.SessionID, reportSkills())
				result(m.ID, map[string]any{"stopReason": "end_turn"})
				continue
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
			if strings.HasPrefix(trimmed, "ERR:") {
				name := strings.TrimSpace(strings.TrimPrefix(trimmed, "ERR:"))
				switch name {
				case "model":
					send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{
						"code":    -32602,
						"message": "Invalid params: model not found: nonexistent/nonexistent",
						"data":    map[string]any{"modelId": "nonexistent/nonexistent", "providerId": "nonexistent"},
					}})
				case "provider.transport", "APIError":
					send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{
						"code":    -32603,
						"message": "Internal error: ECONNREFUSED",
						"data":    map[string]any{"errorName": name, "service": "session"},
					}})
				default: // "provider.free-limit" or an explicit unknown marker
					errorName := "provider.free-limit"
					msg := "Internal error: Free usage limit reached."
					if name != "provider.free-limit" {
						errorName = "unknown.marker"
						msg = "Internal error: unknown fake marker " + name
					}
					send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{
						"code":    -32603,
						"message": msg,
						"data":    map[string]any{"errorName": errorName, "service": "session"},
					}})
				}
				continue
			}
			if strings.Contains(trimmed, "USAGE2") {
				send(map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": p.SessionID,
						"update": map[string]any{
							"sessionUpdate": "usage_update",
							"size":          200000,
							"used":          2345,
							"cost":          map[string]any{"amount": 0.84, "currency": "USD"},
						},
					},
				})
				updateText(p.SessionID, "fake pong 2")
				result(m.ID, map[string]any{
					"stopReason": "end_turn",
					"usage": map[string]any{
						"inputTokens":      200,
						"outputTokens":     100,
						"cachedReadTokens": 50,
						"totalTokens":      350,
					},
				})
				continue
			}
			if strings.Contains(trimmed, "USAGE") {
				send(map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": p.SessionID,
						"update": map[string]any{
							"sessionUpdate": "usage_update",
							"size":          200000,
							"used":          1234,
							"cost":          map[string]any{"amount": 0.42, "currency": "USD"},
						},
					},
				})
				updateText(p.SessionID, "fake pong")
				result(m.ID, map[string]any{
					"stopReason": "end_turn",
					"usage": map[string]any{
						"inputTokens":      100,
						"outputTokens":     50,
						"cachedReadTokens": 25,
						"totalTokens":      175,
					},
				})
				continue
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

// reportSkills returns one line per file under <cwd>/.opencode/skills and the
// first line of the instructions file named in OPENCODE_CONFIG_CONTENT, proving
// per-session skills materialization and instructions delivery.
func reportSkills() string {
	cwd, _ := os.Getwd()
	var b strings.Builder
	entries, err := os.ReadDir(filepath.Join(cwd, ".opencode", "skills"))
	if err == nil {
		for _, e := range entries {
			fmt.Fprintf(&b, "skill=%s\n", e.Name())
		}
	}
	var cfg struct {
		Instructions []string `json:"instructions"`
	}
	if v := os.Getenv("OPENCODE_CONFIG_CONTENT"); v != "" {
		if json.Unmarshal([]byte(v), &cfg) == nil && len(cfg.Instructions) > 0 {
			if data, err := os.ReadFile(cfg.Instructions[0]); err == nil {
				first := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
				fmt.Fprintf(&b, "instruction=%s\n", first)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
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
