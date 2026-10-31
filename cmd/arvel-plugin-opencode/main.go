// SPDX-License-Identifier: Apache-2.0

// Command arvel-plugin-opencode is the Arvel provider plugin for OpenCode.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/provider"
	"github.com/arvel-dev/arvel-sdk/plugin"
)

var version = "0.1.0" // overwritten by -ldflags at release

func main() {
	if err := plugin.Run(context.Background(), provider.New(), plugin.WithVersion("arvel-plugin-opencode", version)); err != nil {
		fmt.Fprintln(os.Stderr, err) // stderr only: stdout is the protocol channel
		os.Exit(1)
	}
}
