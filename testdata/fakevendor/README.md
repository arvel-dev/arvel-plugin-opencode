# Fake vendor

`fakevendor.mjs` is a minimal ACP **agent** (the side that `opencode acp` plays)
used as an offline stand-in. It speaks newline-delimited JSON-RPC 2.0 over
stdio, exactly like the real CLI, and never performs a model call.

The canonical offline fake used by `cmd/check-conformance` and the release-level
scenarios is `testdata/fakeagent` (Go), which also answers the `opencode` CLI
surface (`--version`, `auth list`, `models`). `fakevendor.mjs` remains a quick
manual probe.

It exists so tests and `arvel-plugin-check` can exercise the plugin's ACP client
without an OpenCode install, credentials or network access. The real CLI remains
the only source of recorded findings.

Run it directly and feed it one JSON-RPC line per line:

```sh
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"probe","version":"0"}}}' \
  | node testdata/fakevendor/fakevendor.mjs
```

Supported methods: `initialize`, `session/new`, `session/prompt`. Anything else
is answered with JSON-RPC `-32601` (method not found).
