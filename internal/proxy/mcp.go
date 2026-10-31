// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"log/slog"

	acp "github.com/coder/acp-go-sdk"
)

// filterMCP drops SSE MCP servers (SPEC §5, §10; FINDINGS V5) and keeps stdio
// and HTTP servers. Each dropped server is logged at Warn.
func filterMCP(servers []acp.McpServer, log *slog.Logger) []acp.McpServer {
	if len(servers) == 0 {
		return servers
	}
	out := make([]acp.McpServer, 0, len(servers))
	for _, s := range servers {
		if s.Sse != nil {
			if log != nil {
				log.Warn("proxy: skipping SSE MCP server (unsupported)", "server", mcpName(s))
			}
			continue
		}
		out = append(out, s)
	}
	return out
}
