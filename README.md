# memory-store-mcp 🧠

**MCP server for persistent AI memory with semantic search.**

Long-term memory for AI assistants (like Baron) that survives sessions. Store facts, observations, and knowledge — find them later by meaning, not just keywords.

## Quick Start

```bash
# Start the server (Ollama with embedding model recommended for semantic search)
ollama pull embeddinggemma:latest
memory-store-mcp

# Save a memory
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "memory_save",
    "arguments": {
      "key": "memory/project/example/architecture",
      "value": "{\"content\": \"The system uses libSQL with Ollama embeddings\", \"summary\": \"Architecture overview\", \"tags\": [\"go\", \"architecture\"], \"source\": \"conversation\"}",
      "text": "The system uses libSQL with Ollama embeddings for semantic search"
    }
  }
}' | memory-store-mcp
```

## Features

- **Persistent memory** — survives AI sessions and restarts
- **Semantic search** — find memories by meaning using vector embeddings (Ollama)
- **Hierarchical keys** — S3-style key structure: `memory/project/...`, `memory/user/...`, `memory/technical/...`
- **JSON values** — structured data with content, summary, tags, source
- **MCP protocol** — standard JSON-RPC 2.0 over stdin/stdout

## Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `memory_save` | Save a memory with auto-generated embedding | key (required), value (required), text (required) |
| `memory_get` | Retrieve a memory by key | key (required) |
| `memory_delete` | Delete a memory by key | key (required) |
| `memory_search` | Semantic search across memories | query (required), limit (optional, default: 10) |
| `memory_list` | List memories by prefix | prefix (required) |

## Installation

### From source

```bash
git clone https://github.com/kirill-scherba/memory-store-mcp.git
cd memory-store-mcp
go build -o memory-store-mcp .
sudo cp memory-store-mcp /usr/local/bin/
```

### Dependencies

- **Go 1.26+** for building
- **Ollama** with embedding model (recommended for semantic search)

## Options

```
Usage: memory-store-mcp [options]

MCP server for persistent AI memory with semantic search.
Communicates via JSON-RPC 2.0 over stdin/stdout.

Options:
  -db string
        Path to the database (default: ~/.config/memory-store-mcp/memory.db)
  -embedding-model string
        Embedding model name (default: embeddinggemma:latest)
  -h    Show help
  -ollama-url string
        Ollama API URL (default: http://localhost:11434)

Environment variables:
  OLLAMA_BASE_URL    Ollama API URL (default: http://localhost:11434)
```

## Examples

### Save memories

```bash
# Save a technical memory
echo '{... memory_save ...}' | memory-store-mcp

# Save a user preference
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"memory_save","arguments":{"key":"memory/user/user1/preferences","value":"{\"content\":\"Prefers dark mode\",\"summary\":\"UI preference\",\"tags\":[\"ui\"]}","text":"User prefers dark mode for the UI"}}}' | memory-store-mcp
```

### Search semantically

```bash
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"memory_search","arguments":{"query":"What architecture decisions were made?","limit":5}}}' | memory-store-mcp
```

### List by prefix

```bash
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"memory_list","arguments":{"prefix":"memory/project/"}}}' | memory-store-mcp
```

### Retrieve by key

```bash
echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"memory_get","arguments":{"key":"memory/project/example/architecture"}}}' | memory-store-mcp
```

### Delete

```bash
echo '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"memory_delete","arguments":{"key":"memory/project/example/architecture"}}}' | memory-store-mcp
```

## Key-value Format

```json
{
  "key": "memory/project/example/architecture",
  "value": {
    "content": "Main content of the memory",
    "summary": "Short summary for quick reference",
    "tags": ["tag1", "tag2"],
    "source": "conversation",
    "timestamp": "2026-04-30T10:00:00Z"
  },
  "text": "Text used for embedding generation (usually content + summary)"
}
```

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

## License

BSD-2-Clause. See LICENSE file.
