// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResultMCP(t *testing.T) {
	configDir := t.TempDir()
	writeFile(t, filepath.Join(configDir, "opencode.json"), `{"mcp":{
		"postgres":{"type":"local","command":["npx","-y","pg"],"environment":{"DATABASE_URL":"postgres://secret","TOKEN":{"env":"PG_TOKEN"}}},
		"github":{"type":"remote","url":"https://mcp.example/v1","headers":{"Authorization":"Bearer secret"}}
	}}`)

	res := Result(configDir, []protocol.DiscoverKind{protocol.DiscoverKindMCP}, nil)
	if len(res.McpServers) != 2 {
		t.Fatalf("mcp servers = %d, want 2: %+v", len(res.McpServers), res.McpServers)
	}
	byName := map[string]protocol.DiscoveredMcp{}
	for _, m := range res.McpServers {
		byName[m.Name] = m
	}

	pg, ok := byName["postgres"]
	if !ok {
		t.Fatal("postgres missing")
	}
	if pg.Transport != protocol.McpTransportStdio || len(pg.Command) != 3 || pg.Command[0] != "npx" {
		t.Fatalf("postgres = %+v", pg)
	}
	if !pg.Env["DATABASE_URL"].Secret {
		t.Fatalf("DATABASE_URL must be secret-marked: %+v", pg.Env)
	}
	if pg.Env["TOKEN"].EnvRef != "PG_TOKEN" || pg.Env["TOKEN"].Secret {
		t.Fatalf("TOKEN must be an envRef: %+v", pg.Env)
	}

	gh, ok := byName["github"]
	if !ok {
		t.Fatal("github missing")
	}
	if gh.Transport != protocol.McpTransportHTTP || gh.URL != "https://mcp.example/v1" {
		t.Fatalf("github = %+v", gh)
	}
	if !gh.Headers["Authorization"].Secret {
		t.Fatalf("Authorization must be secret-marked: %+v", gh.Headers)
	}
}

func TestResultSkills(t *testing.T) {
	configDir := t.TempDir()
	project := t.TempDir()
	writeFile(t, filepath.Join(configDir, "skills", "global-skill", "SKILL.md"), "---\nname: global-skill\n---\n")
	writeFile(t, filepath.Join(project, ".opencode", "skills", "project-skill", "SKILL.md"), "---\nname: project-skill\n---\n")

	res := Result(configDir, []protocol.DiscoverKind{protocol.DiscoverKindSkills}, []string{project})
	if len(res.Skills) != 2 {
		t.Fatalf("skills = %d, want 2: %+v", len(res.Skills), res.Skills)
	}
	byName := map[string]protocol.DiscoveredSkill{}
	for _, s := range res.Skills {
		byName[s.Name] = s
	}
	if g, ok := byName["global-skill"]; !ok || g.Scope != protocol.DiscoverScopeUser || g.Path == "" {
		t.Fatalf("global-skill = %+v", byName["global-skill"])
	}
	if p, ok := byName["project-skill"]; !ok || p.Scope != protocol.DiscoverScopeProject || p.Path == "" {
		t.Fatalf("project-skill = %+v", byName["project-skill"])
	}
}

func TestResultEmpty(t *testing.T) {
	res := Result(t.TempDir(), nil, nil)
	if res.McpServers == nil || res.Skills == nil {
		t.Fatal("empty result must have non-nil slices")
	}
	if len(res.McpServers) != 0 || len(res.Skills) != 0 {
		t.Fatalf("empty result = %+v", res)
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("empty result must validate: %v", err)
	}
}
