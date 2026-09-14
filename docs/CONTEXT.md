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

## 2026-09-14 — Graph Phase 1: entity registry and relation vocabulary

The graph was a bag of triples. Production showed it: `chapter-08-baron-sees`
and `chapter-08-baron-sees.md` were two nodes for one document, memoir keys
competed with real people, `Сварня -> рубиновая: заказал` claimed the place
ordered the dish, and nine relations existed exactly once.

Three pieces were missing and are now in place:

- **`graph_aliases`** — alias_key -> entity_id. `resolveEntityID`,
  `edgesForEntity` and `neighborsForEntity` all resolve through it, so a
  retired spelling still finds the canonical node's edges. A table, not a JSON
  column: alias lookup must be an index seek.
- **Entity types** — closed set (person, place, dish, project, tool, image,
  doc, org, event, idea, thing). Relations declare the types they accept at
  each end. Types come from unambiguous name patterns and from the relations an
  entity takes part in (subject of `был_в` is a person, object is a place), so
  no seed list of names is needed.
- **Relation vocabulary** — closed. `canonicalRelation` folds extractor
  spellings on write (`заказала` -> `заказал`); `relationReport` lists what
  falls outside as the work queue. Unknown relations are stored as written.

`mergeEntities` re-points edges, collapses duplicates and retires the dropped
name into aliases. `graph_repair` runs the whole pass; `graph_merge`,
`graph_alias`, `graph_relations` expose the pieces, with CLI subcommands.

**Applied to production:** entities 228 -> 227, edges 422 -> 422 (no fact
lost), untyped 146 -> 28, 91 pattern types, 27 propagated, 1 document
duplicate merged, 3 relation rows folded. Types verified correct.

Remaining outside the vocabulary (work queue for extending it):
`читает_почту_через`, `улетела`, `создал`, `путает_имя`,
`проводил_в_аэропорт`, `почта`, `направил`, `может_получить_письмо`,
`запланировал_заказ`.

Commit `1266803`.

## 2026-09-14 — Graph Phase 2: deterministic extractors

The graph only grew when an LLM emitted a triple or someone called
`graph_add_edge`. The extractors read structured memory entries directly: pure
functions over (key, value), no network, no model. A field present means a fact;
a field absent means nothing at all, never a guess.

| extractor | reads | emits |
|---|---|---|
| `place-visit` | an entry with a `place` field | `был_в`, `был_с`, `заказал`, `подают_в` |
| `food-registry` | the registry's `places[]` | places, dishes, visits |
| `person-relation` | a person entry's `relation` | `жена`, `дочь`, ... to Кирилл |
| `mail-sender` | a mail entry's From header | `направил` (display name only) |

They run in the save path (the graph grows by itself) and in `graph_backfill`
for history. Extractors carry **type hints**, used only on entity creation, so
new entities arrive typed and a hint can never overwrite the registry.

**Not extracted, deliberately: tags.** A tag is a topic of a record, not a
relation between two things in the world; wiring tags in would re-introduce the
low-signal edges the vocabulary removed.

**Bugs found:**
- `keyvalembd.List` has S3 folder semantics and collapses sub-folders, so one
  call sees one level only. `Storage.allKeys` walks recursively.
- `edgesForEntity` / `neighborsForEntity` ignored the alias registry.

**Production:** 5507 entries scanned, 51 matched, 116 triples. Entities
227 → 287, edges 422 → 532, untyped 26 of 287 (was 146 of 228).

Commit `a14718d`.

## 2026-09-14 — Graph Phase 3: vocabulary-bound LLM extraction

The extractor asked for "a short lowercase Russian verb with underscores" and
listed examples with a trailing `...` — an invitation to invent. Nine relations
existed exactly once as a result.

**The prompt is now generated from the vocabulary** (`relationMenu()`), so the
two cannot drift apart; a test fails if they diverge. Document relations are
excluded — the extractor records the world, not the archive.

**A relation outside the vocabulary is refused**, not stored. The model is told
to copy one exactly and to omit the triple when none fits, so an outsider is a
model failure. Measured: phi4-mini answered `works_with` / `travels_in` instead
of the relations it was given. Facts are still saved; the count is reported.

**`graph_backfill_llm`** covers prose the deterministic extractors cannot read.
Bounded by `limit`, resumable via a stored cursor, scoped by `prefix`.

**Measured yield — the honest headline:**

| source | entries | usable relations | cost |
|---|---|---|---|
| deterministic extractors | 51 | 116 | free |
| LLM backfill (cloud) | 32 | 1 | ~15 s/entry |

The store is mostly machine state and technical notes; the narrative worth
extracting lives in the memoir files, which are not memory entries. The
backfill stays (right tool once that text is ingested); a mass run over the
store as it stands is not worth the calls. The section list was cut to the
narrative ones on the strength of this.

**Bugs found:**
- `deepseek-v4-flash` put a backslash before a closing quote
  (`"date": "2026-08-14\"`) and then escaped every following quote, breaking
  extraction entirely. Regex patches cannot fix this — repairing the first
  defect only moves the error down. `repairJSONStrings` walks the document as a
  token stream and handles the slipped backslash, the unescaped inner quote and
  the over-escaped tail together.
- The CLI gave every MCP session a hard 120 s timeout; a 20-entry backfill ran
  past it and printed nothing while the server finished the work.
  `mcpCallTimeout` is now raised by the backfill commands.

Commit `7fc5582`.
