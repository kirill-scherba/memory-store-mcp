# Status — memory-store-mcp

## Current Version

**v1.1.0** — 19 MCP tools, 5 MCP resources, optional Telegram bot, CLI client

## Phases

### Phase 1: Core MCP Server — ✅ Complete

- [x] MCP server framework with `mcp-go`
- [x] Key-value storage backed by libSQL via keyvalembd
- [x] Semantic search via Ollama embeddings (cosine similarity in Go)
- [x] 19 MCP tools (save, get, delete, search, list, context, extract, goal CRUD, timeline, suggest, find, dig, session save/get/list/compact)
- [x] 5 MCP resources (goals, awareness, context, insights, timeline)
- [x] Graceful degradation when Ollama unavailable
- [x] Goal tracking with auto-progress from Markdown subtasks
- [x] Timeline event logging
- [x] Fact extraction via LLM
- [x] Proactive suggestions via LLM
- [x] System instructions injected into MCP server

### Phase 2: Telegram Bot — ✅ Complete

- [x] Telegram bot with notebook mode (auto-classify messages)
- [x] Assistant mode (commands: /search, /memory, /find, /dig, /list, /goals, /goal, /timeline, /suggest, /context, /digest, /language)
- [x] Rule-based message classifier (no LLM call for classification)
- [x] Access control via TELEGRAM_ALLOWED_USERS
- [x] Multi-language support (ru/en)
- [x] Digest summaries (day/week/month)
- [x] LLM-powered question answering in Telegram
- [x] Bot commands registered in BotFather for both languages
- [x] Telegram notes raw key returned in response

### Phase 3: CLI Client — ✅ Complete

- [x] memory-cli binary with 13 subcommands
- [x] MCP client connection to memory-store-mcp via stdio
- [x] Auto-discovery of server binary
- [x] Formatted output (json/table/summary) via tabwriter
- [x] LLM streaming with thinking indicator
- [x] All server flags forwarded from CLI

### Phase 4: Polish & Documentation

- [x] README updated with all features
- [x] Unit tests for goals and LLM
- [x] Comprehensive test coverage: 55.2% main package, 22% CLI
- [x] LLM agent for Telegram (structured JSON commands with fallback)
- [x] Bot logging system with file rotation (stderr + file)
- [x] Telegram assistant unit tests (mock-based, 810 lines)
- [ ] `--debug` flag and log levels
- [ ] Performance benchmarks
- [ ] Docker image for one-command deployment

### Phase 5: Code Review & Testing — PLAN-002

See [PLAN-002.md](PLAN-002.md) for the full plan.

- [ ] Task 1: `--debug` flag and log levels
- [ ] Task 2: Manual bot/LLM testing
- [ ] Task 3: Automated Telegram bot tests
- [ ] Task 4: Semantic e2e memory tests
- [ ] Task 5: CI (Makefile) and docs

## Recent Commits (2026-07)

