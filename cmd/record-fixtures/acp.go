// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// acpConn drives one "opencode acp" process and records every JSONL frame.
type acpConn struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	lines         chan []byte
	done          chan error
	rec           func(direction string, msg []byte) error
	autoApprove   bool
	approveAlways bool
}

func startACP(ctx context.Context, bin, dir string, standalone bool, args []string, autoApprove, approveAlways bool) (*acpConn, func(), error) {
	argv := []string{"acp"}
	if standalone {
		argv = []string{"--standalone", "acp"}
	}
	cmd := exec.CommandContext(ctx, bin, append(argv, args...)...)
	cmd.Env = append(os.Environ(), "OPENCODE_LOG_LEVEL=error")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = os.Stderr // debug only; never committed as a fixture

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	f, err := os.Create(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, nil, err
	}

	c := &acpConn{
		cmd:           cmd,
		stdin:         stdin,
		lines:         make(chan []byte, 256),
		done:          make(chan error, 1),
		autoApprove:   autoApprove,
		approveAlways: approveAlways,
	}
	enc := json.NewEncoder(f)
	c.rec = func(direction string, msg []byte) error {
		return enc.Encode(frame{Direction: direction, Message: sanitize(msg)})
	}

	go func() {
		defer close(c.lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<26) // 64 MiB max line
		for sc.Scan() {
			line := make([]byte, len(sc.Bytes()))
			copy(line, sc.Bytes())
			c.lines <- line
		}
		c.done <- sc.Err()
	}()

	closeFn := func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = f.Close()
	}
	return c, closeFn, nil
}

// send writes one JSON-RPC message and records it as client_to_server.
func (c *acpConn) send(msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return err
	}
	return c.rec("client_to_server", data)
}

// await reads frames until the response with the given id arrives. Every frame
// is recorded; notifications are skipped over. It returns the raw response.
func (c *acpConn) await(ctx context.Context, id int) (json.RawMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case line, ok := <-c.lines:
			if !ok {
				return nil, errors.New("acp: server closed stdout before answering")
			}
			if err := c.rec("server_to_client", line); err != nil {
				return nil, err
			}
			var msg struct {
				ID     *json.RawMessage `json:"id"`
				Result json.RawMessage  `json:"result"`
				Error  json.RawMessage  `json:"error"`
			}
			if err := json.Unmarshal(line, &msg); err != nil {
				continue // not a JSON-RPC frame; keep it in the transcript
			}
			if msg.ID == nil {
				continue // notification, e.g. session/update
			}
			if c.autoApprove && serverRequestMethod(line) == "session/request_permission" {
				if err := c.approvePermission(line); err != nil {
					return nil, err
				}
				continue
			}
			var gotID int
			if err := json.Unmarshal(*msg.ID, &gotID); err != nil || gotID != id {
				continue
			}
			if len(msg.Error) > 0 && string(msg.Error) != "null" {
				return nil, fmt.Errorf("acp: %s answered an error: %s", methodForID(id), msg.Error)
			}
			return line, nil
		}
	}
}

// approvePermission answers a session/request_permission with the first
// allow-once option so a prompt that needs tool approval can complete while
// keeping the request itself in the transcript.
func (c *acpConn) approvePermission(line []byte) error {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Params struct {
			Options []struct {
				OptionID string `json:"optionId"`
				Kind     string `json:"kind"`
			} `json:"options"`
		} `json:"params"`
	}
	if err := json.Unmarshal(line, &req); err != nil {
		return err
	}
	chosen := ""
	want := "allow_once"
	if c.approveAlways {
		want = "allow_always"
	}
	for _, o := range req.Params.Options {
		if o.Kind == want {
			chosen = o.OptionID
			break
		}
	}
	if chosen == "" {
		for _, o := range req.Params.Options {
			if strings.Contains(o.Kind, "allow") {
				chosen = o.OptionID
				break
			}
		}
	}
	if chosen == "" && len(req.Params.Options) > 0 {
		chosen = req.Params.Options[0].OptionID
	}
	if chosen == "" {
		return errors.New("acp: permission request has no options")
	}
	return c.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]any{
			"outcome": map[string]any{"outcome": "selected", "optionId": chosen},
		},
	})
}

// serverRequestMethod returns the method of a server-to-client request, or "".
func serverRequestMethod(line []byte) string {
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(line, &msg); err != nil || len(msg.ID) == 0 {
		return ""
	}
	return msg.Method
}

