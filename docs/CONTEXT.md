# Context — memory-store-mcp

## Project Overview

memory-store-mcp is an MCP (Model Context Protocol) server that provides **persistent long-term memory** for AI assistants (like Baron). It stores facts, observations, and knowledge with auto-generated embeddings for semantic search.

## Key Problem Solved

AI assistants typically have no memory across sessions. Each conversation starts from scratch. This server gives AI assistants a persistent memory store that:

1. **Survives sessions** — stored in a SQLite database on disk
2. **Finds by meaning** — semantic search via Ollama embeddings (not just keyword matching)
3. **Stays organized** — hierarchical S3-style keys for navigation

## Key Features

- **Persistent key-value storage** — backed by pure-Go SQLite (modernc.org/sqlite) with WAL mode and foreign keys; builds with `CGO_ENABLED=0`
- **Semantic search** — vector similarity via Ollama embeddings (embeddinggemma:latest); answered from an in-process, memory-mapped exact index (`<db>-idx`, ~3 KB/vector), falling back to a database scan when no index is configured
- **Hierarchical keys** — S3-style: `memory/project/...`, `memory/user/...`, `memory/technical/...`
- **Structured values** — JSON with content, summary, tags, timestamp, source
- **MCP protocol** — JSON-RPC 2.0 over stdin/stdout or HTTP/SSE, 19 tools, 5 resources
- **Goal tracking** — full CRUD with status/progress/priority/labels/deadlines, auto-progress from Markdown subtasks
- **Timeline** — event log with date range queries
- **Fact extraction** — auto-extract structured facts from conversation via LLM; background AsyncExtractor prevents timeouts when auto_save is true
- **Proactive suggestions** — LLM-powered next-action recommendations
- **Telegram bot** — optional Telegram integration with `/note`, `/search`, `/goal`, `/suggest`, `/context`, `/ask` commands; access control via `TELEGRAM_ALLOWED_USERS`; multi-language support (en/ru)
- **CLI client** — 16 subcommands with formatted output (json/table/summary), including `rebuild-index` (in-process vector index) and `compact` (drop legacy column + VACUUM)
- **Multi-language suggest** — en/ru support for suggestion prompts, configurable via Telegram user language preference
- **Default model**: `phi4-mini` (switched from `qwen2.5-coder:7b` on 2026-07-13 after comparative testing — phi4-mini is faster on short texts, equal on long texts, already loaded by RAG, uses less RAM). `qwen2.5-coder:7b` available via `--extract-model` / `--chat-model` flags
- **Refactored environment** — single env var `TELEGRAM_ALLOWED_USERS`; all other config via CLI flags (`--db`, `--model`, `--chat-model`, `--llm-url`, `--llm-api-key`, `--save-timeout`)
- **OpenAI-compatible API support** — optional `--llm-api-key` flag for authentication with OpenAI, OpenRouter, Groq, etc.
- **HTTP/SSE transport** — optional `--http` flag starts the server in HTTP mode with SSE (Server-Sent Events) and JSON-RPC message endpoint, enabling remote clients and multi-client access
- **AsyncWriter** — non-blocking writes with background worker queue (1 worker, depth 64); `memory_save` returns immediately while embedding generation runs async; critical for voice/Alexa low-latency paths
- **AsyncExtractor** — background LLM fact extraction (1 worker, depth 64); `memory_extract(auto_save=true)` queues the LLM call and returns a job ID immediately, eliminating the MCP gateway + Ollama timeout data-loss bug
- **Keyword search (memory_find)** — exact SQL LIKE search on both keys and values with Unicode case-insensitivity fallback for Russian; complements semantic embedding search; available in MCP, CLI, and Telegram
- **Contextual deep-search (memory_dig)** — finds entries matching a query, builds scenes with time-window context (entries before/after each match), intersects with additional keywords for relevance ranking; designed for "образная память" (associative human memory); available in MCP, CLI, and Telegram
- **Session management** — save, get, list, and compact AI session state; available in MCP and CLI
- **Knowledge graph** — entities and relations in indexed tables (`graph_entities`, `graph_edges`), not as key-value entries. `graph_get_edges` is an indexed lookup (two index seeks) and `graph_query` traverses with a recursive CTE, so `depth` is real. Connections are injected into `memory_get_context` and `memory_search` automatically, so the agent never has to remember to call a graph tool. The graph grows from gallery image metadata (deterministic) and from `memory_extract` triples (same LLM call as the facts).
- **Bounded timeline** — only meaningful events are logged (writes, extraction, sessions, graph edges, goals); read tools are excluded, and `PruneTimeline` drops non-allowlisted types plus events older than `--timeline-retention` (default 30 days)

## Target Audience

- AI assistants (Cline, Claude, etc.) that need persistent memory
- Developers who want their AI to remember context across conversations
- Baron — the AI assistant with transmigrating soul
- Telegram users who want AI memory via chat interface

## Dependencies

- **Go 1.26+** — build and runtime
- **github.com/kirill-scherba/keyvalembd** — S3-like key-value store with embeddings (pure-Go SQLite + Ollama)
- **github.com/mark3labs/mcp-go** — MCP library for Go
- **Ollama** — embedding model (optional, for semantic search)
- **github.com/go-telegram/bot** — Telegram bot framework (optional)

## Related Projects

- [keyvalembd](https://github.com/kirill-scherba/keyvalembd) — underlying storage library
- [s3lite](https://github.com/kirill-scherba/s3lite) — S3-like key-value store interface
- [web-search-mcp](https://github.com/kirill-scherba/web-search-mcp) — reference MCP server implementation
- [db-tool-mcp](https://github.com/kirill-scherba/db-tool-mcp) — another reference MCP server

## 2026-09-14 — LLM reasoning latency

`memory_suggest` was slow on the cloud model too, which was counter-intuitive
because `deepseek-v4-flash:cloud` is far faster per token than `phi4-mini`.
Measured cause: the cloud model is a **reasoning model**. It emits a `thinking`
block before the answer, and those tokens are billed against the response budget.

| | eval tokens | wall time |
|---|---|---|
| thinking on | 898–2018 | 8.6–20.5 s |
| thinking off (`think:false`) | 273–376 | 4.0–5.1 s |

Plumbing was ruled out: a stdio MCP round trip costs 29 ms.

`generateAnswerWithClient` now sends `"think": false`, which covers
`memory_suggest` and `memory_extract` (both want structured JSON, not
deliberation). The field is sent only to the Ollama endpoint — OpenAI-compatible
APIs do not know it. Streaming Telegram answers keep reasoning. `phi4-mini`
ignores the field, so the local path is unchanged.

End to end `memory-cli suggest` on the cloud model: 15.7 s → 5.4 s, and answer
quality improved (suggestions cite the real advisory, invoice amount and goal).

Commit `4416e7f`.
