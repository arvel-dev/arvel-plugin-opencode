// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"os"
	"path/filepath"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/discover"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

var _ plugin.Discoverer = (*Provider)(nil)

// Discover answers _arvel/discover (PROTOCOL §7.9, SPEC §10). It reads the
// OpenCode config root and the listed projects; secrets never leave the plugin.
func (p *Provider) Discover(_ context.Context, params protocol.DiscoverParams) (protocol.DiscoverResult, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return emptyDiscover(), nil
	}
	configDir := filepath.Join(home, ".config", "opencode")
	return discover.Result(configDir, params.Kinds, params.Projects), nil
}

func emptyDiscover() protocol.DiscoverResult {
	return protocol.DiscoverResult{McpServers: []protocol.DiscoveredMcp{}, Skills: []protocol.DiscoveredSkill{}}
}
