// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTestEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	if err := writeTestEnv(p, "/tmp/fakeopencode", flavours[1]); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `opencodePath: "/tmp/fakeopencode"`) {
		t.Fatalf("missing opencodePath:\n%s", s)
	}
	if !strings.Contains(s, `FAKE_OPENCODE_VERSION: "opencode v2.2.5"`) {
		t.Fatalf("missing version:\n%s", s)
	}
	if !strings.Contains(s, `FAKE_OPENCODE_FLAVOUR: "v2"`) {
		t.Fatalf("missing flavour:\n%s", s)
	}
	if !strings.Contains(s, `text: "HOLD"`) {
		t.Fatalf("missing HOLD prompt:\n%s", s)
	}
}
