// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"regexp"
)

var (
	secretKeyRE   = regexp.MustCompile(`(?i)(^|[_-])(key|token|secret|password|authorization|api[_-]?key|bearer)($|[_-])`)
	secretValueRE = regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`)
	bearerRE      = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{16,}`)
)

// redactValue returns REDACTED when the key names a secret-bearing field.
func redactValue(key, v string) string {
	if secretKeyRE.MatchString(key) {
		return "REDACTED"
	}
	return sanitizeText(v)
}

func sanitizeText(s string) string {
	s = secretValueRE.ReplaceAllString(s, "sk-REDACTED")
	s = bearerRE.ReplaceAllString(s, "Bearer REDACTED")
	return s
}

// sanitize scrubs raw JSON bytes before they are written to a fixture. Invalid
// JSON is treated as text.
func sanitize(raw []byte) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		quoted, qerr := json.Marshal(sanitizeText(string(raw)))
		if qerr != nil {
			return []byte(`"unprintable frame"`)
		}
		return quoted
	}
	walk(&v)
	out, err := json.Marshal(v)
	if err != nil {
		return []byte(sanitizeText(string(raw)))
	}
	return out
}

func walk(v *any) {
	switch t := (*v).(type) {
	case map[string]any:
		for k := range t {
			var x any = t[k]
			if s, ok := x.(string); ok {
				t[k] = redactValue(k, s)
				continue
			}
			walk(&x)
			t[k] = x
		}
	case []any:
		for i := range t {
			walk(&t[i])
		}
	}
}
