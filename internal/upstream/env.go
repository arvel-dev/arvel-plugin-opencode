// SPDX-License-Identifier: Apache-2.0

// Package upstream is the ACP client side towards one `opencode acp` child.
package upstream

import (
	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/procx"
)

// Env builds the child environment (PROTOCOL §5.1, §11.4; FINDINGS V4/V9/V10):
// the minimal allow-list env, plus the inline config, plus the project-config
// switch, plus the flavour-correct skill exclusions, plus the sandbox egress
// proxy. An empty proxy or inline config is a no-op.
func Env(c Config) []string {
	b := procx.Env()
	if !c.LoadProjectConfig {
		b.Set("OPENCODE_DISABLE_PROJECT_CONFIG", "1")
	}
	if c.InlineConfig != "" {
		b.Set("OPENCODE_CONFIG_CONTENT", c.InlineConfig)
	}
	// Exclude foreign cross-tool skill roots where the CLI allows (FINDINGS V9):
	// v1 drops all external roots; v2 has no external switch and can only drop
	// .claude roots (the .agents root remains — documented in the README).
	if c.Flavour == detect.V1 {
		b.Set("OPENCODE_DISABLE_EXTERNAL_SKILLS", "1")
	} else {
		b.Set("OPENCODE_DISABLE_CLAUDE_CODE_SKILLS", "1")
	}
	b.Proxy(c.Proxy)
	return b.Build()
}
