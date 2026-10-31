// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestFilterMCP(t *testing.T) {
	stdio := acp.McpServer{Stdio: &acp.McpServerStdio{Name: "a", Command: "x"}}
	http := acp.McpServer{Http: &acp.McpServerHttpInline{Name: "b", Url: "https://x/v1"}}
	sse := acp.McpServer{Sse: &acp.McpServerSseInline{Name: "c", Url: "https://y/sse"}}

	got := filterMCP([]acp.McpServer{stdio, sse, http}, nil)
	if len(got) != 2 {
		t.Fatalf("filterMCP len = %d, want 2", len(got))
	}
	if got[0].Stdio == nil || got[0].Stdio.Name != "a" {
		t.Fatalf("first = %+v, want stdio a", got[0])
	}
	if got[1].Http == nil || got[1].Http.Name != "b" {
		t.Fatalf("second = %+v, want http b", got[1])
	}

	if filterMCP(nil, nil) != nil {
		t.Fatal("nil input must stay nil")
	}
}
