# Bootstrap Findings

Evidence for milestone **O0 — Bootstrap & spikes**. This file answers the
VERIFY list V1–V10 from `SPEC.md` §13 for both OpenCode flavours. Nothing here
is inferred from documentation alone: each claim cites sanitized command output,
a recorded ACP transcript, or a pinned source location. Where the docs and the
binary disagree, the binary wins and the disagreement is recorded.

- OpenCode **v2** (`2.2.5`) and **v1** (`1.19.5`) are both observed on this host.
- **V8 (Windows)** is source-verified only; native verification is deferred to
  the CI `windows-latest` matrix, matching the other Arvel provider plugins.
- Items that would need a capability the host does not provide are marked
  **deferred** with the exact reason; none of them is guessed.

## Evidence and authority

- Recorded on 2026-10-31 on macOS 26.5.2, arm64, Go 1.27.3, SDK `arvel-sdk`
  v0.2.1.
- **v2 subject:** `opencode v2.2.5`, installed by Homebrew at
  `/opt/homebrew/bin/opencode` (symlink to
  `../Cellar/opencode-v2/2.2.5/bin/opencode`). The npm package
  `@opencode/cli@2.2.5` was also installed into an isolated prefix; it exposes
  both `opencode` and `opencode2` and reports the same version.
- **v1 subject:** `opencode-ai@1.19.5` (the latest v1 release), unpacked under
  `/tmp/opencode-v1`; every v1 command used the absolute path
  `/tmp/opencode-v1/node_modules/.bin/opencode`. v1 probes run with all four
  `XDG_*` roots redirected to `/tmp/oc-v1/env` because v1 and v2 share the
  default data directory and cannot share its database (see V10).
- **v1 boundary probes:** `opencode-ai@1.3.15`, `1.3.16`, `1.3.17` and `1.4.0`
  in isolated prefixes, used only to locate the `configOptions` boundary and
  the unknown-model behaviour (V1). The committed boundary fixtures are
  `v1-1.3.15-session-new`, `v1-1.3.16-session-new`, `v1-1.3.16-config-error`.
- **v2 source pin:** tag `v2.2.5` is commit
  `9bbb8ce18c80f099e0e448775f59d965ead40a21`. Source references below are to
  that commit.
- **v1 source:** the v1 built-in `customize-opencode` skill ships inside the
  1.19.5 binary and documents v1's config surface; it was read through
  `opencode debug skill` (see V9 for why that output is not committed).
- Sanitized transcripts and per-recording metadata:
  `testdata/fixtures/index.json`. Every recording is re-checked offline by
  `go run ./cmd/record-fixtures verify`, which also fails on an unredacted
  `sk-…` or `Bearer …` value.
- No credential file is read or copied. `opencode auth list` is the only
  credential surface used; the SQLite credential table is never opened.

### CLI surface observed

| Command | v2.2.5 | v1.19.5 |
|---|---|---|
| `opencode --version` | `opencode v2.2.5` (`v2-version`) | bare `1.19.5` (`v1-version`) |
| `opencode acp` | ACP over newline-delimited JSON-RPC on stdio (`v2-handshake`) | same (`v1-handshake`) |
| `opencode acp --help` | only `--login`; rejects `--standalone` | `--cwd`, `--port`, `--hostname`, `--mdns`, `--mdns-domain`, `--cors`, `--pure`, `--print-logs`, `--log-level` |
| `opencode models` | one `provider/model` per line, sorted (`v2-models`) | one per line, **not sorted**; extra host-configured model (`v1-models`) |
| `opencode models --verbose` | **rejected**: `Unrecognized flag: --verbose` (`v2-models-verbose`) | accepted: per-model JSON with `api.url`, `variants` (`v1-models-verbose`) |
| `opencode auth list` | tabular row per provider, status `stored` (`v2-auth-list`) | box-drawing list plus `auth.json` path and count (`v1-auth-list`, `v1-auth-list-stored`) |
| `opencode debug paths` | includes `db` (`v2-debug-paths`) | no `db` line (`v1-debug-paths`) |
| `opencode debug config` | merged config documents; `apiKey` masked `***` (`v2-debug-config`) | merged config object; provider options print as `{}` (`v1-debug-config`) |
| `opencode debug agents` | normalized agents with `permissions` array (`v2-debug-agents`) | present in `debug --help`; not probed |
| `opencode debug skill` | **absent**: `Unknown subcommand "skill"` (`v2-debug-skill`) | present and dumps full skill bodies (`v1-debug-help`) |

