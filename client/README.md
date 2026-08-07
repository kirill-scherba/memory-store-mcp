# memory-store-mcp/client

Minimal Go client for [memory-store-mcp](../) — speaks MCP JSON-RPC over HTTP.

Zero external dependencies (standard library only). The endpoint defaults to
`http://127.0.0.1:7708/mcp` and can be overridden with `MEMORY_URL`, so any
host can reach a remote memory store.

## Usage

```go
import "github.com/kirill-scherba/memory-store-mcp/client"

mem := client.New()
ctx := context.Background()

// Save a value (JSON string) with searchable text.
err := mem.Save(ctx, "memory/project/scheduler/tasks", `{"total":1}`, "scheduler task snapshot")

// Read it back.
entry, err := mem.Get(ctx, "memory/project/scheduler/tasks")

// List keys under a prefix.
keys, err := mem.List(ctx, "memory/project/scheduler/")

// Delete a key.
err = mem.Delete(ctx, "memory/project/scheduler/tasks")

// Semantic search.
results, err := mem.Search(ctx, "scheduler tasks", 10)
```

## Methods

| Method | MCP tool | Purpose |
|--------|----------|---------|
| `Save(ctx, key, value, text)` | memory_save | store value + searchable text |
| `Get(ctx, key)` | memory_get | read entry by key |
| `List(ctx, prefix)` | memory_list | list keys under prefix |
| `Delete(ctx, key)` | memory_delete | remove a key |
| `Search(ctx, query, limit)` | memory_search | semantic search |
