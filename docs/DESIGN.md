# Design & Architecture — memory-store-mcp

## Architecture

```txt
┌──────────────────────────────────────────────────────────────────────────────┐
│                         MCP Client (AI Assistant)                              │
│              initialize → tools/list → resources/ → tools/call                │
└─────────────────────────────────┬──────────────────────────────────────────────┘
                                   │ JSON-RPC 2.0 over stdin/stdout
┌──────────────────────────────────▼───────────────────────────────────────────┐
│                         memory-store-mcp (Go binary)                           │
│                                                                                │
│  ┌────────────────────────┐   ┌──────────────────────────────────┐            │
│  │     MCP Server Loop     │   │        memory-cli (Go binary)    │            │
│  │     (ServeStdio)        │   │                                  │            │
│  │                         │   │  Launch → memory-store-mcp via   │            │
│  │  13 tools:              │   │  stdin/stdout MCP connection     │            │
│  │  • memory_save          │   │                                  │            │
│  │  • memory_get           │   │  10 subcommands:                 │            │
│  │  • memory_delete        │   │  • save / get / delete / search  │            │
│  │  • memory_search        │   │  • list / context / extract      │            │
│  │  • memory_list          │   │  • goals / timeline / suggest    │            │
│  │  • memory_get_context   │   │                                  │            │
│  │  • memory_extract       │   └──────────┬───────────────────────┘            │
│  │  • memory_goal_create   │              │ (direct connection,                │
│  │  • memory_goal_list     │              │  no external deps)                 │
│  │  • memory_goal_update   │                                                  │
│  │  • memory_goal_delete   │                                                  │
│  │  • memory_timeline      │   ┌─────────────────────────────────────┐        │
│  │  • memory_suggest       │   │        keyvalembd Library            │        │
│  │                         │   │  ┌───────────────────────────────┐  │        │
│  │  5 resources:           │   │  │  libSQL (libsql-server or     │  │        │
│  │  • context/current      │──▶│  │  SQLite via go-libsql)        │  │        │
│  │  • goals/active         │   │  │  kv_data                      │  │        │
│  │  • timeline/today       │   │  │  kv_embeddings                │  │        │
│  │  • insights/recent      │   │  │  goals                        │  │        │
│  │  • awareness            │   │  │  timeline_events              │  │        │
│  └─────────────────────────┘   │  └───────────┬───────────────────┘  │        │
│                                 │              │                      │        │
│  ┌─────────────────────────┐   │  ┌────────────▼──────────────────┐  │        │
│  │    Telegram Bot (opt.)   │   │  │  Ollama Embedder              │  │        │
│  │                         │   │  │  (embeddinggemma, 768d)       │  │        │
│  │  Notebook mode:         │   │  └───────────────────────────────┘  │        │
│  │  • classifyMessage()    │   │                                      │        │
│  │  • note / goal /        │   │  ┌───────────────────────────────┐  │        │
│  │    question / command   │   │  │  Ollama Chat LLM              │  │        │
│  │                         │   │  │  (phi4-mini / configurable)   │  │        │
│  │  Assistant mode:        │   │  │                                │  │        │
│  │  • /search /goals       │   │  │  Used for: extraction          │  │        │
│  │  • /goal /timeline      │   │  │  and suggestions               │  │        │
│  │  • /suggest /context    │   │  └───────────────────────────────┘  │        │
│  │  • /digest /language    │   └─────────────────────────────────────┘        │
│  │                         │                                                  │
│  │  Access control:        │                                                  │
│  │  TELEGRAM_ALLOWED_USERS │                                                  │
│  │                         │                                                  │
│  │  i18n: ru/en            │                                                  │
│  └─────────────────────────┘                                                  │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Why keyvalembd?

- **libSQL backend** — SQL semantics alongside key-value storage, WAL mode for concurrent access
- **S3-like interface** — implements `s3lite.KeyValueStore` for compatibility
- **Built-in embedder** — Ollama-based, auto-generates embeddings on save
- **Semantic search** — cosine similarity in Go, no external vector DB needed

## Database Schema

```sql
-- Primary key-value + metadata
CREATE TABLE kv_data (
    key          TEXT PRIMARY KEY NOT NULL,
    value        BLOB NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    checksum     TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    modified_at  TEXT NOT NULL DEFAULT (datetime('now')),
    metadata     TEXT NOT NULL DEFAULT '{}'
);