`opencode debug config` is only trustworthy in a private instance. Without
`--standalone`, the v2 CLI queries the background service and ignores
`OPENCODE_CONFIG_CONTENT`; `opencode --standalone debug config` and
`opencode acp` are private instances and honour it. v1 has no background
service split: `opencode acp` and the CLI both read the inline config.

## V1 — Binary, version, install, `minVersion`

**v2 (observed).** The binary is `opencode`; `opencode --version` prints
`opencode v2.2.5` and exits 0. The flavour boundary (`major >= 2` → v2) can be
parsed from that string.

Install commands for v2 (v2 docs, checked 2026-10-31):

- macOS/Linux: `curl -fsSL https://opencode.ai/v2/install | bash`
- Homebrew: `brew install anomalyco/tap/opencode-v2`
- npm: `npm install -g @opencode/cli`
- Bun/pnpm: `bun install -g --trust @opencode/cli` /
  `pnpm add -g --allow-build=@opencode/cli @opencode/cli`
- Windows package managers are **not supported**; the v2 docs point at the
  standalone `opencode-windows-x64.zip` / `opencode-windows-arm64.zip`.
  `npm install -g opencode-ai` installs **v1**, not v2.

The npm v2 package's `package.json` declares two bins:
`{"opencode": "bin/opencode.exe", "opencode2": "bin/opencode.exe"}`. In an
isolated npm prefix both shims exist and `opencode2 --version` prints
`opencode v2.2.5`; `opencode2` speaks ACP with the same capabilities
(`v2-npm-opencode2-handshake`). This is the only side-by-side install path:
v1's npm package exposes only `opencode`, so a global install of both packages
collides on that name, while `opencode2` stays unambiguous.

**v1 (observed).** The binary is also `opencode`; `--version` prints the bare
version `1.19.5` (`v1-version`). Detection must therefore parse the first
semver token rather than expect an `opencode v` prefix. v1 install:

- npm: `npm install -g opencode-ai@1.19.5` (`opencode-ai` latest is 1.19.5)
- the package's `bin` field is `{"opencode": "bin/opencode.exe"}`; on macOS the
  `.exe` name is only a filename, the file is a Mach-O arm64 executable.

v1 has no isolated binary path from the vendor: it reads the same
`~/.config/opencode`, `~/.local/share/opencode`, `~/.cache/opencode` and
`~/.local/state/opencode` roots as v2 (V10). Only `XDG_*` redirection keeps
the probes separate.

**`minVersion` = `1.3.16` (both flavours).** The manifest has one `requires`
entry for the one `opencode` binary, so its floor must accept the oldest v1
that satisfies the plugin's ACP contract. The contract needs
`session/new` to return `configOptions` with categories `model` and `mode`:

| Version | `session/new` result |
|---|---|
| 1.3.15 and older | legacy `models` + `modes` only; no `configOptions` (`v1-1.3.15-session-new`) |
| **1.3.16** | `configOptions` **plus** legacy `models`/`modes` (`v1-1.3.16-session-new`) |
| 1.19.5 (latest) | `configOptions` only (`v1-session-new`) |

`1.3.16` is therefore the first supportable v1 release. Two caveats belong to
the compatibility notes, not to the floor:

- On 1.3.16 and 1.4.0, `session/set_config_option` with an unknown model is
  **accepted silently**: the response is the unchanged `configOptions` with
  `currentValue: "nonexistent/nonexistent"` (`v1-1.3.16-config-error`), while
  1.19.5 returns `-32602` `model not found` (`v1-config-error`). The plugin
  must validate model ids against the vendor list itself and not rely on the
  error.
- On 1.3.16/1.3.17 the `model` option values carry the effort variant as a
  third path segment (for example `opencode/big-pickle/high`, probe recorded on
  1.3.17). PROTOCOL allows `provider/model[/variant]`, so values are passed
  through opaquely; do not assume exactly two segments.

**Evidence:** `v1-version`, `v1-1.3.15-session-new`, `v1-1.3.16-session-new`,
`v1-1.3.16-config-error`, `v2-version`, `v2-npm-opencode2-handshake`.

## V2 — Model listing and variant naming

**v2 (observed).** `opencode models` prints one `provider/model` per line,
sorted by provider, with no headers, variants or cost:

```
deepseek/deepseek-flash
deepseek/deepseek-v4-pro
opencode/beam-free
opencode/big-pickle
opencode/deepseek-v4.1-flash-free
opencode/flint-40b-free
opencode/glm-5.3-flash-free
opencode/mimo-v2.6-flash-free
opencode/nemotron-3.5-lightning-free
opencode/nemotron-3.5-super-vl-free
```

The list reflects the providers configured on the machine. `--refresh` is
accepted; `--verbose` is not (CLI table above).

**v2 variants are not in `opencode models`.** They arrive over ACP from
`session/new` (and `session/load`) as `configOptions`:

| `id` | `category` | Observed values | Default |
|---|---|---|---|
| `model` | `model` | every `provider/model`, displayed as `provider/Label` | `opencode/flint-40b-free` |
| `effort` | `thought_level` | `low`, `medium`, `high`, `xhigh`, `max`, `default` | `default` |
| `mode` | `mode` | `build`, `plan` | `build` |

The effort vocabulary matches PROTOCOL's enum plus OpenCode's `default`
("model default"), which must be mapped to "no explicit effort". Model
selection is `session/set_config_option` with `configId: "model"` and value
`provider/model`; effort uses the same call with `configId: "effort"`.

**v1 (observed).** `opencode models` prints one `provider/model` per line but
is **not sorted**: all `opencode/*` models appear first and the configured
`deepseek/*` providers follow (`v1-models`). Do not rely on order or on a
provider grouping.

v1 has the `--verbose` flag that v2 lacks. For each model it prints the
`provider/model` line followed by a pretty-printed JSON object:

```json
{
  "id": "beam-free", "providerID": "opencode", "name": "Beam Free",
  "api": {"id": "beam-free", "url": "https://opencode.ai/inference/openai/v1",
          "npm": "@ai-sdk/openai-compatible"},
  "status": "active", "cost": {"input": 0, "output": 0},
  "limit": {"context": 1048576, "output": 131072},
  "capabilities": {"reasoning": true, ...},
  "variants": {"low": {"reasoningEffort": "low"}, ...}
}
```

