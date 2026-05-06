# Status

**Project**: memory-store-mcp — MCP server for persistent AI memory  
**Last updated**: 2026-05-06  
**Status**: ✅ Alpha — all core features implemented and compiling

## Current State

### Implemented (Phase 1 & 2 — complete)

- [x] **Auto-key generation** — `memory_save` supports `auto_key=true` parameter
- [x] **Context injection** — `memory_get_context` aggregates memories + goals for AI prompt
- [x] **Fact extraction** — `memory_extract` uses Ollama chat LLM to extract structured facts; supports `auto_save`
- [x] **Goal tracking** — `memory_goal_create`, `memory_goal_list`, `memory_goal_update`, `memory_goal_delete` with status/progress/priority
- [x] **Goal labels** — goals store `labels` as JSON, create/list/update accept JSON array strings or comma-separated labels, and list can filter by labels
- [x] **Subtask auto-progress** — Markdown checklist items in goal descriptions (`- [x]`, `- [ ]`) calculate goal progress on create and on description update when explicit progress is omitted
- [x] **Goal semantic mirror** — goals are mirrored into `kv_data` at `memory/goals/{status}/{goal_id}` so semantic search and context injection can surface them naturally
- [x] **Timeline** — `memory_timeline` queries events by date range; auto-logged on memory save/extract/goal changes
- [x] **Proactive suggestions** — `memory_suggest` analyzes context + active goals + recent timeline to recommend next actions
- [x] **MCP Resources** — 5 dynamic resources: `memory://context/current`, `memory://goals/active`, `memory://timeline/today`, `memory://insights/recent`, `memory://awareness`
- [x] **System prompt** — Built-in instructions telling AI to auto-save, auto-search, auto-suggest
- [x] **Separate chat model** — `--chat-model` flag, `LLM_MODEL` env var for extraction/suggest (separate from embedding model)
- [x] **Database schema migration** — `goals` table with additive `labels` migration, `timeline_events` table, columns added to `kv_data` via ALTER TABLE
- [x] **Build passes** — `go build -o /tmp/memory-store-mcp-check .` succeeds, server starts with "13 tools and 5 resources"
- [x] **Fix: chat model no longer falls back to embedding model** — `chatModel()` selects `chatModelOverride` → `LLM_MODEL` → `phi4-mini`, never `ollamaModelOverride`
- [x] **CLI client** — `cmd/memory-cli/` (10 subcommands, stdio MCP client, proxyStderrWithThinking, auto-binary discovery)
- [x] **CLI format output** — `-o` flag on goals, timeline, list, search commands supports `json|table|summary` with tabwriter rendering
- [x] **Telegram bot** — optional Telegram integration with /note, /search, /goal, /suggest, /context, /ask commands; access control via TELEGRAM_ALLOWED_USERS
- [x] **Multi-language suggest** — `Suggest()` API accepts `lang` parameter (en/ru); Telegram uses user language preference
- [x] **Fix: Chat model log shows empty string** — `defaultLLMModel` used as fallback for display

### Not Yet Implemented (Phase 3 — future)

- [ ] **HTTP API** — REST endpoint parallel to MCP stdin/stdout. Would enable external agents and push notifications.
- [ ] **Provider abstraction** — Configurable provider for embedding and chat models (currently hardcoded to local Ollama).
- [ ] Scheduler/background daemon for proactive push notifications
- [ ] Web dashboard (Go templates)

## Known Issues

- Timeline auto-logging uses simple timestamp; no timezone support yet.
- Schema migration uses `CREATE TABLE IF NOT EXISTS` and additive `ALTER TABLE` — safe for existing databases.
- `go build ./...` can fail to overwrite `./memory-store-mcp` while that binary is running; build to a different output path for verification or stop the running process first.
- Only local Ollama supported as provider for both embeddings and chat; need provider abstraction.

## MCP Integration

The server is registered in `~/.config/cline/cline_dynamic.json` and works with the current Cline instance. After rebuilding the binary, all 13 tools and 5 resources are available.
