# Status — memory-store-mcp

## Current Version

**v1.0.0** — 13 MCP tools, 5 MCP resources, optional Telegram bot, CLI client

## Phases

### Phase 1: Core MCP Server — ✅ Complete

- [x] MCP server framework with `mcp-go`
- [x] Key-value storage backed by libSQL via keyvalembd
- [x] Semantic search via Ollama embeddings (cosine similarity in Go)
- [x] 13 MCP tools (save, get, delete, search, list, context, extract, goal CRUD, timeline, suggest)
- [x] 5 MCP resources (goals, awareness, context, insights, timeline)
- [x] Graceful degradation when Ollama unavailable
- [x] Goal tracking with auto-progress from Markdown subtasks
- [x] Timeline event logging
- [x] Fact extraction via LLM
- [x] Proactive suggestions via LLM
- [x] System instructions injected into MCP server

### Phase 2: Telegram Bot — ✅ Complete

- [x] Telegram bot with notebook mode (auto-classify messages)
- [x] Assistant mode (commands: /search, /goals, /goal, /timeline, /suggest, /context, /digest, /language)
- [x] Rule-based message classifier (no LLM call for classification)
- [x] Access control via TELEGRAM_ALLOWED_USERS
- [x] Multi-language support (ru/en)
- [x] Digest summaries (day/week/month)
- [x] LLM-powered question answering in Telegram
- [x] Bot commands registered in BotFather for both languages
- [x] Telegram notes raw key returned in response

### Phase 3: CLI Client — ✅ Complete

- [x] memory-cli binary with 10 subcommands
- [x] MCP client connection to memory-store-mcp via stdio
- [x] Auto-discovery of server binary
- [x] Formatted output (json/table/summary) via tabwriter
- [x] LLM streaming with thinking indicator
- [x] All server flags forwarded from CLI

### Phase 4: Polish & Documentation

- [x] README updated with all features
- [x] Unit tests for goals and LLM
- [ ] Integration tests (end-to-end)
- [ ] Performance benchmarks
- [ ] Docker image for one-command deployment

## Recent Commits (2026-05)

| Date | Commit | Description |
|------|--------|-------------|
| 2026-05-06 | `11d8542` | Return raw key for Telegram notes |
| 2026-05-06 | `e18bba6` | Add unit tests for helpers |
| 2026-05-06 | `075fa9a` | Clarify Telegram access env |
| 2026-05-05 | `7099bf6` | Add memory CLI sources (10 subcommands) |
| 2026-05-05 | `5a471d9` | Update README for current options |
| 2026-05-05 | `d5adbc7` | Fix telegram question title formatting |
| 2026-05-05 | `c893e88` | refactor: remove all os.Getenv except TELEGRAM_ALLOWED_USERS |
| 2026-05-04 | `91a5052` | refactor: replace --ollama-url with --llm-url, add setLLMURL with priority chain |
| 2026-05-04 | `0206524` | Project docs updated |
| 2026-05-04 | `94e013b` | Remove PLAN-002-telegram.md (plan completed) |
| 2026-05-04 | `65118b1` | Add Telegram bot integration and multi-language suggest support |
| 2026-05-03 | `693179d` | Fix JSON sanitisation in Suggest: order patterns correctly |
| 2026-05-03 | `470dd64` | Implement goal TODO features |
| 2026-05-03 | `efd62e0` | Project status updated |
| 2026-05-03 | `6bc1f9c` | docs: update STATUS.md and DESIGN.md to reflect CLI client and Phase 1-2 completeness |

## Recently Removed

- **PLAN-002-telegram.md** — removed because the Telegram bot is now fully implemented
- **`os.Getenv` calls** — removed all environment variable reads except `TELEGRAM_ALLOWED_USERS`; all other config uses CLI flags (`--db`, `--model`, `--chat-model`, `--llm-url`, `--telegram`)
- **`--ollama-url` flag** — replaced with `--llm-url` (more descriptive; also supports non-Ollama LLM providers that mimic the Ollama API)

## Files

### Top-level

| File | Purpose |
|------|---------|
| `main.go` | MCP server entry point, resource registration, Telegram bot launch |
| `llm.go` | Ollama chat client for extraction and suggestions |
| `llm_test.go` | Unit tests for chat model selection and sanitisation |
| `storage.go` | Storage struct, Telegram wrapper methods, awareness resource helpers |
| `extraction.go` | LLM-based fact extraction from conversation |
| `tools.go` | Tool definitions and registration (13 tools) |
| `tools_context.go` | Context aggregation for memory_get_context tool |
| `tools_extract.go` | Extraction tool handler |
| `tools_goals.go` | Goal CRUD tools, goal mirror to kv_data, Telegram goal wrappers |
| `tools_goals_test.go` | Unit tests for goal functionality |
| `tools_suggest.go` | Suggest tool handler |

### `telegram/` directory

| File | Purpose |
|------|---------|
| `bot.go` | Telegram bot core: NewBot, Run loop, handleUpdate, command routing, language, help |
| `assistant.go` | Command handlers: search, goals, goal, timeline, suggest, context |
| `notebook.go` | Text message handler, classification dispatching, note/goal/question/command handling |
| `classifier.go` | Rule-based message classifier: detect note/goal/question/command |
| `digest.go` | Digest command handler |
| `i18n.go` | Internationalisation: all translated strings, BotCommand descriptions |
| `types.go` | Shared types: SearchResult, MemoryValue, Goal, TimelineEntry, Suggestion, ContextResult |

### `cmd/memory-cli/` directory

| File | Purpose |
|------|---------|
| `main.go` | CLI entry point, flag parsing, stdout/stderr piping |
| `client.go` | MCP client: spawn server, connect, proxyStdout, CallTool |
| `save.go` | `memory save` subcommand |
| `get.go` | `memory get` subcommand |
| `delete.go` | `memory delete` subcommand |
| `search.go` | `memory search` subcommand |
| `list.go` | `memory list` subcommand |
| `context.go` | `memory context` subcommand |
| `extract.go` | `memory extract` subcommand |
| `goals.go` | `memory goals` subcommand (list, show, create) |
| `timeline.go` | `memory timeline` subcommand |
| `suggest.go` | `memory suggest` subcommand |
| `format.go` | Output formatting: json/table/summary with tabwriter, truncate, proxyStderrWithThinking |

## Known Issues

1. **No integration tests** — unit tests cover goals and LLM, but no end-to-end test with a real database
2. **No performance benchmarks** — semantic search performance on large datasets (10k+ entries) is unknown
3. **No Dockerfile** — currently requires manual Go build; no containerised deployment
4. **Ollama dependency** — semantic search and LLM features require a running Ollama instance; graceful degradation is in place but reduced functionality
5. **BotFather commands** — bot commands are registered via API on every start; this is fine but could be made optional with a flag