The `variants` map is the v1 source of truth for effort support. Observed keys
include `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; `none` and
`minimal` have no PROTOCOL effort equivalent and should not be offered as
protocol options. `api.url` is the v1 source for provider base URLs (V10).

**v1 ACP options.** `session/new` returns only `model` and `mode` — there is no
`effort` config option in 1.19.5 (`v1-session-new`). Effort can still be set:

- the documented path (SPEC §5) is a `/variant` suffix on the model value, and
- `session/set_config_option` with `configId: "effort"` is accepted by 1.19.5
  for models that advertise the variant (`v1-effort`), and rejected with
  `-32602` `{"effort":"high"}` for models that do not (`v1-effort-error`).

The plugin should prefer the `/variant` suffix (portable across the v1 line)
and use the `effort` config id only when the vendor advertises the matching
variant.

**Evidence:** `v1-models`, `v1-models-verbose`, `v1-session-new`, `v1-effort`,
`v1-effort-error`, `v2-models`, `v2-models-verbose`, `v2-session-new`.

## V3 — Credential listing

**v2 (observed).** `opencode auth list` exits 0 and prints one row per
authenticated provider:

```
DeepSeek  DeepSeek                    stored
```

The row carries a display name, a provider identifier and a status word
(`stored`); empty output means no provider is authenticated. `opencode auth ls`
is the short alias. Credentials live in the SQLite `credential` table under
`~/.local/share/opencode/opencode.db`; the v1-style `auth.json` does not exist
for v2 (`v2-debug-paths`). The plugin must never open either store.

**v1 (observed).** `opencode auth list` prints a box-drawing list that includes
the credential file path and the count:

```
┌  Credentials /tmp/oc-v1/env/data/opencode/auth.json
│
└  0 credentials
```

With one stored credential it prints the provider name and type instead:

```
●  DeepSeek api
└  1 credentials
```

`auth.json` is a JSON object keyed by provider id with `{type, key}` entries
(it was synthesized for the probe and deleted afterwards; the fixture shows
only the listing, never the key). v1 has no SQLite credential table. Because v1
and v2 share the default data directory, `auth.json` and `opencode.db` can
coexist, but their credential stores are independent — a login in one flavour
is not visible to the other.

**Evidence:** `v2-auth-list`, `v1-auth-list`, `v1-auth-list-stored`.

## V4 — Inline config, permissions, instructions, project config

**v2 (observed).** `OPENCODE_CONFIG_CONTENT` is the inline-config channel and
works over ACP. Setting the model through it changed the `model` option of a
fresh `session/new` (`v2-inline-config`). A file named in `instructions` was
delivered to the model: with
`OPENCODE_CONFIG_CONTENT='{"instructions":["INSTR.md"]}'` and `INSTR.md`
containing `PINEAPPLE-42`, the model answered `PINEAPPLE-42`
(`v2-instructions`). The variable is ignored by CLI commands that talk to the
background service; probe with `opencode --standalone debug config` or over
ACP.

The documented key is `permission`; the plugin may also send the normalized
ordered form `permissions`. Both are accepted and `permission` is normalized:

```json
{"permission":{"*":"ask","bash":{"*":"ask","git *":"allow","rm *":"deny"},"edit":"deny"}}
```

became, in `v2-permissions-config`:

```json
{"permissions":[
  {"action":"*","resource":"*","effect":"ask"},
  {"action":"shell","resource":"*","effect":"ask"},
  {"action":"shell","resource":"git *","effect":"allow"},
  {"action":"shell","resource":"rm *","effect":"deny"},
  {"action":"edit","resource":"*","effect":"deny"}
]}
```

Key facts:

- Effects are `allow`, `ask`, `deny`; a whole-map string shorthand is
  documented.
- The vendor tool vocabulary is `read`, `edit`, `glob`, `grep`, `bash`, `task`,
  `todowrite`, `skill`, `lsp`, `question`, `webfetch`, `websearch`,
  `external_directory`, `doom_loop`. This corrects SPEC §6.1: v2 config keys are
  **`bash`**, **`edit`** and **`task`**, not `shell`/`subagent`. The normalized
  array uses the action name `shell` for `bash` rules.
- **Rule order is last-match-wins**, with `*`/`?` patterns. Put broad rules
  first and narrow rules last. SPEC §6.1's "first-match-wins ordered array"
  must be corrected in the config builder and evaluator.
- `edit` covers `edit`, `write` and `patch`. `read` defaults to allow except
  `*.env`/`*.env.*`, which ask. `external_directory` and `doom_loop` default to
  ask. `read` **is** a v2 and v1 permission key; the SPEC's "no read switch in
  v1" is wrong (see below).
- Per-agent permissions live under `agent.<name>.permission` and merge with the
  global rules; agent rules take precedence.

**Permission prompt and "Always allow" (v2).** A bash call under
`{"bash":{"*":"deny","echo *":"ask"}}` produced
`session/request_permission` with three options:

```
once   Allow once     allow_once
always Always allow   allow_always
reject Reject         reject_once
```

(`v2-permission-run1`). Answering `always` **persisted a rule** into the v2
database: a `shell|echo *` row appeared in the `permission` table, scoped to
the project (probe with `sqlite3` on 2026-10-31; the row was deleted after the
probe). This contradicts the v2 docs, which describe `always` as lasting "for
the rest of the current session". The consequence for Arvel is exactly SPEC
§6.2's rule: never forward the host's "always" to the vendor's `always`;
answer `once` upstream and keep Arvel's own session-local allow list.

**Project config.** `OPENCODE_DISABLE_PROJECT_CONFIG=1` removes the project's
`opencode.json` document from the merged config (`v2-debug-config` vs
`v2-debug-config-noproject`); the undocumented alias
`OPENCODE_CONFIG_PROJECT_DISABLE=1` did the same in a standalone probe. The
documented variable also disables project instruction files.

**v1 (observed).** `OPENCODE_CONFIG_CONTENT` works over ACP and in CLI
commands. A model object in the inline config changed the `session/new` model
option to `opencode/mimo-v2.6-flash-free` (`v1-inline-config`), and
`instructions: ["INSTR.md"]` delivered `PINEAPPLE-42` to the model
(`v1-instructions`).

v1 preserves the `permission` **object** verbatim (no normalization to an
array). With
`{"permission":{"*":"ask","bash":{"*":"ask","git *":"allow"},"edit":"deny"}}`
the merged config prints exactly that object (`v1-permissions-config`). v1 has
no `permissions` array form.

- `read` **is** accepted by 1.19.5: `{"permission":{"read":"deny"}}` appears
  verbatim in `opencode debug config` (`v1-permission-read`). This corrects
  SPEC §6.1's claim that v1 has no read switch. Whether every v1 tool honours
  it was not exercised end-to-end; treat it as an accepted config key.
- v1's built-in `customize-opencode` skill documents the same key set and the
  same last-match-wins, broad-first ordering.
- `OPENCODE_DISABLE_PROJECT_CONFIG=1` removes the project config document
  (`v1-debug-config-project` vs `v1-debug-config-noproject`).

**"Always allow" (v1).** The v1 prompt offers the same `once`/`always`/
`reject` options (`v1-permission-run1`). Answering `always`
(`v1-permission-always-run1`) did **not** suppress the request in a second
process with the same project and config (`v1-permission-always-run2` asked
again), while answering `once` also asked again (`v1-permission-run2`). v1
therefore does not persist `always` across processes on this host; do not rely
on that difference, and keep Arvel's own allow list for both flavours.

**Evidence:** `v2-inline-config`, `v2-instructions`, `v2-permissions-config`,
`v2-debug-config`, `v2-debug-config-noproject`, `v2-debug-agents`,
`v2-permission-run1`, `v1-inline-config`, `v1-instructions`,
`v1-permissions-config`, `v1-permission-read`, `v1-debug-config-project`,
`v1-debug-config-noproject`, `v1-permission-run1`, `v1-permission-run2`,
`v1-permission-always-run1`, `v1-permission-always-run2`.

## V5 — Resume: `loadSession` and `session/load`

**v2 (observed).** `initialize` advertises:

```json
{"agentCapabilities":{
  "loadSession": true,
  "mcpCapabilities": {"http": true, "sse": false},
  "promptCapabilities": {"embeddedContext": true, "image": true},
  "sessionCapabilities": {
    "additionalDirectories": {}, "close": {}, "delete": {},
    "fork": {}, "list": {}, "resume": {}
  },
  "_meta": {"opencode/child-session-updates": true}
}}
```

A real `session/new` followed by `session/load` with the same `sessionId`,
`cwd` and `mcpServers` succeeded and returned the same `configOptions`
(`model`, `effort`, `mode`) with current values (`v2-resume`). The plugin can
use `session/load` for idle-child resume in O2; `sessionCapabilities.resume`
also exists but is not the documented ACP method, so prefer `session/load`
while `loadSession` is true. SSE is advertised as **false** in v2, matching
SPEC's "skip SSE".

**v1 (observed).** `initialize` advertises `loadSession: true`,
`mcpCapabilities: {http: true, sse: true}`,
`promptCapabilities: {embeddedContext: true, image: true}`,
`sessionCapabilities: {close, fork, list, resume}` (no
`additionalDirectories`, no `delete`), `agentInfo: {name: "OpenCode",
version: "1.19.5"}`, `protocolVersion: 1` (`v1-handshake`). There is no
`_meta` object at all; the 1.19.5 handshake no longer carries the
`_meta.opencode.version` that 1.0.0 had (1.0.0 probe).

`session/load` succeeds and returns **only** `configOptions` (`model`, `mode`)
— no `sessionId` echo (`v1-resume`). On 1.3.16 the same call additionally
carries `_meta.opencode.{modelId, variant, availableVariants}` (probe). The
plugin must not require a `sessionId` in the `session/load` result.

Although v1 advertises SSE, SPEC's decision stands: pass only stdio and HTTP
MCP servers and skip SSE with a warning.

**Evidence:** `v2-handshake`, `v2-resume`, `v2-session-new`,
`v2-npm-opencode2-handshake`, `v1-handshake`, `v1-resume`, `v1-session-new`.

## V6 — Usage, cost and provider errors

**v2 (observed).** `session/prompt` returns usage as a **top-level field**:

```json
{"stopReason":"end_turn","_meta":{},
 "usage":{"inputTokens":11312,"outputTokens":98,"thoughtTokens":149,
          "cachedReadTokens":11904,"totalTokens":23463}}
