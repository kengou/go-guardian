---
name: newrelic
description: "New Relic observability specialist with live MCP access. Use when the user asks to build, create, or design a dashboard; write NRQL queries; analyze golden signals, Kubernetes metrics, SLIs/SLOs, or error budgets; troubleshoot slow services or incidents; list deployments, alerts, or errors; or discover entities in New Relic — even without saying 'New Relic'. Delegates to `newrelic-dashboards`, which tests every query against real data before presenting it."
argument-hint: "[dashboard|query|alert|troubleshoot|incidents] [service-name]"
---

# /newrelic — New Relic Specialist

Analyse the request and delegate to the `newrelic-dashboards` agent. The agent has access to 27 MCP tools from the New Relic hosted MCP server via the gateway bridge.

## Gotchas

- **`newrelic-dashboards` is the only caller of `mcp__newrelic__*`
  tools.** Do not attempt NRQL execution from this skill directly.
- **Every NRQL query must be tested against real data via MCP before
  dashboarding.** Offline-generated NRQL is forbidden — it breaks on
  edge-case attribute names and stale schemas.
- **Empty entity discovery usually means the wrong account.**
  Re-discover the account ID before assuming the service doesn't exist.

## Routing

| Request Pattern | Action |
|---|---|
| "dashboard" / "build" / "create" | Build complete dashboard (discover entities → test queries → compose layout) |
| "query" / "NRQL" / "show me" | Write NRQL, execute via MCP, show results |
| "alert" / "monitor" / "notify" | Create NRQL alert condition |
| "golden signals" | Analyze golden metrics via MCP + build dashboard |
| "k8s" / "kubernetes" / "cluster" | Build Kubernetes cluster dashboard |
| "service health" / "service dashboard" | Build per-service health dashboard |
| "SLO" / "SLI" / "error budget" | Build SLI/SLO tracking dashboard |
| "terraform" / "HCL" | Output as Terraform newrelic_one_dashboard resource |
| "troubleshoot" / "why is ... slow" / "what's wrong" | Analyze golden metrics → transactions → logs → deployments |
| "incidents" / "issues" / "alerts firing" | List recent issues, search incidents, generate reports |
| "errors" / "error groups" | Fetch error groups from Errors Inbox |
| "deployments" / "changes" | List change events, analyze deployment impact |
| "entities" / "services" / "find" | Discover entities by name, tag, or type |
| "logs" / "log analysis" | Analyze entity logs for error patterns |

## Instructions

1. Invoke the `newrelic-dashboards` agent with the full user request
2. The agent will:
   - Use MCP tools to query live data (not just generate offline NRQL)
   - Discover entities and accounts automatically
   - Test every NRQL query against real data before presenting it
   - Compose dashboard layouts with working, verified queries
   - Output in the requested format (JSON, Terraform, or queries only)
3. Do NOT re-explain what the agent will do — just dispatch
