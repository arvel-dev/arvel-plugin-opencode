// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"sync"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

// matchPattern reports whether s matches pattern, where '*' matches any
// sequence (including empty) and '?' matches one character (FINDINGS V4).
func matchPattern(pattern, s string) bool {
	p, n, star, pp := 0, 0, -1, 0
	for n < len(s) {
		switch {
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == s[n]):
			p++
			n++
		case p < len(pattern) && pattern[p] == '*':
			star, pp = p, n
			p++
		case star >= 0:
			p = star + 1
			pp++
			n = pp
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// commandText extracts the command from a tool call's rawInput, if it is a
// map with a string "command" key (OpenCode bash/task rawInput shape).
func commandText(raw any) string {
	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := m["command"].(string); ok {
		return s
	}
	return ""
}

// pathsFrom collects the non-empty file paths of a tool call.
func pathsFrom(locs []acp.ToolCallLocation) []string {
	var out []string
	for _, l := range locs {
		if l.Path != "" {
			out = append(out, l.Path)
		}
	}
	return out
}

// allowList is the session-local "always allow" set (SPEC §6.2). The host's
// "always" is never forwarded to the vendor; it is kept here instead.
type allowList struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (a *allowList) hit(k string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.keys[k]
}

func (a *allowList) add(k string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.keys == nil {
		a.keys = map[string]bool{}
	}
	a.keys[k] = true
}

// allowKey is the allow-list key: category + the most specific identity
// (command text when present, else the first affected path, else the title).
// The path keeps an "always allow" for one file from auto-approving another
// file whose tool call carries only the same generic title.
func allowKey(category protocol.PermissionCategory, tc acp.ToolCallUpdate) string {
	id := ""
	if tc.Title != nil {
		id = *tc.Title
	}
	if cmd := commandText(tc.RawInput); cmd != "" {
		id = cmd
	} else if paths := pathsFrom(tc.Locations); len(paths) > 0 {
		id = paths[0]
	}
	return string(category) + "\x00" + id
}

// decide returns the Arvel level for a request, using last-match-wins for
// command patterns (deny wins over allow; FINDINGS V4).
func (s *Session) decide(category protocol.PermissionCategory, tc acp.ToolCallUpdate) protocol.PermissionLevel {
	p := s.meta.Permissions
	if p == nil {
		return protocol.PermissionLevelAsk
	}
	switch category {
	case protocol.PermissionCategoryCommand:
		if c := p.Commands; c != nil {
			cmd := commandText(tc.RawInput)
			for _, pat := range c.Deny {
				if matchPattern(pat, cmd) {
					return protocol.PermissionLevelDeny
				}
			}
			for _, pat := range c.Allow {
				if matchPattern(pat, cmd) {
					return protocol.PermissionLevelAllow
				}
			}
			if c.Default != "" {
				return c.Default
			}
		}
		return protocol.PermissionLevelAsk
	case protocol.PermissionCategoryEdit:
		if p.Edit != "" {
			return p.Edit
		}
	case protocol.PermissionCategoryRead:
		if p.Read != "" {
			return p.Read
		}
	case protocol.PermissionCategoryNetwork:
		if p.Network != "" {
			return p.Network
		}
	case protocol.PermissionCategoryMCP:
		if m := p.MCP; m != nil {
			name := ""
			if tc.Title != nil {
				name = *tc.Title
			}
			if l, ok := m.Tools[name]; ok {
				return l
			}
			if m.Default != "" {
				return m.Default
			}
		}
	}
	return protocol.PermissionLevelAsk
}

// optionByKind returns the option id with the given kind, and whether it exists.
func optionByKind(opts []acp.PermissionOption, k acp.PermissionOptionKind) (acp.PermissionOptionId, bool) {
	for _, o := range opts {
		if o.Kind == k {
			return o.OptionId, true
		}
	}
	return "", false
}

// isKind reports whether the option with the given id has the given kind.
func isKind(opts []acp.PermissionOption, id string, k acp.PermissionOptionKind) bool {
	for _, o := range opts {
		if string(o.OptionId) == id && o.Kind == k {
			return true
		}
	}
	return false
}

// permissionMeta builds the _meta.arvel for a forwarded ask request (PROTOCOL §7.3).
func permissionMeta(category protocol.PermissionCategory, tc acp.ToolCallUpdate) protocol.PermissionMeta {
	return protocol.PermissionMeta{
		Category: category,
		Command:  commandText(tc.RawInput),
		Paths:    pathsFrom(tc.Locations),
	}
}

// mcpName returns the MCP server's name from whichever transport is set.
func mcpName(s acp.McpServer) string {
	switch {
	case s.Stdio != nil:
		return s.Stdio.Name
	case s.Http != nil:
		return s.Http.Name
	case s.Sse != nil:
		return s.Sse.Name
	}
	return ""
}
