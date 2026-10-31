// SPDX-License-Identifier: Apache-2.0

package occonfig

import (
	"testing"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// goldenMeta is the shared fixture for the golden tests.
func goldenMeta() protocol.SessionMeta {
	return protocol.SessionMeta{
		Permissions: &protocol.Permissions{
			Read:    protocol.PermissionLevelAllow,
			Edit:    protocol.PermissionLevelAsk,
			Network: protocol.PermissionLevelAsk,
			Commands: &protocol.CommandPolicy{
				Default: protocol.PermissionLevelAsk,
				Allow:   []string{"go test *"},
				Deny:    []string{"git push *"},
			},
		},
		Subagents: &protocol.Subagents{Enabled: true},
	}
}

func TestBuildGolden(t *testing.T) {
	const path = "/data/run/r1/instructions.md"
	want := `{"instructions":["/data/run/r1/instructions.md"],"permission":{"bash":{"*":"ask","go test *":"allow","git push *":"deny"},"edit":"ask","read":"allow","webfetch":"ask"}}`

	for _, f := range []detect.Flavour{detect.V1, detect.V2} {
		t.Run(string(f), func(t *testing.T) {
			got := string(Build(f, goldenMeta(), path))
			if got != want {
				t.Fatalf("Build(%s) =\n%s\nwant\n%s", f, got, want)
			}
		})
	}
}

func TestBuildSubagentsDisabled(t *testing.T) {
	meta := goldenMeta()
	meta.Subagents = &protocol.Subagents{Enabled: false}
	want := `{"instructions":["/data/run/r1/instructions.md"],"permission":{"bash":{"*":"ask","go test *":"allow","git push *":"deny"},"edit":"ask","read":"allow","task":"deny","webfetch":"ask"}}`

	for _, f := range []detect.Flavour{detect.V1, detect.V2} {
		t.Run(string(f), func(t *testing.T) {
			if got := string(Build(f, meta, "/data/run/r1/instructions.md")); got != want {
				t.Fatalf("Build(%s) =\n%s\nwant\n%s", f, got, want)
			}
		})
	}
}

func TestBuildEmpty(t *testing.T) {
	for _, f := range []detect.Flavour{detect.V1, detect.V2} {
		t.Run(string(f), func(t *testing.T) {
			if got := string(Build(f, protocol.SessionMeta{}, "")); got != `{}` {
				t.Fatalf("Build(%s) = %s, want {}", f, got)
			}
		})
	}
}

func TestBuildNilCommands(t *testing.T) {
	meta := protocol.SessionMeta{
		Permissions: &protocol.Permissions{Edit: protocol.PermissionLevelDeny},
	}
	want := `{"permission":{"edit":"deny"}}`
	for _, f := range []detect.Flavour{detect.V1, detect.V2} {
		t.Run(string(f), func(t *testing.T) {
			if got := string(Build(f, meta, "")); got != want {
				t.Fatalf("Build(%s) = %s, want %s", f, got, want)
			}
		})
	}
}

// TestBuildMatrix pins every category x level combination for both flavours,
// including the ordered bash rules (last-match-wins, broad first, deny last).
func TestBuildMatrix(t *testing.T) {
	rows := []struct {
		name string
		meta protocol.SessionMeta
		want string
	}{
		{
			name: "allow_ask_deny_with_patterns",
			meta: protocol.SessionMeta{
				Permissions: &protocol.Permissions{
					Read:    protocol.PermissionLevelAllow,
					Edit:    protocol.PermissionLevelAsk,
					Network: protocol.PermissionLevelDeny,
					Commands: &protocol.CommandPolicy{
						Default: protocol.PermissionLevelAsk,
						Allow:   []string{"go test *"},
						Deny:    []string{"git push *"},
					},
				},
				Subagents: &protocol.Subagents{Enabled: true},
			},
			want: `{"permission":{"bash":{"*":"ask","go test *":"allow","git push *":"deny"},"edit":"ask","read":"allow","webfetch":"deny"}}`,
		},
		{
			name: "deny_allow_ask_default_allow_subagents_off",
			meta: protocol.SessionMeta{
				Permissions: &protocol.Permissions{
					Read:     protocol.PermissionLevelDeny,
					Edit:     protocol.PermissionLevelAllow,
					Network:  protocol.PermissionLevelAsk,
					Commands: &protocol.CommandPolicy{Default: protocol.PermissionLevelAllow},
				},
				Subagents: &protocol.Subagents{Enabled: false},
			},
			want: `{"permission":{"bash":{"*":"allow"},"edit":"allow","read":"deny","task":"deny","webfetch":"ask"}}`,
		},
		{
			name: "empty",
			meta: protocol.SessionMeta{},
			want: `{}`,
		},
	}
	for _, f := range []detect.Flavour{detect.V1, detect.V2} {
		for _, row := range rows {
			t.Run(string(f)+"/"+row.name, func(t *testing.T) {
				if got := string(Build(f, row.meta, "")); got != row.want {
					t.Fatalf("Build(%s) =\n%s\nwant\n%s", f, got, row.want)
				}
			})
		}
	}
}
