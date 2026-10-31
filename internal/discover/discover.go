// SPDX-License-Identifier: Apache-2.0

// Package discover implements the read-only _arvel/discover response (PROTOCOL
// §7.9, SPEC §10). It reads the OpenCode config "mcp" key and the skill
// directories for the global config root and each listed project. Secret values
// are never read out: env and header entries are returned only as secret/envRef
// markers.
package discover

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

// Result builds the discovery result. configDir is the OpenCode config root
// (~/.config/opencode). projects are absolute project directories; when empty
// only the user scope is listed. Unknown files or malformed entries are skipped,
// so an empty result is always valid.
func Result(configDir string, kinds []protocol.DiscoverKind, projects []string) protocol.DiscoverResult {
	out := protocol.DiscoverResult{
		McpServers: []protocol.DiscoveredMcp{},
		Skills:     []protocol.DiscoveredSkill{},
	}
	wantMCP, wantSkills := want(kinds)

	if wantMCP {
		out.McpServers = append(out.McpServers, readMCP(filepath.Join(configDir, "opencode.json"), "", protocol.DiscoverScopeUser, "~/.config/opencode/opencode.json")...)
		out.McpServers = append(out.McpServers, readMCP(filepath.Join(configDir, "opencode.jsonc"), "", protocol.DiscoverScopeUser, "~/.config/opencode/opencode.jsonc")...)
	}
	if wantSkills {
		out.Skills = append(out.Skills, readSkills(filepath.Join(configDir, "skills"), protocol.DiscoverScopeUser, "~/.config/opencode/skills")...)
	}

	for _, p := range projects {
		if wantMCP {
			out.McpServers = append(out.McpServers, readMCP(filepath.Join(p, "opencode.json"), p, protocol.DiscoverScopeProject, filepath.Join(p, "opencode.json"))...)
			out.McpServers = append(out.McpServers, readMCP(filepath.Join(p, "opencode.jsonc"), p, protocol.DiscoverScopeProject, filepath.Join(p, "opencode.jsonc"))...)
		}
		if wantSkills {
			out.Skills = append(out.Skills, readSkills(filepath.Join(p, ".opencode", "skills"), protocol.DiscoverScopeProject, filepath.Join(p, ".opencode", "skills"))...)
		}
	}
	return out
}

func want(kinds []protocol.DiscoverKind) (mcp, skills bool) {
	if len(kinds) == 0 {
		return true, true // no filter: list everything
	}
	for _, k := range kinds {
		switch k {
		case protocol.DiscoverKindMCP:
			mcp = true
		case protocol.DiscoverKindSkills:
			skills = true
		}
	}
	return
}

// mcpConfig is the well-known OpenCode "mcp" entry shape. It is decoded
// defensively: unknown fields are ignored and a malformed entry is skipped.
type mcpConfig struct {
	Type        string         `json:"type"`
	Command     []string       `json:"command"`
	URL         string         `json:"url"`
	Headers     map[string]any `json:"headers"`
	Environment map[string]any `json:"environment"`
}

// readMCP parses the "mcp" key of one config file, if it exists.
func readMCP(path, project string, scope protocol.DiscoverScope, source string) []protocol.DiscoveredMcp {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		MCP map[string]mcpConfig `json:"mcp"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.MCP == nil {
		return nil
	}
	out := make([]protocol.DiscoveredMcp, 0, len(doc.MCP))
	for name, c := range doc.MCP {
		m := protocol.DiscoveredMcp{Name: name, Scope: scope, Project: project, Source: source}
		if c.Type == "remote" && c.URL != "" {
			m.Transport = protocol.McpTransportHTTP
			m.URL = c.URL
		} else {
			m.Transport = protocol.McpTransportStdio
			m.Command = c.Command
		}
		m.Env = redactFields(c.Environment)
		m.Headers = redactFields(c.Headers)
		out = append(out, m)
	}
	return out
}

// redactFields converts raw env/header values into secret markers. A value that
// is an {"env": "NAME"} reference becomes EnvRef; any literal becomes Secret.
// The value itself is never copied out.
func redactFields(in map[string]any) map[string]protocol.SecretField {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]protocol.SecretField, len(in))
	for k, v := range in {
		if m, ok := v.(map[string]any); ok {
			if ref, ok := m["env"].(string); ok {
				out[k] = protocol.SecretField{EnvRef: ref}
				continue
			}
		}
		out[k] = protocol.SecretField{Secret: true}
	}
	return out
}

// readSkills lists the subdirectories of a skill directory, if it exists.
func readSkills(dir string, scope protocol.DiscoverScope, source string) []protocol.DiscoveredSkill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]protocol.DiscoveredSkill, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "" || e.Name()[0] == '.' {
			continue
		}
		out = append(out, protocol.DiscoveredSkill{
			Name:   e.Name(),
			Scope:  scope,
			Source: source,
			Path:   filepath.Join(dir, e.Name()),
		})
	}
	return out
}