-- Embeddings for semantic search (cascading delete)
CREATE TABLE kv_embeddings (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    key        TEXT NOT NULL UNIQUE REFERENCES kv_data(key) ON DELETE CASCADE,
    text       TEXT NOT NULL DEFAULT '',
    embedding  BLOB,           -- []float32 → 4 bytes per float, little-endian
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Tracked goals with progress / priority / deadline
CREATE TABLE IF NOT EXISTS goals (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active',
    labels      TEXT NOT NULL DEFAULT '[]', -- JSON array of strings
    priority    INTEGER NOT NULL DEFAULT 5,
    progress    INTEGER NOT NULL DEFAULT 0,
    deadline    TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Timeline of events (auto-logged)
CREATE TABLE IF NOT EXISTS timeline_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    key        TEXT NOT NULL DEFAULT '',
    summary    TEXT NOT NULL DEFAULT '',
    details    TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

Embedding dimension: 768 (embeddinggemma model).

Tracked goals are also mirrored into `kv_data` under `memory/goals/{status}/{goal_id}` with:

- `content`: goal description
- `summary`: goal title
- `tags`: goal labels
- `source`: `goal-tracker`
- `status`, `priority`, `goal_id`: copied from the goal row

This mirror gives goals semantic search coverage through the existing `kv_embeddings` flow. Updating a goal rewrites the mirror; changing status moves the key between `active`, `completed`, and `archived`; deleting a goal removes the mirror.

## Protocol

Implements MCP (Model Context Protocol) via JSON-RPC 2.0:

1. **`initialize`** — handshake (server identifies as `memory-store-mcp` v1.0.0)
2. **`tools/list`** — returns all 13 tool definitions
3. **`resources/list`** — returns 5 resource definitions
4. **`tools/call`** — executes the requested tool
5. **`resources/read`** — reads the requested resource

## Tool Definitions

### memory_save

- **Purpose**: Save a memory with auto-generated embedding
- **Parameters**: `key` (string, required), `value` (string, required — JSON), `text` (string, required — for embedding), `auto_key` (bool, optional — auto-generate key as `memory/auto/YYYY-MM-DD/<hash>`)
- **Returns**: Success message with key, checksum, size

### memory_get

- **Purpose**: Retrieve a memory by key
- **Parameters**: `key` (string, required)
- **Returns**: Stored JSON value

### memory_delete

- **Purpose**: Delete a memory and its embedding
- **Parameters**: `key` (string, required)
- **Returns**: Success message

### memory_search

- **Purpose**: Semantic search across memories
- **Parameters**: `query` (string, required), `limit` (number, optional, default 10, max 50)
- **Returns**: JSON array of `{key, text, score}` sorted by relevance

### memory_list

- **Purpose**: List memories by key prefix
- **Parameters**: `prefix` (string, required)
- **Returns**: List of keys (S3-style folder semantics)

### memory_get_context

- **Purpose**: Get aggregated relevant context including facts, decisions, and active goals
- **Parameters**: `query` (string, required), `limit` (number, optional, default 5, max 20)
- **Returns**: Formatted text with relevant memories and active goals
- **Note**: Primary tool — always called first when user asks about their work

### memory_extract

- **Purpose**: Auto-extract key facts, decisions, and intentions from conversation
- **Parameters**: `text` (string, required), `auto_save` (bool, optional)
- **Returns**: Structured JSON with extracted facts
- **Note**: Should be called after every meaningful user exchange

### memory_goal_create

- **Purpose**: Create a new tracked goal
- **Parameters**: `title` (string, required), `description` (string, optional), `priority` (number, optional, 0-10), `deadline` (string, optional, ISO 8601), `labels` (string, optional — JSON array or comma-separated list)
- **Returns**: Created goal object with auto-generated ID
- **Behavior**: If `description` contains Markdown subtasks like `- [x]` / `- [ ]`, progress is calculated automatically as completed subtasks divided by total subtasks.

### memory_goal_list

- **Purpose**: List user's active goals and their progress
- **Parameters**: `status` (string, optional — active, completed, archived), `labels` (string, optional — JSON array or comma-separated list)
- **Returns**: JSON array of goals

### memory_goal_update

- **Purpose**: Update an existing goal (title, description, status, deadline, priority, progress)
- **Parameters**: `id` (string, required), plus any of `title`, `description`, `status`, `deadline`, `priority`, `progress`, `labels`
- **Returns**: Updated goal object
- **Behavior**: If `description` is changed and `progress` is omitted, progress is recalculated from Markdown subtasks when present. Status changes move the mirrored memory key.

### memory_goal_delete

- **Purpose**: Delete an existing tracked goal and its mirrored memory entry
- **Parameters**: `id` (string, required)
- **Returns**: Success message

### memory_timeline

- **Purpose**: Get timeline of events for a date range
- **Parameters**: `from` (string, optional, ISO 8601), `to` (string, optional), `limit` (number, optional, default 20, max 100)
- **Returns**: JSON array of timeline entries with event_type, key, summary, created_at

### memory_suggest

- **Purpose**: Proactive suggestions based on goals, history, and current context
- **Parameters**: `context` (string, optional), `limit` (number, optional, default 5, max 10)
- **Returns**: List of suggested next actions ranked by relevance

## MCP Resources

Five dynamic MCP resources provide direct access to aggregated state:

| Resource URI | Description |
| --- | --- |
| `memory://goals/active` | List of currently active goals |
| `memory://awareness` | Aggregated awareness: active goals + today's timeline + recent memories |
| `memory://context/current` | Aggregated relevant context from memory for the current conversation |
| `memory://insights/recent` | Recently noticed patterns and insights from memory |
| `memory://timeline/today` | Memory events from today |

## CLI Client (`memory-cli`)

- **Location**: `cmd/memory-cli/`
- **Implementation**: Uses `mcp-go` client library to connect to memory-store-mcp via stdio
- **Architecture**:
  - Spawns memory-store-mcp as a child process
  - Connects via JSON-RPC 2.0 over stdin/stdout
  - All 10 subcommands map 1:1 to MCP tools
- **Features**:
  - Auto-discovery of memory-store-mcp binary (PATH, same directory, GOPATH/bin)
  - `proxyStderrWithThinking()` — elegant LLM streaming output with "Thinking..." indicator
  - All server flags forwarded: `--db`, `--model`, `--chat-model`
  - Formatted output via `-o` flag: `json`, `table`, `summary` with tabwriter rendering

### Subcommands

| Command | Purpose |
| --- | --- |
| `save` | Save a memory with optional auto_key |
| `get` | Retrieve a memory by key |
| `delete` | Delete a memory by key |
| `search` | Semantic search across memories |
| `list` | List memories by prefix (S3-style) |
| `context` | Get aggregated context for a query |
| `extract` | Extract facts from conversation text |
| `goals` | Create, list, update goals |
| `timeline` | Query timeline events by date range |
| `suggest` | Get proactive suggestions |

## Telegram Bot (Optional)

- **Activated by**: `--telegram <token>` CLI flag
- **Library**: `github.com/go-telegram-bot-api/telegram-bot-api/v5` (long-polling)
- **Architecture**: Two modes co-exist:

### Notebook Mode (auto-classify text messages)

Uses `classifyMessage()` — a purely rule-based classifier (no LLM call) that detects:

| Classified Type | Heuristics | Action |
| --- | --- | --- |
| `note` | Default / no match | Save as memory via `SaveNote` |
| `goal` | Starts with "хочу"/"i want"/"goal:" etc. | Create goal via `CreateGoal` |
| `question` | Starts with "что"/"what"/"how"/ends with `?` | Semantic search + optional LLM answer |
| `command` | Natural language like "покажи цели", "что делать" | Route to corresponding bot command handler |

- Labels extracted from `#hashtags`
- Priority guessed from urgency words ("срочно"/"urgent" → 9, "low priority" → 2)

### Assistant Mode (commands)

| Command | Description |
| --- | --- |
| `/start`, `/help` | Show welcome and help |
| `/search <query>` | Semantic search across memory |
| `/goals` | List active goals |
| `/goal <id>` | Show goal details by ID |
| `/timeline` | Recent events timeline |
| `/suggest` | Get proactive suggestions |
| `/context [query]` | Current context and memory |
| `/digest [day\|week\|month]` | Summary for period |
| `/language <ru\|en>` | Change bot language |

### Access Control

- `TELEGRAM_ALLOWED_USERS` env var — comma-separated Telegram user IDs
- Empty/unset = open to all users
- Access denied message in user's language

### i18n

- Two languages: Russian (default) and English
- All strings stored in `i18n` map in `telegram/i18n.go`
- User language auto-detected from Telegram's `LanguageCode`
- Manual override via `/language` command
- `BotCommand` descriptions registered in BotFather for both languages

### Digest

- `/digest` generates a summary with note count, active goals, total events, and recent entries
- Periods: `day` (default), `week`, `month`
- Based on timeline events + active goals

## Chat Model

- **Purpose**: Powers `memory_extract` (fact extraction), `memory_suggest` (proactive suggestions), and Telegram `/ask` questions
- **Configuration**: `--chat-model` flag or `LLM_MODEL` environment variable
- **Default**: `phi4-mini` (small, fast; runs locally)
- **LLM URL**: `--llm-url` flag (replaces legacy `--ollama-url`), default `http://localhost:11434`
- **Priority chain**: `--chat-model` flag → `LLM_MODULE` env var → `phi4-mini` default
- **LLM URL priority chain**: `--llm-url` flag → `LLM_URL` env var → Ollama default base URL
- **Separate from embedding model**: Chat model is never used as fallback for embeddings, and vice versa
- **API**: Ollama `/api/chat` endpoint with structured JSON response format

### Telegram LLM Question Answering

When a question is asked in Telegram and an LLM processor is available:
1. Search for relevant context via `GetContext`
2. Pass question + context to LLM for a natural answer
3. Fall back to showing raw context results if LLM unavailable

## Environment Configuration

All server configuration is via CLI flags. Only one environment variable is used:

- **`TELEGRAM_ALLOWED_USERS`** — Comma-separated Telegram user IDs for access control
- All other config (`--db`, `--model`, `--chat-model`, `--llm-url`, `--telegram`, `--save-timeout`) via CLI flags

Previously used `os.Getenv` calls have been removed in favour of CLI flags for consistency.

## Unit Tests

- `tools_goals_test.go` — tests for goal CRUD, auto-progress calculation, label parsing
- `llm_test.go` — tests for chat model selection and sanitisation
- Tested via in-memory SQLite databases

## Key Hierarchy (S3-style)

Keys are hierarchical, mimicking S3 object storage:

- `memory/project/cooksy/architecture`
- `memory/project/cooksy/deployment`
- `memory/user/kirill/preferences`
- `memory/auto/YYYY-MM-DD/<hash>` — auto-generated keys
- `memory/technical/go/snippets`
- `memory/technical/ollama/setup`

Folder semantics:

- Keys ending with `/` are folders
- `List(prefix)` collapses sub-paths into folder entries
- Empty prefix `""` lists all top-level entries

## Value Format

Values are JSON with the following recommended structure:

```json
{
  "content":  "Main content of the memory",
  "summary":  "Short summary for quick reference",
  "tags":     ["project", "go", "architecture"],
  "source":   "conversation",
  "timestamp": "2026-04-30T10:00:00Z"
}
```

## Embedding Generation

- Ollama API via `/api/embeddings` endpoint
- Default model: `embeddinggemma:latest`
- Retry logic with exponential backoff (3 attempts)
- Non-fatal: if Ollama unavailable, memory is saved without embedding (semantic search won't find it, but CRUD still works)
- The `text` parameter in `memory_save` is what gets embedded (typically content + summary)

## memory_save Timeout (Issue #18)

To prevent `memory_save` from appearing to hang during slow Ollama embedding generation, the operation is bounded:

- `--save-timeout` CLI flag (default: `60s`) controls the maximum wall-clock time for `memory_save`, including embedding generation
- `Storage.SaveWithTimeout` wraps the synchronous save in a goroutine with a deadline
- The final key (including auto-generated keys) is resolved before the timeout starts, so timeouts always report the exact key under which the save was attempted
- If the deadline elapses, `memory_save` returns a clear error: *"memory_save for key ... timed out after ... (the operation may still complete in the background; if the initial database write finished, the embedding may be pending or skipped)"*
- Per-stage timing logs are emitted to stderr:
  ```
  ⏱ memory_save: key=... marshal=... keyvalembd_set_with_embedding=... total=...
  ```
  The `keyvalembd_set_with_embedding` duration includes the SQLite write, keyvalembd interaction, Ollama embedding request/retries, and embedding upsert combined because `keyvalembd` exposes them as a single operation
- Result text includes elapsed duration so callers can see how long the save took

The timeout is a server-side guard. The underlying Ollama HTTP call still respects its own 30s client timeout, but the MCP client is no longer blocked indefinitely.

## Similarity Search

Cosine similarity computed in Go (not SQL):

```go
cosineSimilarity(a, b) = dot(a,b) / (|a| * |b|)
```

All stored embeddings are fetched and compared in Go. For large collections, SQL-level vector search via libsql vector extension can be added later.

## Graceful Degradation

| Component | Available | Behavior |
| ----------- | ----------- | ---------- |
| Ollama (embed) | ✅ | Full semantic search |
| Ollama (embed) | ❌ | CRUD tools work; search returns error with helpful message |
| Ollama (chat) | ✅ | Extract & suggest work |
| Ollama (chat) | ❌ | Extract/suggest return error; all other tools work |
| Database | ✅ | All tools work |
| Database | ❌ | Server won't start |
| Telegram | ✅ | Full bot functionality |
| Telegram | ❌ | Bot won't start; MCP server continues unaffected |