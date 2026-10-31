// SPDX-License-Identifier: Apache-2.0

// Package proxy is the session bridge: host ACP ⇄ upstream ACP. It implements
// plugin.Session and plugin.ConfigOptioner for one OpenCode child per host
// session (SPEC §4, §7).
package proxy

import (
	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
)

// modelValue returns the upstream `model` config value for a flavour. v1
// carries the effort as a /variant suffix; v2 keeps the bare provider/model
// and sends effort as a separate config option (FINDINGS V2). An empty or
// "default" effort means "no explicit effort" (PROTOCOL §6.2 Auto).
func modelValue(f detect.Flavour, model, effort string) string {
	if model == "" {
		return ""
	}
	if f == detect.V1 && effort != "" && effort != "default" {
		return model + "/" + effort
	}
	return model
}

// modeValue returns the upstream `mode` config value (build|plan) for the
// connection's agent setting; "" maps to the OpenCode default "build".
func modeValue(agent string) string {
	if agent == "" {
		return "build"
	}
	return agent
}

// hasOption reports whether opts contains a select option with the given id.
func hasOption(opts []acp.SessionConfigOption, id string) bool {
	for _, o := range opts {
		if o.Select != nil && string(o.Select.Id) == id {
			return true
		}
	}
	return false
}
