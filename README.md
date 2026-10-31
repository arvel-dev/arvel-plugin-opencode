# OpenCode for Arvel

[![CI](https://github.com/arvel-dev/arvel-plugin-opencode/actions/workflows/ci.yml/badge.svg)](https://github.com/arvel-dev/arvel-plugin-opencode/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/arvel-dev/arvel-plugin-opencode)](https://github.com/arvel-dev/arvel-plugin-opencode/releases)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

This provider plugin lets [Arvel](https://github.com/arvel-dev/arvel) roles run on **any model [OpenCode](https://opencode.ai) can reach**. That includes Kimi, DeepSeek, GLM, Qwen and other models through OpenRouter or the vendors' own APIs, plus local models through Ollama. It uses your existing OpenCode setup and keys.

It ships with Arvel. You can turn it off or remove it in **Settings → Plugins**, and restore it later.

---

## What you need

| | |
|---|---|
| OpenCode | v1 (`opencode-ai`) or v2 (`@opencode/cli`). See [Compatibility](#compatibility). |
| At least one provider in OpenCode | e.g. `opencode auth login` → choose a provider → paste its key |
| Arvel | 1.0 or newer (plugin API 1) |
| OS | macOS, Linux and Windows on x64 and arm64. It also runs on a Linux server that Arvel connects to. |

```bash
curl -fsSL https://opencode.ai/install | bash      # macOS / Linux
npm i -g opencode-ai                               # any OS (v1)
```

## How it connects

| Connection | Billing | What Arvel shows |
|---|---|---|
| **OpenCode** | Whatever you configured in OpenCode: mostly pay-per-token API keys, sometimes a subscription | Spend per task when OpenCode reports cost; otherwise tokens only |

Arvel talks to OpenCode through the [Agent Client Protocol](https://agentclientprotocol.com) (`opencode acp`). Each Arvel session gets its own OpenCode process with that role's instructions and rules.

## Signing in

OpenCode keeps its own provider keys. To add one, open **Arvel → Settings → OpenCode → Add provider**. Arvel opens its terminal and runs `opencode auth login`. Pick the provider and paste the key. On a remote server this happens in the server's terminal. Arvel never sees the keys.

## Models and reasoning effort

Every `provider/model` that OpenCode offers appears in Arvel, for example `Kimi K2.5 · Moonshot`. If a model has reasoning **variants**, Arvel shows them as effort levels: Low, Medium, High, Max. Auto uses the model's default.

## Permissions and safety

The plugin writes the Arvel role's rules into OpenCode's configuration **for that session only**:

| Role rule | OpenCode |
|---|---|
| Edit files: Yes / Ask / No | `edit`: allow / ask / deny |
| Commands: allow and deny lists, default | `bash` patterns (v1) / ordered `permissions` (v2) |
| Internet: Yes / Ask / No | `webfetch`: allow / ask / deny |
| Subagents off | the task/subagent tool is denied |

The plugin also checks each permission request itself. "No" answers right away, "Yes" passes, and "Ask" comes to you in Arvel or the Telegram bot. "Allow for session" is remembered only by the plugin and is never written into OpenCode's configuration.

The project's own `opencode.json` (agents, MCP, permissions) is **not** loaded unless you enable *Load the project's own opencode.json*.

Limitations:
- OpenCode v1 has no read permission, so *Read files: No* cannot be enforced there.
- Shell commands like `curl` count as commands, not as network use.

## Skills and MCP servers

You manage skills and MCP servers in Arvel (**Agent tools → MCP servers / Skills**) and assign them to roles. Before each session starts, the plugin places that role's skills in `.opencode/skills/` inside the task's working folder, hidden from git, and passes only that role's MCP servers. Both stdio and HTTP servers are supported.

## Usage and limits

OpenCode does not know your provider's quota windows. The plugin therefore reports spend when OpenCode includes cost, and tokens otherwise. When a provider rejects requests because a limit was hit, the run stops cleanly and stays resumable. Arvel then applies the role's rule: wait, switch to a fallback executor, or ask you.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| "opencode not found" | Not installed or not on PATH | Install it, or set *Path to opencode* |
| "No providers configured" | No keys in OpenCode | Add provider (runs `opencode auth login`) |
| A model is missing | The provider is not configured, or OpenCode hides the model | Check `opencode models` in a terminal |
| Runs stop with "limit reached" | The provider rejected requests | Top up the provider account or add a fallback executor |

## Privacy

The plugin talks only to the `opencode` program on the same machine. OpenCode talks to the providers you configured. The plugin itself makes no network connections and has no telemetry.

## Compatibility

| Plugin | OpenCode tested | Plugin API |
|---|---|---|
| 0.1.x | v1 and v2 (see release notes) | 1 |

## Disclaimer

Independent open-source project, not affiliated with the OpenCode project or any model provider. Product names belong to their owners.

## Development

```bash
go test -race ./...
go build -o dist/arvel-plugin-opencode ./cmd/arvel-plugin-opencode
arvel-plugin-check --level release --scenarios ./conformance -- ./dist/arvel-plugin-opencode
```

Fixtures for both OpenCode versions are described in `testdata/fixtures/README.md`. E2E tests: `ARVEL_E2E=1 go test -tags e2e ./e2e/...`. Use a free or local model for these.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