```

`thoughtTokens` appears only when the model produced reasoning. A live
`usage_update` notification carries context and cost:

```json
{"sessionUpdate":"usage_update","used":12970,"size":200000,
 "cost":{"amount":0,"currency":"USD"}}
```

`size` is the context window, `used` is current context, and `cost` was `0`
USD for the free model. There is no cost field in the prompt result.

**v1 (observed).** The same top-level `usage` shape, with `_meta: {}` and no
`thoughtTokens` in any observed run — including runs that emitted
`agent_thought_chunk` frames (`v1-prompt`, `v1-subagent`, `v1-skills`):

```json
{"_meta":{},"stopReason":"end_turn",
 "usage":{"inputTokens":9540,"outputTokens":60,
          "cachedReadTokens":1920,"totalTokens":11520}}
```

`usage_update` has the identical shape (`used`, `size`, `cost`).

**Errors observed over ACP:**

| Case | v2.2.5 | v1.19.5 |
|---|---|---|
| Unknown model | `-32602` `Invalid params: model not found: nonexistent/nonexistent`, data `{providerId, modelId}` (`v2-config-error`) | same code/message and data (`v1-config-error`); **1.3.16 accepts it silently** (`v1-1.3.16-config-error`) |
| Bad effort | not probed | `-32602` `Invalid params: effort not found: high`, data `{effort}` (`v1-effort-error`) |
| Provider transport | `-32603` `Internal error: ECONNREFUSED: …`, data `{errorName: "provider.transport", service: "session"}` (`v2-proxy`) | `-32603` `Internal error: Cannot connect to API: ECONNREFUSED: …`, data `{errorName: "APIError", service: "session"}` (`v1-proxy`) |
| Free quota exhausted | `-32603` `Internal error: Free usage limit reached. Connect a provider or subscribe to OpenCode Go…`, data `{errorName: "provider.free-limit", service: "session"}` (`v2-free-limit-error`) | not observed |

`data.errorName` plus `data.service` is the stable discriminator. The two
flavours use different transport names (`provider.transport` vs `APIError`), so
map both. `provider.free-limit` is a distinct quota signal that O4 can surface
without inventing a rate-limit code. A true HTTP `429`/rate-limit response was
not produced in O0; O4 must map it from a real failure.

**Evidence:** `v2-prompt`, `v2-subagent`, `v2-config-error`, `v2-proxy`,
`v2-free-limit-error`, `v1-prompt`, `v1-subagent`, `v1-config-error`,
`v1-effort-error`, `v1-proxy`, `v1-1.3.16-config-error`.

## V7 — Subagent (task) work over ACP

**v2 (observed).** A prompt that spawned one subagent produced:

1. `tool_call` with `title: "subagent"`, `kind: "think"`, a `toolCallId`; then
   `tool_call_update` with `status: "in_progress"` and
   `rawInput: {"agent":"general","description":"…","prompt":"…"}`.
2. The child's `agent_message_chunk` carries
   `_meta["opencode/child-session"] = {"depth":1,"id":"ses_…","parentID":"ses_…","title":"…"}`.
3. The parent `tool_call_update` finishes with
   `content: [{type:"content",content:{type:"text",text:"<subagent sessionID=\"…\" state=\"completed\">…</subagent>"}}]`.
4. The parent `session/prompt` result's `usage` aggregates the child's tokens
   (parent output 98 vs `totalTokens` 23463).

`_meta["opencode/child-session-updates"]: true` in the handshake announces
this attribution. Forward child chunks keyed by
`child-session.id`/`parentID`, and do not double-count child tokens when
aggregating.

**v1 (observed).** The same flow uses the `task` tool:

1. `tool_call` with `title: "task"`, `kind: "think"`; a `tool_call_update`
   carries `rawInput: {"description":…,"prompt":…,"subagent_type":"general"}`.
2. There is **no `_meta`** on any frame: v1 does not tag child-session updates.
   Child text is not distinguishable from parent text in the stream.
3. The completed tool content is
   `<task id="ses_…" state="completed">\n<task_result>\nsub-ok\n</task_result>\n</task>`.
4. The parent `usage` again aggregates the child (prompt output 4 vs
   `totalTokens` 12306).

The plugin must parse the `<task>`/`<task_result>` envelope for v1 (and
`<subagent>` for v2) and cannot rely on child-session metadata in v1. If child
tokens matter, they are only available through the aggregate `usage`.

**Evidence:** `v2-subagent`, `v1-subagent`.

## V8 — Windows (source-verified; native deferred)

Native Windows is **not tested** on this macOS host. The following is
source-verified from the npm registry metadata, the v2 docs (checked
2026-10-31) and the pinned v2 source; ACP execution on Windows is deferred to
the CI `windows-latest` matrix.

**Binary names and shims.** Both packages' `package.json` `bin` fields were
read from the registry:

| Package | `bin` |
|---|---|
| `opencode-ai@1.19.5` (v1) | `{"opencode": "bin/opencode.exe"}` |
| `@opencode/cli@2.2.5` (v2) | `{"opencode": "bin/opencode.exe", "opencode2": "bin/opencode.exe"}` |

On Windows, npm installs the target `opencode.exe` and generates the shims
`opencode.cmd` and `opencode.ps1` from the `bin` key; v2 therefore also gets
`opencode2.cmd`/`opencode2.ps1`. The executable itself is the same file for
both names.

**Install.** v1: `npm install -g opencode-ai@1.19.5` (npm on Windows is
supported for v1). v2: package managers are **not supported**; use the
standalone `opencode-windows-x64.zip` / `opencode-windows-arm64.zip` (the
manifest's `install.windows` points at the x64 zip). A user who needs both
flavours on Windows can install v1 with npm and unpack v2 separately.

**Paths.** v2's path roots are XDG-based on every platform
(`packages/util/src/global-roots.ts` at the pinned commit). With no `XDG_*`
variables on Windows this yields
`C:\Users\<user>\.local\share\opencode`,
`C:\Users\<user>\.local\state\opencode`,
`C:\Users\<user>\.cache\opencode` and
`C:\Users\<user>\.config\opencode` — **not** `%APPDATA%`. v1 uses the same
scheme. `OPENCODE_CONFIG_DIR` overrides only the config root. v2 also keeps
`opencode.db` under `data`; v1 keeps `auth.json` there.

**ACP over stdio.** ACP is newline-delimited JSON-RPC on stdio, which is
platform-independent in the implementation, but this was not executed on
Windows. No Windows-specific flag or transcript is claimed. The acceptance
check is: start the installed shim with `acp`, complete
`initialize`/`session/new`, and confirm the same frame shapes as
`v1-handshake`/`v2-handshake`.

## V9 — Skill directories and exclusion

**v2 (observed).** v2 reads project and global skill directories, including
cross-tool roots:

| Scope | Roots |
|---|---|
| Project | `.opencode/skills/`, `.claude/skills/`, `.agents/skills/` |
| Global | `~/.config/opencode/skills/`, `~/.claude/skills/`, `~/.agents/skills/` |

The project probe had `.opencode/skills/alpha-skill` and
`.claude/skills/beta-skill`; both were visible to the model (`v2-skills`).
Skills present under both `~/.claude/skills` and `~/.agents/skills` were listed
once, so discovery is de-duplicated by name.

Exclusion switches that work on 2.2.5:

- `OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1` removes `.claude/skills` discovery
  (project `beta-skill` disappears) but leaves `.agents` roots and the global
  `.claude` copies that are also present under `.agents`
  (`v2-skills-no-claude`).
- `OPENCODE_DISABLE_CLAUDE_CODE=1` disables `.claude` prompts **and** skills;
  the same observation holds (`v2-skills-no-claude-code`).
- `OPENCODE_DISABLE_EXTERNAL_SKILLS` is **documented but absent from the 2.2.5
  binary**: the variable does not appear in the binary's environment strings
  and setting it changed nothing (`v2-skills-no-external`).
- There is no switch for `.agents/skills` in 2.2.5. Full isolation requires
  redirecting `HOME`/`XDG_*` or accepting the cross-tool roots.
- `opencode debug skill` is absent (`v2-debug-skill`), so the inventory was
  observed through a model prompt.

**v1 (observed).** The same roots plus a built-in skill:

| Scope | Roots |
|---|---|
| Project | `.opencode/skills/`, `.claude/skills/` |
| Global | `~/.config/opencode/skills/`, `~/.claude/skills/`, `~/.agents/skills/` |
| Built-in | `customize-opencode`, always present |

The same probe listed `alpha-skill`, `beta-skill`, `customize-opencode` and the
global cross-tool skills (`v1-skills`).

- `OPENCODE_DISABLE_EXTERNAL_SKILLS=1` removes **all** external roots
  (project and global `.claude`, `.agents`); only project `.opencode/skills`
  plus the built-in `customize-opencode` remain
  (`v1-skills-no-external`).
- `OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1` removes only `.claude` roots;
  `.agents` skills remain (`v1-skills-no-claude`).

`opencode debug skill` exists in v1 (`v1-debug-help`) and prints every skill's
name, description, location **and full body** — 131 KB on this host because it
includes global cross-tool skills. That output must never be logged or
committed by the plugin: it is user content, not metadata. For O3,
`.opencode/skills` remains the canonical delivery root; the other roots are
foreign and can add skills the user did not configure for Arvel.

**Evidence:** `v2-skills`, `v2-skills-no-claude`,
`v2-skills-no-claude-code`, `v2-skills-no-external`, `v2-debug-skill`,
`v2-debug-help`, `v1-skills`, `v1-skills-no-external`,
`v1-skills-no-claude`, `v1-debug-help`.

## V10 — Proxy, paths and provider base URLs

**Proxy (both flavours, observed).** With
`HTTPS_PROXY=http://127.0.0.1:1` and `ALL_PROXY=http://127.0.0.1:1`, a prompt
failed with `-32603` and `ECONNREFUSED`; the recording metadata preserves the
proxy variables. v2 reports `errorName: "provider.transport"` (`v2-proxy`), v1
reports `errorName: "APIError"` (`v1-proxy`). Provider HTTP therefore goes
through the standard proxy variables, so the Arvel sandbox proxy can enforce
egress. `NO_PROXY` passes through unchanged.

