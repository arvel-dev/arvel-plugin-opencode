// SPDX-License-Identifier: Apache-2.0

// Package occonfig builds the per-session OpenCode inline config (SPEC §6.1,
// FINDINGS V4). The JSON is passed to the child as OPENCODE_CONFIG_CONTENT.
package occonfig

import (
	"bytes"
	"encoding/json"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// Build returns the inline config JSON for one session. instructionsPath is the
// absolute path of the written instructions.md (empty omits the "instructions"
// key). The output is currently flavour-independent (FINDINGS V4: v1 and v2
// share the permission key set and both accept the "permission" object); the
// flavour parameter is kept so any future divergence goes through detect.Flavour.
func Build(f detect.Flavour, meta protocol.SessionMeta, instructionsPath string) []byte {
	cfg := map[string]any{}
	if instructionsPath != "" {
		cfg["instructions"] = []string{instructionsPath}
	}
	if perm := permissionObject(meta.Permissions, meta.Subagents); len(perm) > 0 {
		cfg["permission"] = perm
	}
	// All values are enum strings, protocol patterns and a fixed path; the only
	// way marshal can fail is an impossible value type, so ignore the error.
	b, _ := json.Marshal(cfg)
	return b
}

// permissionObject maps Arvel policy to the OpenCode "permission" object. Both
// flavours accept this object: v1 keeps it verbatim, v2 normalizes it to the
// "permissions" array (FINDINGS V4). Keys are per-tool, so their relative order
// does not matter; only the inner "bash" rules are order-sensitive.
func permissionObject(p *protocol.Permissions, sub *protocol.Subagents) map[string]any {
	if p == nil {
		p = &protocol.Permissions{}
	}
	out := map[string]any{}
	if p.Edit != "" {
		out["edit"] = string(p.Edit)
	}
	if p.Read != "" {
		out["read"] = string(p.Read) // read IS a key in both flavours (FINDINGS V4)
	}
	if p.Network != "" {
		out["webfetch"] = string(p.Network)
	}
	if p.Commands != nil {
		out["bash"] = bashRules(p.Commands)
	}
	// subagents.enabled:false denies the task tool (FINDINGS V4: the v2 tool is
	// "task", not "subagent").
	if sub != nil && !sub.Enabled {
		out["task"] = string(protocol.PermissionLevelDeny)
	}
	return out
}

// bashRules maps the command policy to the OpenCode "bash" rule object.
// OpenCode matches LAST rule first (FINDINGS V4), so the catch-all "*" goes
// first, then allow patterns, then deny patterns last (deny wins on overlap).
// The result is an orderedRules, because encoding/json sorts map keys and would
// destroy this ordering.
func bashRules(c *protocol.CommandPolicy) orderedRules {
	def := string(c.Default)
	if def == "" {
		def = string(protocol.PermissionLevelAsk)
	}
	r := orderedRules{}
	r.add("*", def)
	for _, pat := range c.Allow {
		r.add(pat, string(protocol.PermissionLevelAllow))
	}
	for _, pat := range c.Deny {
		r.add(pat, string(protocol.PermissionLevelDeny))
	}
	return r
}

// orderedRules is a rule object whose MarshalJSON preserves insertion order.
type orderedRules struct{ pairs [][2]string }

func (o *orderedRules) add(k, v string) { o.pairs = append(o.pairs, [2]string{k, v}) }

func (o orderedRules) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range o.pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(p[0])
		v, _ := json.Marshal(p[1])
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
