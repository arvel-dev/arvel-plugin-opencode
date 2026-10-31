// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// categoryFromKind maps an ACP tool kind to a permission category. O2 keeps
// the minimal mapping; O3 adds command text, paths and hosts.
func categoryFromKind(k *acp.ToolKind) protocol.PermissionCategory {
	if k == nil {
		return protocol.PermissionCategoryOther
	}
	switch *k {
	case acp.ToolKindExecute:
		return protocol.PermissionCategoryCommand
	case acp.ToolKindEdit, acp.ToolKindDelete, acp.ToolKindMove:
		return protocol.PermissionCategoryEdit
	case acp.ToolKindRead, acp.ToolKindSearch:
		return protocol.PermissionCategoryRead
	case acp.ToolKindFetch:
		return protocol.PermissionCategoryNetwork
	default:
		return protocol.PermissionCategoryOther
	}
}

// RequestPermission forwards an upstream permission request to the host and
// maps the decision back to the upstream option ids (O2 pass-through).
func (o *outputSink) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
	dec, err := o.out.RequestPermission(ctx, plugin.PermissionRequest{
		ToolCall: req.ToolCall,
		Options:  req.Options,
		Meta:     protocol.PermissionMeta{Category: categoryFromKind(req.ToolCall.Kind)},
	})
	if err != nil || dec.Cancelled {
		return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}
	}
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected(acp.PermissionOptionId(dec.OptionID))}
}