**Paths (observed).** v2 `debug paths` on macOS:

| Key | Path |
|---|---|
| `home` | `/Users/<user>` |
| `data` | `~/.local/share/opencode` |
| `cache` | `~/.cache/opencode` |
| `config` | `~/.config/opencode` |
| `state` | `~/.local/state/opencode` |
| `tmp` | `$TMPDIR/opencode` |
| `bin` | `~/.cache/opencode/bin` |
| `log` | `~/.local/share/opencode/log` |
| `repos` | `~/.local/share/opencode/repos` |
| `db` | `~/.local/share/opencode/opencode.db` |

v1 `debug paths` prints the same roots except `db` (`v1-debug-paths`; the
fixture shows the redirected `XDG_*` values, and the same scheme applies with
the defaults). The pinned v2 source (`packages/util/src/global-roots.ts`)
confirms the XDG defaults on every OS, including Windows (V8).

`arvel.json` `sandbox.vendorPaths` therefore covers config/data/state/cache for
reading and data/state/cache for writing; `bin` is inside cache and
`db`/`log`/`repos` inside data.

**Shared data directory is not shared state (observed).** v1 and v2 use the
same default data root but incompatible persistence: v2 initialises
`opencode.db`, v1 expects its JSON layout. Running v1 against a data root that
v2 has initialised fails before any session starts:

