// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"context"
	"encoding/json"
	"fmt"

	acp "github.com/coder/acp-go-sdk"
)

// Sink receives a session's upstream events while a turn is running.
type Sink interface {
	Update(n acp.SessionNotification)
	RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) acp.RequestPermissionResponse
}

// register wires the two child-initiated methods. Call once in handshake.
func (c *Client) register() {
	c.peer.HandleNotification("session/update", c.onUpdate)
	c.peer.Handle("session/request_permission", c.onRequestPermission)
}

func (c *Client) onUpdate(_ context.Context, params json.RawMessage) {
	var n acp.SessionNotification
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	c.mu.Lock()
	s := c.sink
	c.mu.Unlock()
	if s == nil {
		if c.cfg.Log != nil {
			c.cfg.Log.Debug("upstream update with no active turn", "session", c.redact(string(n.SessionId)))
		}
		return
	}
	s.Update(n)
}

func (c *Client) onRequestPermission(ctx context.Context, params json.RawMessage) (any, error) {
	var req acp.RequestPermissionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("upstream: request_permission: %w", err)
	}
	c.mu.Lock()
	s := c.sink
	c.mu.Unlock()
	if s == nil {
		// No turn is running: reject so the child cannot hang.
		return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
	}
	return s.RequestPermission(ctx, req), nil
}

// NewSession creates the child's session.
func (c *Client) NewSession(ctx context.Context, cwd string, mcp []acp.McpServer) (acp.NewSessionResponse, error) {
	if mcp == nil {
		mcp = []acp.McpServer{}
	}
	var res acp.NewSessionResponse
	err := c.peer.Call(ctx, "session/new", acp.NewSessionRequest{Cwd: cwd, McpServers: mcp}, &res)
	return res, MapError(err)
}

// LoadSession resumes the child's session.
func (c *Client) LoadSession(ctx context.Context, id acp.SessionId, cwd string, mcp []acp.McpServer) (acp.LoadSessionResponse, error) {
	if mcp == nil {
		mcp = []acp.McpServer{}
	}
	var res acp.LoadSessionResponse
	err := c.peer.Call(ctx, "session/load", acp.LoadSessionRequest{SessionId: id, Cwd: cwd, McpServers: mcp}, &res)
	return res, MapError(err)
}

// Prompt runs one turn. sink is installed for the duration of the call.
func (c *Client) Prompt(ctx context.Context, id acp.SessionId, blocks []acp.ContentBlock, sink Sink) (acp.PromptResponse, error) {
	c.mu.Lock()
	c.sink = sink
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.sink = nil
		c.mu.Unlock()
	}()
	var res acp.PromptResponse
	err := c.peer.Call(ctx, "session/prompt", acp.PromptRequest{SessionId: id, Prompt: blocks}, &res)
	return res, MapError(err)
}

// SetOption changes a config option.
func (c *Client) SetOption(ctx context.Context, id acp.SessionId, configID, value string) ([]acp.SessionConfigOption, error) {
	var res acp.SetSessionConfigOptionResponse
	err := c.peer.Call(ctx, "session/set_config_option", acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: id,
			ConfigId:  acp.SessionConfigId(configID),
			Value:     acp.SessionConfigValueId(value),
		},
	}, &res)
	return res.ConfigOptions, MapError(err)
}

// Cancel sends the session/cancel notification (graceful).
func (c *Client) Cancel(id acp.SessionId) error {
	return c.peer.Notify("session/cancel", acp.CancelNotification{SessionId: id})
}
