# Design & Architecture — memory-store-mcp

## Architecture

```
┌──────────────────────────────────────────────┐
│              MCP Client (AI Assistant)         │
│  initialize → tools/list → tools/call         │
└──────────────────────┬───────────────────────┘
                       │ JSON-RPC 2.0 over stdin/stdout
┌──────────────────────▼───────────────────────┐
│           memory-store-mcp (Go binary)         │
│                                                │
│  ┌────────────┐    ┌──────────────────────┐   │
│  │  MCP Loop   │───▶│  keyvalembd Library  │   │
│  │ ServeStdio  │    │  ┌────────────────┐  │   │
│  │             │    │  │  libSQL (SQLite)│  │   │
│  │  5 tools:   │    │  │  kv_data       │  │   │
│  │  • save     │    │  │  kv_embeddings │  │   │
│  │  • get      │    │  └───────┬────────┘  │   │
│  │  • delete   │    │          │            │   │
│  │  • search   │    │  ┌───────▼────────┐  │   │
│  │  • list     │    │  │ Ollama Embedder│  │   │
│  └────────────┘    │  │ (embeddinggemma)│  │   │
│                    │  └────────────────┘  │   │
│                    └──────────────────────┘   │
└──────────────────────────────────────────────┘
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
```

Embedding dimension: 768 (embeddinggemma model).

## Protocol

Implements MCP (Model Context Protocol) via JSON-RPC 2.0:

1. **`initialize`** — handshake (server identifies as `memory-store-mcp` v0.1.0)
2. **`tools/list`** — returns all 5 tool definitions
3. **`tools/call`** — executes the requested tool

## Tool Definitions

### memory_save
- **Purpose**: Save a memory with auto-generated embedding
- **Parameters**: `key` (string, required), `value` (string, required — JSON), `text` (string, required — for embedding)
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

## Key Hierarchy (S3-style)

Keys are hierarchical, mimicking S3 object storage:

- `memory/project/cooksy/architecture`
- `memory/project/cooksy/deployment`
- `memory/user/kirill/preferences`
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
| Ollama | ✅ | Full semantic search |
| Ollama | ❌ | CRUD tools work; search returns error with helpful message |
| Database | ✅ | All tools work |
| Database | ❌ | Server won't start |
