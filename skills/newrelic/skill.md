---
name: newrelic
description: "New Relic observability specialist with live MCP access. Build dashboards, run NRQL queries, analyze metrics, troubleshoot incidents, manage alerts."
argument-hint: "[dashboard|query|alert|troubleshoot|incidents] [service-name]"
---

# /newrelic — New Relic Specialist

Analyse the request and delegate to the `newrelic-dashboards` agent. The agent has access to 27 MCP tools from the New Relic hosted MCP server via the gateway bridge.

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
