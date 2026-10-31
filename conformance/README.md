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

Status at O0: the files describe the target contract. Only the `basic` level
(`arvel-plugin-check --level basic`) is expected to pass at O0; the session
steps stay red until O2 implements sessions.