// sessionIDOf extracts result.sessionId from a session/new response.
func sessionIDOf(resp json.RawMessage) (string, error) {
	var msg struct {
		Result struct {
			SessionID string `json:"sessionId"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &msg); err != nil {
		return "", err
	}
	if msg.Result.SessionID == "" {
		return "", errors.New("acp: session/new returned no sessionId")
	}
	return msg.Result.SessionID, nil
}

// methodForID names the request for error messages.
func methodForID(id int) string {
	switch id {
	case 1:
		return "initialize"
	case 2:
		return "session/new"
	case 3:
		return "session/set_config_option"
	case 4:
		return "session/prompt"
	case 5:
		return "session/load"
	case 6:
		return "session/set_config_option (effort)"
	default:
		return fmt.Sprintf("request %d", id)
	}
}

func (c *acpConn) initialize(ctx context.Context) error {
	return c.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": 1,
			"clientCapabilities": map[string]any{
				"fs":       map[string]any{"readTextFile": false, "writeTextFile": false},
				"terminal": false,
			},
			"clientInfo": map[string]any{"name": "arvel-plugin-opencode", "version": "0.1.0"},
		},
	})
}

func (c *acpConn) initializedNotice() error {
	return c.send(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
}

// workDir is the session cwd: the explicit --cwd, else the fixture directory.
func workDir(r recordEnv) string {
	if r.CWD != "" {
		return r.CWD
	}
	return r.Dir
}

func runACP(ctx context.Context, r recordEnv, steps func(*acpConn) error) (Metadata, error) {
	c, closeFn, err := startACP(ctx, r.Bin, r.Dir, r.ACPStandalone, r.Args, r.AutoApprove, r.ApproveAlways)
	if err != nil {
		return Metadata{}, err
	}
	defer closeFn()

	if err := c.initialize(ctx); err != nil {
		return Metadata{}, err
	}
	if _, err := c.await(ctx, 1); err != nil {
		return Metadata{}, err
	}
	if err := c.initializedNotice(); err != nil {
		return Metadata{}, err
	}
	if steps != nil {
		if err := steps(c); err != nil {
			return Metadata{}, err
		}
	}
	return Metadata{}, nil
}

func scenarioHandshake(ctx context.Context, r recordEnv) (Metadata, error) {
	return runACP(ctx, r, nil)
}

func scenarioSessionNew(ctx context.Context, r recordEnv) (Metadata, error) {
	return runACP(ctx, r, func(c *acpConn) error {
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "session/new",
			"params":  map[string]any{"cwd": workDir(r), "mcpServers": []any{}},
		}); err != nil {
			return err
		}
		_, err := c.await(ctx, 2)
		return err
	})
}

// scenarioResume opens a session and immediately loads it again; it records
// whether the ACP implementation advertises and implements session/load.
func scenarioResume(ctx context.Context, r recordEnv) (Metadata, error) {
	return runACP(ctx, r, func(c *acpConn) error {
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "session/new",
			"params":  map[string]any{"cwd": workDir(r), "mcpServers": []any{}},
		}); err != nil {
			return err
		}
		resp, err := c.await(ctx, 2)
		if err != nil {
			return err
		}
		id, err := sessionIDOf(resp)
		if err != nil {
			return err
		}
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      5,
			"method":  "session/load",
			"params":  map[string]any{"sessionId": id, "cwd": workDir(r), "mcpServers": []any{}},
		}); err != nil {
			return err
		}
		_, err = c.await(ctx, 5)
		return err
	})
}

// scenarioPrompt asks one question. When a model is given it is selected first
// through session/set_config_option; the model id must come from
// `opencode models`.
func scenarioPrompt(ctx context.Context, r recordEnv) (Metadata, error) {
	return runACP(ctx, r, func(c *acpConn) error {
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "session/new",
			"params":  map[string]any{"cwd": workDir(r), "mcpServers": []any{}},
		}); err != nil {
			return err
		}
		resp, err := c.await(ctx, 2)
		if err != nil {
			return err
		}
		id, err := sessionIDOf(resp)
		if err != nil {
			return err
		}
		if r.Model != "" {
			if err := c.send(map[string]any{
				"jsonrpc": "2.0",
				"id":      3,
				"method":  "session/set_config_option",
				"params":  map[string]any{"sessionId": id, "configId": "model", "value": r.Model},
			}); err != nil {
				return err
			}
			if _, err := c.await(ctx, 3); err != nil {
				return err
			}
		}
		if r.Effort != "" {
			if err := c.send(map[string]any{
				"jsonrpc": "2.0",
				"id":      6,
				"method":  "session/set_config_option",
				"params":  map[string]any{"sessionId": id, "configId": "effort", "value": r.Effort},
			}); err != nil {
				return err
			}
			if _, err := c.await(ctx, 6); err != nil {
				return err
			}
		}
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      4,
			"method":  "session/prompt",
			"params": map[string]any{
				"sessionId": id,
				"prompt":    []any{map[string]any{"type": "text", "text": r.Prompt}},
			},
		}); err != nil {
			return err
		}
		// A failed turn is a finding (error shape, proxy behaviour), so the
		// error answer is recorded instead of aborting the capture.
		_, _ = c.await(ctx, 4)
		return nil
	})
}

// scenarioConfigError selects a model that does not exist and records the
// error answer. A failing probe is evidence, so the error is not returned.
func scenarioConfigError(ctx context.Context, r recordEnv) (Metadata, error) {
	return runACP(ctx, r, func(c *acpConn) error {
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "session/new",
			"params":  map[string]any{"cwd": workDir(r), "mcpServers": []any{}},
		}); err != nil {
			return err
		}
		resp, err := c.await(ctx, 2)
		if err != nil {
			return err
		}
		id, err := sessionIDOf(resp)
		if err != nil {
			return err
		}
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "session/set_config_option",
			"params":  map[string]any{"sessionId": id, "configId": "model", "value": "nonexistent/nonexistent"},
		}); err != nil {
			return err
		}
		_, _ = c.await(ctx, 3) // the error is the finding
		return nil
	})
}

// scenarioEffortError selects an effort level and records the answer. v1 does
// not advertise an effort option, so the error (or its absence) is the finding.
func scenarioEffortError(ctx context.Context, r recordEnv) (Metadata, error) {
	return runACP(ctx, r, func(c *acpConn) error {
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "session/new",
			"params":  map[string]any{"cwd": workDir(r), "mcpServers": []any{}},
		}); err != nil {
			return err
		}
		resp, err := c.await(ctx, 2)
		if err != nil {
			return err
		}
		id, err := sessionIDOf(resp)
		if err != nil {
			return err
		}
		if err := c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "session/set_config_option",
			"params":  map[string]any{"sessionId": id, "configId": "effort", "value": "high"},
		}); err != nil {
			return err
		}
		_, _ = c.await(ctx, 3) // the error is the finding
		return nil
	})
}
