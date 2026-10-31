# Conformance scenarios

These scenarios drive the plugin exactly as the Arvel host does: JSON-RPC over
stdio, starting with `initialize`, then `session/new` and `session/prompt`.

Run them at release level:

```sh
arvel-plugin-check --level release --manifest arvel.json \
  --scenarios ./conformance --test-env ./conformance-env.yaml -- ./bin/arvel-plugin-opencode
```

`conformance-env.yaml` is a template: replace `<opencodePath>` with an absolute
path to a pinned `opencode` binary before a run. The generic runner that fills
it in and supplies a fake vendor (`cmd/check-conformance`) arrives in O5
together with the remaining scenarios (`usage`, `resume`, `subagents`,
`skills`, `proxy`).

Status at O2: sessions are implemented. `basic-turn` and `resume-cancel`
describe the session contract (one `opencode acp` child per session, pass-through
updates, resume via `session/load`). The generic runner that fills
`conformance-env.yaml` and supplies a fake vendor (`cmd/check-conformance`)
arrives in O5 together with the remaining scenarios (`usage`, `subagents`,
`skills`, `proxy`, `cancel`).
