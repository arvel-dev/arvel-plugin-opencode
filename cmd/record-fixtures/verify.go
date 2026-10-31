// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	root := fs.String("out", "testdata/fixtures", "fixtures root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := filepath.Join(*root, "index.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("no fixtures yet")
		return nil
	}
	if err != nil {
		return err
	}
	var entries []indexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if len(entries) == 0 {
		fmt.Println("no fixtures yet")
		return nil
	}

	indexed := make(map[string]bool, len(entries))
	for _, e := range entries {
		indexed[filepath.ToSlash(e.Path)] = true
	}
	dirs, err := os.ReadDir(*root)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() || d.Name() == "raw" {
			continue
		}
		if !indexed[d.Name()] {
			return fmt.Errorf("%s is not in index.json: index it or delete it", filepath.Join(*root, d.Name()))
		}
	}

	var failed int
	for _, e := range entries {
		if err := verifyEntry(*root, e); err != nil {
			fmt.Printf("FAIL %s: %v\n", e.Label, err)
			failed++
			continue
		}
		fmt.Printf("ok   %s (%s)\n", e.Label, e.Scenario)
	}
	if failed > 0 {
		return fmt.Errorf("%d recording(s) failed verification", failed)
	}
	return nil
}

func verifyEntry(root string, e indexEntry) error {
	dir := filepath.Join(root, filepath.FromSlash(e.Path))
	metaData, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return err
	}
	if leaked(metaData) {
		return errors.New("metadata.json contains an unsanitized secret")
	}
	var md Metadata
	if err := json.Unmarshal(metaData, &md); err != nil {
		return fmt.Errorf("metadata.json: %w", err)
	}
	if md.Scenario == "" || md.Label == "" || md.Version == "" {
		return errors.New("metadata.json is missing scenario, label or version")
	}
	if md.Label != e.Label || md.Scenario != e.Scenario {
		return errors.New("index entry does not match metadata.json")
	}

	if data, err := os.ReadFile(filepath.Join(dir, "transcript.jsonl")); err == nil {
		if err := verifyTranscript(data); err != nil {
			return err
		}
		if leaked(data) {
			return errors.New("transcript contains an unsanitized secret")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	out, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		return errors.New("recording has neither transcript.jsonl nor output.txt")
	}
	if leaked(out) {
		return errors.New("output.txt contains an unsanitized secret")
	}
	return nil
}

func verifyTranscript(data []byte) error {
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var f frame
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			return fmt.Errorf("transcript line %d is not a frame: %w", i+1, err)
		}
		if f.Direction != "client_to_server" && f.Direction != "server_to_client" {
			return fmt.Errorf("transcript line %d has direction %q", i+1, f.Direction)
		}
		var v any
		if err := json.Unmarshal(f.Message, &v); err != nil {
			return fmt.Errorf("transcript line %d message is not JSON: %w", i+1, err)
		}
	}
	return nil
}

// leaked reports whether text still contains something that looks like a live
// credential.
func leaked(data []byte) bool {
	s := string(data)
	return secretValueRE.MatchString(s) || bearerRE.MatchString(s)
}
