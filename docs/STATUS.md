# Status

**Project**: memory-store-mcp — MCP server for persistent AI memory  
**Last updated**: 2026-05-03  
**Status**: ✅ Alpha — all core features implemented and compiling

## Current State

### Implemented (Phase 1 & 2 — complete)

- [x] **Auto-key generation** — `memory_save` supports `auto_key=true` parameter
- [x] **Context injection** — `memory_get_context` aggregates memories + goals for AI prompt
- [x] **Fact extraction** — `memory_extract` uses Ollama chat LLM to extract structured facts; supports `auto_save`
- [x] **Goal tracking** — `memory_goal_create`, `memory_goal_list`, `memory_goal_update` with status/progress/priority
- [x] **Timeline** — `memory_timeline` queries events by date range; auto-logged on memory save/extract/goal changes
- [x] **Proactive suggestions** — `memory_suggest` analyzes context + active goals + recent timeline to recommend next actions
- [x] **MCP Resources** — 4 dynamic resources: `memory://context/current`, `memory://goals/active`, `memory://timeline/today`, `memory://insights/recent`
- [x] **System prompt** — Built-in instructions telling AI to auto-save, auto-search, auto-suggest
- [x] **Separate chat model** — `--chat-model` flag, `LLM_MODEL` env var for extraction/suggest (separate from embedding model)
- [x] **Database schema migration** — `goals` table, `timeline_events` table, columns added to `kv_data` via ALTER TABLE
- [x] **Build passes** — `go build ./...` succeeds, server starts with "12 tools and 4 resources"

### Not Yet Implemented (Phase 3 — future)

- [ ] HTTP API (REST endpoint parallel to MCP stdin/stdout)
- [ ] Scheduler/background daemon for proactive push notifications
- [ ] Web dashboard (Go templates)
- [ ] go.mod cleanup (remove `replace` directives, publish deps)

## Known Issues

- Chat model defaults to embedding model if not specified; some embedding-only models (like `embeddinggemma`) may not support chat. Recommend setting `--chat-model` to a chat-capable model.
- Timeline auto-logging uses simple timestamp; no timezone support yet.
- Schema migration uses `CREATE TABLE IF NOT EXISTS` and additive `ALTER TABLE` — safe for existing databases.

## MCP Integration

The server is registered in `~/.config/cline/cline_dynamic.json` and works with the current Cline instance. Previous version (without new features) was runnable; after rebuilding the binary, all 12 tools and 4 resources are available.