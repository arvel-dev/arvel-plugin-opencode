// SPDX-License-Identifier: Apache-2.0
// Command record-fixtures records sanitized OpenCode evidence for the plugin
// spikes. Run "record" to capture a scenario and "verify" to validate the
// committed fixtures offline. Recording requires the opencode binary; verify
// never runs it.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: record-fixtures <record|verify> [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "record":
		if err := runRecord(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "record-fixtures: "+err.Error())
			os.Exit(1)
		}
	case "verify":
		if err := runVerify(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "record-fixtures: "+err.Error())
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "record-fixtures: unknown subcommand %q\n", os.Args[1])
		os.Exit(2)
	}
}
