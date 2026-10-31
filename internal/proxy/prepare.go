// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/occonfig"
	"github.com/arvel-dev/arvel-plugin-opencode/internal/upstream"
	"github.com/arvel-dev/arvel-sdk/protocol"
	"github.com/arvel-dev/arvel-sdk/protocol/skills"
)

var runSeq atomic.Int64

// nextRunID names the per-session run directory under the data dir.
func nextRunID() string { return fmt.Sprintf("r%d", runSeq.Add(1)) }

// instructionsFile renders instructions.md: the Arvel instructions, then the
// language line (SPEC §10). Empty when neither is set.
func instructionsFile(meta protocol.SessionMeta) string {
	var b strings.Builder
	if meta.Instructions != "" {
		b.WriteString(meta.Instructions)
		if !strings.HasSuffix(meta.Instructions, "\n") {
			b.WriteByte('\n')
		}
	}
	if meta.Language != "" {
		fmt.Fprintf(&b, "\nReply to the user in %s.\n", meta.Language)
	}
	return b.String()
}

// prepare computes the per-session artifacts: the instructions file and the
// inline config. Called once from New/Load.
func (s *Session) prepare() error {
	content := instructionsFile(s.meta)
	if s.cfg.DataDir != "" && content != "" {
		s.runDir = filepath.Join(s.cfg.DataDir, "run", nextRunID())
		if err := os.MkdirAll(s.runDir, 0o755); err != nil {
			return fmt.Errorf("proxy: mkdir run dir: %w", err)
		}
		p := filepath.Join(s.runDir, "instructions.md")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return fmt.Errorf("proxy: write instructions: %w", err)
		}
		s.inlineConfig = occonfig.Build(s.cfg.Flavour, s.meta, p)
	}
	if s.inlineConfig == nil {
		s.inlineConfig = occonfig.Build(s.cfg.Flavour, s.meta, "")
	}

	cleanup, err := skills.Materialize(s.cwd, []string{".opencode/skills"}, s.meta.Skills)
	if err != nil {
		return fmt.Errorf("proxy: materialize skills: %w", err)
	}
	s.skillsCleanup = cleanup
	return nil
}

// start spawns one child with the prepared inline config.
func (s *Session) start(ctx context.Context) (*upstream.Client, error) {
	return upstream.Start(ctx, upstream.Config{
		Bin:               s.cfg.Bin,
		CWD:               s.cwd,
		Flavour:           s.cfg.Flavour,
		LoadProjectConfig: s.cfg.LoadProjectConfig,
		InlineConfig:      string(s.inlineConfig),
		Proxy:             s.cfg.Proxy,
		Log:               s.cfg.Log,
		Redactor:          s.cfg.Redactor,
	})
}

// cleanupArtifacts removes the materialized skills and the run dir.
func (s *Session) cleanupArtifacts() {
	if s.skillsCleanup != nil {
		s.skillsCleanup()
		s.skillsCleanup = nil
	}
	if s.runDir != "" {
		_ = os.RemoveAll(s.runDir)
		s.runDir = ""
	}
}
