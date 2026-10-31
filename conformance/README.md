# Conformance scenarios

These scenarios drive the plugin exactly as the Arvel host does: JSON-RPC over
stdio, starting with `initialize`, then `session/new` and `session/prompt`.

`conformance-env.yaml` is a template. `go run ./cmd/check-conformance` builds the
plugin and the fake vendor, fills the template for v1 and v2, and runs the release
level for both flavours:

```sh
go run ./cmd/check-conformance
```

To run the checker directly against a real binary, fill in `opencodePath` in a
copy of `conformance-env.yaml` first:

```sh
arvel-plugin-check --level release --manifest arvel.json \
  --scenarios ./conformance --test-env ./conformance-env.yaml -- ./bin/arvel-plugin-opencode
```

The scenarios cover the session contract (`basic-turn`, `permission`,
`resume-cancel`, `usage`, `subagents`, `skills`, `proxy`) against the fake
vendor in `testdata/fakeagent`.
