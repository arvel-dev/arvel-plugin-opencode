// SPDX-License-Identifier: Apache-2.0

// Package upstream is the ACP client side towards one `opencode acp` child.
package upstream

import "github.com/arvel-dev/arvel-sdk/procx"

// Env builds the child environment (PROTOCOL §5.1, §11.4; FINDINGS V4/V10):
// the minimal allow-list env, plus OPENCODE_DISABLE_PROJECT_CONFIG when the
// connection does not load the project's own config, plus the sandbox egress
// proxy variables. An empty proxy is a no-op.
func Env(loadProjectConfig bool, proxy string) []string {
	b := procx.Env()
	if !loadProjectConfig {
		b.Set("OPENCODE_DISABLE_PROJECT_CONFIG", "1")
	}
	b.Proxy(proxy)
	return b.Build()
}
