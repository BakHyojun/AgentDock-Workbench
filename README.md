<div align="center">

English | [简体中文](./README.zh-CN.md)

<img src="./docs/assets/agentdock-logo.png" alt="AgentDock Workbench logo" width="128" />

# AgentDock Workbench

**A task, execution, and permission workbench for AI agents operating real devices.**

AgentDock Workbench connects ChatGPT, Claude, Codex, and other MCP clients to local computers, remote servers, and mobile nodes. It combines the AgentDock runtime with a conversation-centered control plane for tasks, tool calls, approvals, permissions, mid-task instructions, plugins, and multi-device operations.

[Download Windows x64 Setup 1.1.8200](https://github.com/eerraa/AgentDock-Workbench/releases/download/v1.1.8200/AgentDockSetup-amd64.exe) · [Fork releases](https://github.com/eerraa/AgentDock-Workbench/releases) · [Stable v1.1.8100](https://github.com/eerraa/AgentDock-Workbench/releases/tag/v1.1.8100) · [Preview v1.1.8200](https://github.com/eerraa/AgentDock-Workbench/releases/tag/v1.1.8200)

[![GitHub Release](https://img.shields.io/github/v/release/eerraa/AgentDock-Workbench?include_prereleases&display_name=tag&logo=github)](https://github.com/eerraa/AgentDock-Workbench/releases)
[![License](https://img.shields.io/github/license/eerraa/AgentDock-Workbench)](./LICENSE)

</div>

This extension fork distributes Windows x64 Setup only. Version 1.1.8200 is an unsigned pre-release; actual isolated installation and upgrade acceptance have not been run. [Release notes](./docs/releases/v1.1.8200.md) · [한국어 릴리즈 등록 안내](./docs/eerraa/windows-release.ko.md). The Workbench source baseline remains upstream v1.1.8.

<p align="center">
  <img
    src="./docs/assets/agentdock-multi-device.png"
    alt="AgentDock Workbench managing multiple devices from one AI conversation"
    width="100%"
  />
</p>

## Project purpose

AgentDock Workbench is an independently maintained extension of [AgentDock](https://github.com/uvwt/agentdock). The runtime exposes files, commands, Git, Skills, dynamic MCP servers, browser automation, and other host capabilities through MCP. The Workbench adds the management layer needed to operate those capabilities as a durable product rather than a collection of isolated tool calls.

The project does not provide a chat interface or perform model inference. Your AI client supplies the model and conversation; AgentDock Workbench runs authorized operations in the connected environment and records what happened.

## Workbench capabilities

| Area | What the Workbench provides |
| --- | --- |
| Conversation-first navigation | Organizes active and historical conversations by workspace, with search, pinning, archiving, recovery, and explicit selection state. |
| Recoverable tasks | Persists goals, steps, threads, checkpoints, blockers, and final review so long-running work can continue after interruption. |
| Execution activity center | Tracks root calls and child calls, parameters, output, duration, files changed, errors, approvals, and terminal state without treating background probes as user work. |
| Mid-task insertion and stop | Sends new user instructions into an active conversation, records delivery and acknowledgement state, and supports stopping a conversation or a specific call. |
| Layered permissions | Separates the permission profile, approval policy, and approval reviewer; global and workspace settings remain visible and auditable. |
| Skills, plugins, and MCP | Discovers, installs, enables, disables, and inspects Skills, self-contained plugins, and dynamic MCP servers through one management surface. |
| Multi-device operation | Connects local machines, servers, containers, and mobile nodes while preserving the target workspace and execution identity. |
| Installation and recovery | Uses explicit installation, update, rollback, retention, and health states instead of reporting an operation complete before it is verified. |

## Architecture

```text
 ChatGPT / Claude / Codex / other MCP client
                       │
              MCP + Bearer/OAuth
                       │
              AgentDock Core runtime
       ┌───────────────┼────────────────┐
       │               │                │
 Workbench UI      Management CLI   Tool runtime
       │               │                │
 Conversations     Tasks / calls     Files / shell / Git
 Permissions       Approvals         Skills / plugins / MCP
 Insert / stop     Logs / export     Browser / deployment
                       │
       Local computer · Server · Container · Mobile node
```

A conversation is resolved from trusted host metadata. Tasks and calls inherit that binding, and approvals attach to the concrete call being reviewed. This prevents the management interface from guessing identity from the currently selected window or the most recently active task.

## Platform status

| Platform | Experience | Release status |
| --- | --- | --- |
| Windows | Native WPF Workbench, graphical installer, task and activity center, permission management, insertion, updates, and recovery. | Main stable desktop experience. |
| macOS | Native Swift/AppKit Workbench using the shared Core management contract. | Expanded in the v1.1.8 preview. |
| Android | Kotlin/Jetpack Compose Workbench connected to an external Termux/PRoot Core, with deployment and keep-alive controls. | Introduced in the v1.1.8 preview. |
| Linux | Headless Core and management CLI for local, server, and scripted operation. | Available through release packages and source builds. |
| Containers / VPS | Headless runtime with MCP, CLI, authentication, and remote access options. | Deployment depends on the selected release asset and environment. |

The table describes upstream platform capabilities. This fork ships Windows x64 only: stable v1.1.8100 and pre-release v1.1.8200. Check the fork release notes and validation scope before installing.

## Typical workflows

- Let an AI agent modify a real project, run its tests, inspect failures, and commit the verified result on the target machine.
- Continue a multi-hour task after a disconnected browser session by restoring its persisted steps and checkpoints.
- Inspect exactly which tool calls belong to a conversation, including child calls, output, errors, and file changes.
- Insert a requirement while work is running without starting a second task or losing the current execution context.
- Require user approval for selected operations while keeping hard filesystem, network, and sandbox limits in force.
- Manage several AgentDock nodes from one conversation and direct each operation to the correct workspace and device.

## Quick start

1. Open [fork Releases](https://github.com/eerraa/AgentDock-Workbench/releases) and choose the stable release or current preview.
2. Download **AgentDockSetup-amd64.exe** from Assets. The Source code archives are not installers.
3. Run Setup on your Windows x64 PC to install or update the Workbench.
4. Obtain the MCP endpoint and Bearer Token or complete the OAuth connection flow.
5. Add the endpoint to the MCP, Tools, or Connectors settings of your AI client.

A typical local MCP configuration is:

```json
{
  "mcpServers": {
    "agentdock": {
      "url": "http://127.0.0.1:8765/mcp",
      "headers": {
        "Authorization": "Bearer <AGENTDOCK_AUTH_TOKEN>"
      }
    }
  }
}
```

Keep authentication enabled for every non-local connection. Do not publish tokens, OAuth credentials, private origins, approval payloads, or execution logs containing secrets.

## Project documentation

| Topic | Document |
| --- | --- |
| Task and execution center | [Execution center](./docs/execution-center.md) |
| Core management interface | [Execution center API](./docs/execution-center-api.md) |
| Permission model | [Permission profiles](./docs/permission-profiles.md) |
| Custom permission settings | [Custom permission settings](./docs/permissions-custom-settings.md) |
| Mid-task instruction delivery | [Insertion delivery](./docs/insertion-delivery-1.1.7.md) |
| AGENTS.md context injection | [Agent context](./docs/agents-context.md) |
| Skills and self-contained plugins | [Agent plugins](./docs/agent-plugins.md) |
| Tailscale Funnel access | [Tailscale Funnel](./docs/tailscale-funnel.md) |
| Differences from upstream and migration | [Version differences and migration](./docs/official-version-differences-and-migration.md) |
| Stable fork release details | [v1.1.8100 release notes](./docs/releases/v1.1.8100.md) |
| Preview fork release details | [v1.1.8200 release notes](./docs/releases/v1.1.8200.md) |
| Registering a Windows release | [한국어 릴리즈 등록 안내](./docs/eerraa/windows-release.ko.md) |

## Repository layout

| Path | Responsibility |
| --- | --- |
| `cmd/` | AgentDock command-line entrypoints. |
| `internal/` | Core runtime, state, permissions, tasks, calls, installation, and platform services. |
| `api/` | Public and management API definitions. |
| `desktop/` | Native desktop clients and desktop-specific integration. |
| `mobile/` | Android Workbench and mobile integration. |
| `core-skills/` | Skills that ship with the runtime. |
| `packaging/` | Installers, release packaging, and platform delivery files. |
| `docs/` | Architecture, behavior, release, migration, and validation documentation. |

## Development and verification

Read [AGENTS.md](./AGENTS.md) before changing the repository. Run the complete repository check before submitting changes:

```bash
make check
```

GitHub Actions is disabled for this fork by repository policy. Builds use a clean local clone; actual isolated runner acceptance remains a separate delivery state. A successful source build does not by itself prove that an installer or upgrade path has been validated.

Submit reproducible fork bugs and feature requests through [GitHub Issues](https://github.com/eerraa/AgentDock-Workbench/issues). Include the Workbench version, operating system, relevant call or task state, and redacted logs when they affect the failure.

## Relationship to upstream

AgentDock Workbench is based on the Apache-2.0-licensed [upstream AgentDock project](https://github.com/uvwt/agentdock) and retains the required license and attribution. Workbench-specific features, releases, documentation, installation behavior, and support are maintained in this repository. Review the [migration guide](./docs/official-version-differences-and-migration.md) before switching between the upstream distribution and AgentDock Workbench because their state and installation boundaries are not interchangeable.

## License

Apache License 2.0. See [LICENSE](./LICENSE).
