# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Detect the `opencode` binary, its version and the v1/v2 flavour, and report install, version and sign-in status with sign-in and install-help actions.
- Model catalogue from OpenCode's configured providers, with effort variants mapped to the Arvel effort vocabulary and cached for 6 hours.
- Run each Arvel session in its own `opencode acp` process, bridged through ACP, with model and effort switching from the next turn.
- Stop idle sessions after 15 minutes and resume them transparently, and cancel turns by interrupting then killing a stuck process.
- Apply the Arvel permission policy to each session: a baseline written into OpenCode's inline config plus a second evaluator that denies or allows without asking, and a session-local "always allow" that is never forwarded to OpenCode's own persistent rules.
- Deliver per-session instructions (role instructions plus reply language) and pass MCP servers through to OpenCode, skipping SSE servers with a warning.
- Deliver the session's Arvel skills into `.opencode/skills` before OpenCode starts, and add read-only `_arvel/discover` for the MCP servers and skills configured in OpenCode, with secret values redacted.
- Aggregate best-effort spend from each turn into `_arvel/usage` with a per-day ledger, and report per-turn tokens and estimated cost in every `session/prompt` result.
- Map upstream OpenCode failures to typed errors: usage limits to `rate_limited`, unknown models to `model_unavailable`, auth failures to `auth_required`, and everything else to `provider_error`.
