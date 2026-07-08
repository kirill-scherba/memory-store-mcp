# Context — memory-store-mcp

## Project Overview

memory-store-mcp is an MCP (Model Context Protocol) server that provides **persistent long-term memory** for AI assistants (like Baron). It stores facts, observations, and knowledge with auto-generated embeddings for semantic search.

## Key Problem Solved

AI assistants typically have no memory across sessions. Each conversation starts from scratch. This server gives AI assistants a persistent memory store that:

1. **Survives sessions** — stored in libSQL database on disk
2. **Finds by meaning** — semantic search via Ollama embeddings (not just keyword matching)
3. **Stays organized** — hierarchical S3-style keys for navigation

## Key Features

- **Persistent key-value storage** — backed by libSQL (SQLite-compatible) with WAL mode
- **Semantic search** — vector similarity via Ollama embeddings (embeddinggemma:latest)
- **Hierarchical keys** — S3-style: `memory/project/...`, `memory/user/...`, `memory/technical/...`
- **Structured values** — JSON with content, summary, tags, timestamp, source
- **MCP protocol** — JSON-RPC 2.0 over stdin/stdout, 13 tools, 5 resources
- **Goal tracking** — full CRUD with status/progress/priority/labels/deadlines, auto-progress from Markdown subtasks
- **Timeline** — event log with date range queries
- **Fact extraction** — auto-extract structured facts from conversation via LLM
- **Proactive suggestions** — LLM-powered next-action recommendations
- **Telegram bot** — optional Telegram integration with `/note`, `/search`, `/goal`, `/suggest`, `/context`, `/ask` commands; access control via `TELEGRAM_ALLOWED_USERS`; multi-language support (en/ru)
- **CLI client** — 10 subcommands with formatted output (json/table/summary)
- **Multi-language suggest** — en/ru support for suggestion prompts, configurable via Telegram user language preference
- **Refactored environment** — single env var `TELEGRAM_ALLOWED_USERS`; all other config via CLI flags (`--db`, `--model`, `--chat-model`, `--llm-url`, `--llm-api-key`, `--save-timeout`)
- **OpenAI-compatible API support** — optional `--llm-api-key` flag for authentication with OpenAI, OpenRouter, Groq, etc.
- **HTTP/SSE transport** — optional `--http` flag starts the server in HTTP mode with SSE (Server-Sent Events) and JSON-RPC message endpoint, enabling remote clients and multi-client access

## Target Audience

- AI assistants (Cline, Claude, etc.) that need persistent memory
- Developers who want their AI to remember context across conversations
- Baron — the AI assistant with transmigrating soul
- Telegram users who want AI memory via chat interface

## Dependencies

- **Go 1.26+** — build and runtime
- **github.com/kirill-scherba/keyvalembd** — S3-like key-value store with embeddings (libSQL + Ollama)
- **github.com/mark3labs/mcp-go** — MCP library for Go
- **Ollama** — embedding model (optional, for semantic search)
- **github.com/go-telegram/bot** — Telegram bot framework (optional)

## Related Projects

- [keyvalembd](https://github.com/kirill-scherba/keyvalembd) — underlying storage library
- [s3lite](https://github.com/kirill-scherba/s3lite) — S3-like key-value store interface
- [web-search-mcp](https://github.com/kirill-scherba/web-search-mcp) — reference MCP server implementation
- [db-tool-mcp](https://github.com/kirill-scherba/db-tool-mcp) — another reference MCP server