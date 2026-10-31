// SPDX-License-Identifier: Apache-2.0
// Minimal ACP agent used as an offline stand-in for "opencode acp".
import readline from "node:readline";

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
const sessions = new Set();

function send(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}

rl.on("line", (line) => {
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return; // not JSON-RPC; ignore
  }

  if (msg.method === "initialize") {
    send({
      jsonrpc: "2.0",
      id: msg.id,
      result: {
        protocolVersion: 1,
        agentCapabilities: {
          loadSession: true,
          promptCapabilities: { image: false, audio: false, embeddedContext: false },
          mcpCapabilities: { http: true, sse: false },
        },
        agentInfo: { name: "fake-opencode", version: "0.0.0" },
        authMethods: [],
      },
    });
    return;
  }

  if (msg.method === "session/new") {
    const id = "s_" + (sessions.size + 1);
    sessions.add(id);
    send({ jsonrpc: "2.0", id: msg.id, result: { sessionId: id, configOptions: [] } });
    return;
  }

  if (msg.method === "session/prompt") {
    send({
      jsonrpc: "2.0",
      method: "session/update",
      params: {
        sessionId: msg.params && msg.params.sessionId,
        update: {
          sessionUpdate: "agent_message_chunk",
          content: { type: "text", text: "fake-vendor pong" },
        },
      },
    });
    send({ jsonrpc: "2.0", id: msg.id, result: { stopReason: "end_turn" } });
    return;
  }

  // Notifications (no id) are not answered.
  if (msg.id === undefined) {
    return;
  }
  send({ jsonrpc: "2.0", id: msg.id, error: { code: -32601, message: "method not found" } });
});