| Date | Commit | Description |
|------|--------|-------------|
| 2026-07-08 | `c51e4f8` | feat: add memory_dig — contextual deep-search with scenes and time windows |
| 2026-07-08 | `0554f4f` | fix test: update expected subcommand count to 11 (added 'find') |
| 2026-07-08 | `e1c8c97` | Merge origin/main (PR #32 save timeout) into local changes |
| 2026-07-08 | `e58fef2` | Add memory_find keyword search, async writes, enriched memory_search, and memory-cli find subcommand |
| 2026-07-08 | `5d75fe9` | feat(deploy): add user-mode systemd service for HTTP MCP server |
| 2026-07-08 | `1d86d1f` | feat: add deploy script for orchestrator auto-deploy |
| 2026-07-08 | `ddcced0` | docs: add project image to README |
| 2026-07-08 | `e6104db` | feat: auto-inject upcoming reminders into GetContextForInjection |
| 2026-07-08 | `d7cc3d5` | Merge pull request #31 from kirill-scherba/feature/30 |
| 2026-06-14 | *branch* | test: add comprehensive test coverage for storage, tools, extraction, CLI (#20) |

## Recently Removed

- **PLAN-002-telegram.md** — removed because the Telegram bot is now fully implemented
- **`os.Getenv` calls** — removed all environment variable reads except `TELEGRAM_ALLOWED_USERS`; all other config uses CLI flags (`--db`, `--model`, `--chat-model`, `--llm-url`, `--llm-api-key`, `--telegram`)
- **`--ollama-url` flag** — replaced with `--llm-url` (more descriptive; also supports non-Ollama LLM providers that mimic the Ollama API)

## Files

### Top-level

| File | Purpose |
|------|---------|
| `main.go` | MCP server entry point, resource registration, Telegram bot launch |
| `llm.go` | Ollama chat client for extraction and suggestions |
| `llm_test.go` | Unit tests for chat model selection and sanitisation |
| `storage.go` | Storage struct, Telegram wrapper methods, awareness resource helpers, Find, Dig |
| `extraction.go` | LLM-based fact extraction from conversation |
| `tools.go` | Tool definitions and registration (19 tools) |
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
| `agent.go` | LLM agent: structured JSON commands, rule-based fallback (graceful degradation) |
| `assistant_test.go` | Unit tests for assistant commands and LLM agent (810 lines) |
| `log.go` | Bot logger: stderr + file with rotation (10MB, 3 files), structured log helpers |

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
| `find.go` | `memory find` subcommand — keyword search via SQL LIKE |

### `docs/` directory

| File | Purpose |
|------|---------|
| `STATUS.md` | Current project status (this file) |
| `DESIGN.md` | Architectural design and decisions |
| `PLAN-001.md` | Phase 1-3 implementation plan (completed) |
| `PLAN-002.md` | Phase 5 plan: logging, testing, CI |

## Known Issues

1. **No integration tests** — unit tests cover goals, LLM, and Telegram assistant, but no end-to-end test with a real database
2. **No performance benchmarks** — semantic search performance on large datasets (10k+ entries) is unknown
3. **No Dockerfile** — currently requires manual Go build; no containerised deployment
4. **Ollama dependency** — semantic search and LLM features require a running Ollama instance; graceful degradation is in place but reduced functionality
5. **BotFather commands** — bot commands are registered via API on every start; this is fine but could be made optional with a flag
6. **```json fences** — qwen2.5-coder:7b wraps JSON in ```json in ~40% of responses; parseAgentResponse handles this via 5 fallback strategies
7. **No `--debug` flag** — bot logs always go to both stderr and file; no verbosity levels
8. **mcp-gateway tool caching** — gateway at port 7711 caches tool lists; restart required after adding new tools to memory-store-mcp

## Recent Fixes & Features

- **Issue #33 / PR #34** (2026-07-13): `memory_extract` async — added `AsyncExtractor` (1 worker, queue depth 64), dedicated no-timeout `extractClient`, `--extract-model` flag. `memory_extract(auto_save=true)` returns `{status: "accepted", job_id}` immediately; facts are saved in the background. `memory_extract(auto_save=false)` stays synchronous and returns the extracted facts directly. Eliminates double timeout (MCP gateway 30s + Ollama HTTP 120s) that caused data loss
- **Model switch** (2026-07-13): default model changed from `qwen2.5-coder:7b` to `phi4-mini` after comparative testing. See [DESIGN.md](DESIGN.md) for full test results. `qwen2.5-coder:7b` remains available via `--extract-model` / `--chat-model` flags
- **memory_find** (2026-07-08): keyword search via SQL LIKE on keys and values with Unicode case-insensitivity fallback for Russian. Complements semantic memory_search
- **AsyncWriter** (2026-07-08): non-blocking background writes (1 worker, queue depth 64); memory_save returns immediately while embedding generation runs async. Critical for voice/alice low-latency paths
- **Enriched memory_search** (2026-07-08): server-side enrichment returns `{key, score, content}` instead of raw `{key, score, value: {content, summary}}`
- **memory-cli find** (2026-07-08): new `memory-cli find <keyword>` subcommand
- **memory_dig** (2026-07-08): contextual deep-search — finds entries matching query, builds scenes with time-window context before/after each match, intersects with additional keywords for relevance ranking. Designed for associative/"образная память" queries

## Next Steps (PLAN-002)

Start with **Task 1**: Add `--debug` flag and log levels to `telegram/log.go` and `main.go`.
See [PLAN-002.md](PLAN-002.md) for details.
