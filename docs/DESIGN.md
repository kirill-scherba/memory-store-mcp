# Design & Architecture — memory-store-mcp

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                     MCP Client (AI Assistant)                        │
│          initialize → tools/list → resources/ → tools/call          │
└────────────────────────────┬────────────────────────────────────────┘
                              │ JSON-RPC 2.0 over stdin/stdout
┌─────────────────────────────▼──────────────────────────────────────┐
│                    memory-store-mcp (Go binary)                     │
│                                                                     │
│  ┌─────────────────────┐   ┌───────────────────────────────────┐   │
│  │    MCP Server Loop   │   │        memory-cli (Go binary)     │   │
│  │    (ServeStdio)      │   │                                   │   │
│  │                      │   │  Launch → memory-store-mcp via    │   │
│  │  12 tools:           │   │  stdin/stdout MCP connection      │   │
│  │  • memory_save       │   │                                   │   │
│  │  • memory_get        │   │  10 subcommands:                  │   │
│  │  • memory_delete     │   │  • save / get / delete / search   │   │
│  │  • memory_search     │   │  • list / context / extract       │   │
│  │  • memory_list       │   │  • goals / timeline / suggest     │   │
│  │  • memory_get_context│   │                                   │   │
│  │  • memory_extract    │   └──────────┬────────────────────────┘   │
│  │  • memory_goal_create│              │ (direct connection,       │
│  │  • memory_goal_list  │              │  no external deps)         │
│  │  • memory_goal_update│                                           │
│  │  • memory_timeline   │   ┌──────────────────────────────┐       │
│  │  • memory_suggest    │   │      keyvalembd Library       │       │
│  │                      │   │  ┌────────────────────────┐  │       │
│  │  4 resources:        │   │  │  libSQL (libsql-server │  │       │
│  │  • context/current   │──▶│  │  or SQLite via go-libsql)│  │       │
│  │  • goals/active      │   │  │  kv_data                │  │       │
│  │  • timeline/today    │   │  │  kv_embeddings          │  │       │
│  │  • insights/recent   │   │  │  goals                  │  │       │
│  └──────────────────────┘   │  │  timeline_events        │  │       │
│                              │  └───────────┬────────────┘  │       │
│                              │               │              │       │
│                              │  ┌────────────▼───────────┐  │       │
│                              │  │  Ollama Embedder       │  │       │
│                              │  │  (embeddinggemma, 768d)│  │       │
│                              │  └────────────────────────┘  │       │
│                              │                               │       │
│                              │  ┌────────────────────────┐  │       │
│                              │  │  Ollama Chat LLM       │  │       │
│                              │  │  (phi4-mini / config.) │  │       │
│                              │  │  Used for: extraction  │  │       │
│                              │  │  and suggestions       │  │       │
│                              │  └────────────────────────┘  │       │
│                              └──────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────────┘
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

## Protocol

Implements MCP (Model Context Protocol) via JSON-RPC 2.0:

1. **`initialize`** — handshake (server identifies as `memory-store-mcp` v0.1.0)
2. **`tools/list`** — returns all 12 tool definitions
3. **`resources/list`** — returns 4 resource definitions
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
- **Parameters**: `title` (string, required), `description` (string, optional), `priority` (number, optional, 0-10), `deadline` (string, optional, ISO 8601)
- **Returns**: Created goal object with auto-generated ID

### memory_goal_list
- **Purpose**: List user's active goals and their progress
- **Parameters**: `status` (string, optional — active, completed, archived)
- **Returns**: JSON array of goals

### memory_goal_update
- **Purpose**: Update an existing goal (title, description, status, deadline, priority, progress)
- **Parameters**: `id` (string, required), plus any of `title`, `description`, `status`, `deadline`, `priority`, `progress`
- **Returns**: Updated goal object

### memory_timeline
- **Purpose**: Get timeline of events for a date range
- **Parameters**: `from` (string, optional, ISO 8601), `to` (string, optional), `limit` (number, optional, default 20, max 100)
- **Returns**: JSON array of timeline entries with event_type, key, summary, created_at

### memory_suggest
- **Purpose**: Proactive suggestions based on goals, history, and current context
- **Parameters**: `context` (string, optional), `limit` (number, optional, default 5, max 10)
- **Returns**: List of suggested next actions ranked by relevance

## MCP Resources

Four dynamic MCP resources provide direct access to aggregated state:

| Resource URI | Description |
|---|---|
| `memory://goals/active` | List of currently active goals |
| `memory://awareness` | Aggregated awareness: active goals + today's timeline + recent memories |
| `memory://context/current` | Aggregated relevant context from memory for the current conversation |
| `memory://insights/recent` | Recently noticed patterns and insights from memory |
| `memory://timeline/today` | Memory events from today |

## Chat Model

- **Purpose**: Powers `memory_extract` (fact extraction) and `memory_suggest` (proactive suggestions)
- **Configuration**: `--chat-model` flag or `LLM_MODEL` environment variable
- **Default**: `phi4-mini` (small, fast; runs locally)
- **Separate from embedding model**: Chat model is never used as fallback for embeddings, and vice versa
- **API**: Ollama `/api/chat` endpoint with structured JSON response format

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

### Subcommands

| Command | Purpose |
|---|---|
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

## Similarity Search

Cosine similarity computed in Go (not SQL):

```go
cosineSimilarity(a, b) = dot(a,b) / (|a| * |b|)
```

All stored embeddings are fetched and compared in Go. For large collections, SQL-level vector search via libsql vector extension can be added later.

## Graceful Degradation

| Component | Available | Behavior |
|-----------|-----------|----------|
| Ollama (embed) | ✅ | Full semantic search |
| Ollama (embed) | ❌ | CRUD tools work; search returns error with helpful message |
| Ollama (chat) | ✅ | Extract & suggest work |
| Ollama (chat) | ❌ | Extract/suggest return error; all other tools work |
| Database | ✅ | All tools work |
| Database | ❌ | Server won't start |