```
Error: Unexpected error
Database is not empty and has no session table
```

(`v1-shared-datadir`). This is why every v1 spike above redirects `XDG_*` and
why the plugin must isolate v1 state (or refuse to start) when the user's data
root belongs to the other flavour.

**Provider base URLs without secrets.**

- v2: `opencode --standalone debug config` returns the merged config with
  provider `apiKey` replaced by `***` and `baseURL` preserved
  (`v2-debug-config`, `v2-permissions-config`):

  ```json
  {"providers":{"probe":{"package":"aisdk:@ai-sdk/openai-compatible",
    "settings":{"apiKey":"***","baseURL":"https://example.invalid/v1"}}}}
  ```

- v1: `opencode --standalone` is not a v1 concept, and `opencode debug config`
  prints the merged object but reduces provider options to `{}`
  (`v1-debug-config`), so it cannot be used for base URLs. The v1 source is
  `opencode models --verbose`, where each model carries
  `api.url` (`v1-models-verbose`).

Neither path exposes credentials. Credentials remain in v2's SQLite
`credential` table and v1's `auth.json` and must never be opened.

**Evidence:** `v2-proxy`, `v1-proxy`, `v2-debug-paths`, `v1-debug-paths`,
`v1-shared-datadir`, `v2-debug-config`, `v2-permissions-config`,
`v1-debug-config`, `v1-models-verbose`.

## Deferred items

| Item | Why deferred | Where it is checked |
|---|---|---|
| Native Windows ACP (V8) | no Windows host in O0; source-verified only | CI `windows-latest` matrix in O5 |
| HTTP `429`/rate-limit mapping | no quota failure reproduced; only `provider.free-limit` was observed | O4, from a real provider failure |
| v1 `read: deny` end-to-end enforcement | the key is accepted and preserved, but no read-tool denial was exercised | O3 permission conformance |
| v1 `session/load` across process loss | O0 only verified new → load in one process, for both flavours | O2 resume conformance |
