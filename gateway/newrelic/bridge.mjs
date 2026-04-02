// bridge.mjs — Entry point for the New Relic MCP bridge container.
//
// Reads configuration from environment variables, then spawns mcp-remote
// as a stdio bridge to New Relic's hosted MCP server. agentgateway connects
// to this process via stdio.
//
// Environment variables:
//   NEW_RELIC_API_KEY   — User API key (NRAK-...). Required unless using OAuth.
//   NEW_RELIC_MCP_URL   — Override MCP endpoint (default: US region).
//   NEW_RELIC_REGION    — Set to "eu" for the EU endpoint.
//   NEW_RELIC_TAGS      — Comma-separated include-tags filter (e.g. "discovery,alerting").

import { spawn } from "node:child_process";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));

const US_URL = "https://mcp.newrelic.com/mcp/";
const EU_URL = "https://mcp.eu.newrelic.com/mcp/";

const region = process.env.NEW_RELIC_REGION?.toLowerCase();
const url = process.env.NEW_RELIC_MCP_URL || (region === "eu" ? EU_URL : US_URL);
const apiKey = process.env.NEW_RELIC_API_KEY;
const tags = process.env.NEW_RELIC_TAGS;

const mcpRemote = resolve(__dirname, "node_modules/.bin/mcp-remote");
const args = [mcpRemote, url, "--transport", "http"];

// NOTE: mcp-remote requires headers as CLI args, making the API key visible
// in the process list (ps aux / /proc/<pid>/cmdline). This is a known limitation.
// Mitigations: use PID namespace isolation in K8s (default), restrict host access.
// TODO: migrate to env-based auth when mcp-remote supports it.
if (apiKey) {
  args.push("--header", `Api-Key: ${apiKey}`);
}
if (tags) {
  args.push("--header", `include-tags: ${tags}`);
}

const child = spawn(process.execPath, args, { stdio: "inherit" });

child.on("exit", (code) => process.exit(code ?? 1));
process.on("SIGTERM", () => child.kill("SIGTERM"));
process.on("SIGINT", () => child.kill("SIGINT"));
