// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"bytes"
	"io"
	"log/slog"
	"sync"

	"github.com/arvel-dev/arvel-sdk/redact"
)

// vendorStderr forwards child stderr lines to the plugin log at debug
// (PROTOCOL §10). Each line is redacted first. It is a no-op when log is nil.
func vendorStderr(log *slog.Logger, rd *redact.Redactor) io.Writer {
	if log == nil {
		return io.Discard
	}
	return &stderrWriter{log: log, rd: rd}
}

type stderrWriter struct {
	mu  sync.Mutex
	log *slog.Logger
	rd  *redact.Redactor
	buf bytes.Buffer
}

// maxStderrLine bounds one buffered vendor stderr line; a longer run is logged
// truncated and the buffer reset so a pathological stream cannot grow memory.
const maxStderrLine = 64 * 1024

func (w *stderrWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			if w.buf.Len() > maxStderrLine {
				line := string(w.buf.Next(maxStderrLine))
				if w.rd != nil {
					line = w.rd.String(line)
				}
				w.log.Debug("vendor stderr", "source", "vendor", "line", line, "truncated", true)
				w.buf.Reset()
			}
			break
		}
		line := string(w.buf.Next(i))
		_ = w.buf.Next(1) // drop the newline
		if w.rd != nil {
			line = w.rd.String(line)
		}
		w.log.Debug("vendor stderr", "source", "vendor", "line", line)
	}
	return n, nil
}
