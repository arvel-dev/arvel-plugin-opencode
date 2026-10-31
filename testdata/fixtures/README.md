# Recorded fixtures

Every directory here is one sanitized recording of the real OpenCode CLI or
its ACP server, produced by `cmd/record-fixtures`. Nothing in this tree is
hand-written vendor output.

## Contract

Each recording directory contains:

- `metadata.json` — `scenario`, `label`, `bin`, `version`, `os`, `arch`,
  `captured`, and the `OPENCODE_*`/`XDG_*`/proxy variables that shaped the run.
- `output.txt` — for CLI scenarios: the exact command, exit code, stdout and
  stderr, with secret-shaped values redacted.
- `transcript.jsonl` — for ACP scenarios: one JSON object per line,
  `{"direction":"client_to_server"|"server_to_client","message":{...}}`.
  Server-to-client requests (for example `session/request_permission`) are kept
  in the stream; `--approve` / `--approve-always` answer them with the selected
  option and the answer is recorded too.
- `index.json` at the fixtures root lists every recording (label, scenario,
  path, version, OS, arch, captured).

Raw, unsanitized transcripts go under `testdata/fixtures/raw/`, which is
git-ignored. They are never committed.

## Rules

- Redaction happens in memory before anything is written. A recording that
  still contains an `sk-…` value or a `Bearer …` token fails
  `go run ./cmd/record-fixtures verify`.
- A failed probe is evidence: a non-zero exit code or a JSON-RPC `error` is
  recorded verbatim (sanitized), not replaced with a success shape.
- RPC errors have a numeric `code` and a string `message`; provider errors also
  carry `data.errorName` and `data.service` (see `../../docs/FINDINGS.md`).
- `v1-*` recordings were made with an isolated `opencode-ai@1.19.5` binary and
  redirected `XDG_*` roots. `v2-*` recordings were made with `opencode v2.2.5`.
  `v1-1.3.15-*` and `v1-1.3.16-*` are the `minVersion` boundary probes.
- No credential file is read or copied. `opencode auth list` is the only auth
  surface; its output never includes a key.

## Adding a recording

```sh
./scripts/record-fixtures.sh record --scenario handshake --label v2-handshake
go run ./cmd/record-fixtures verify
```

Pass `--bin` to select a specific binary, `--cwd` to set the ACP session
working directory, and `--approve`/`--approve-always` to answer permission
prompts. See `go run ./cmd/record-fixtures record --help`.

## Environment

- `ARVEL_DEV=1` enables the development binary override; together with
  `ARVEL_OPENCODE_BIN` it selects the `opencode` binary to record against.
  Without `ARVEL_DEV`, the override is ignored and the harness resolves
  `opencode` from `$PATH`. Normal runs should use `--bin` instead.
- The harness records the `OPENCODE_*`, `XDG_*` and proxy variables present in
  the environment into `metadata.json`; it never records other variables.
