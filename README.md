# memory-store-mcp 🧠

**MCP server for persistent AI memory with semantic search, goal tracking, and proactive suggestions.**

Long-term memory for AI assistants that survives sessions. Store facts, observations, goals, and knowledge — find them later by meaning, not just keywords. The assistant automatically saves context and injects relevant memories before each response.

Supports two transport modes:
- **stdin/stdout** (default) — JSON-RPC 2.0 over standard I/O, ideal for local MCP integration
- **HTTP/SSE** (via `--http` flag) — HTTP server with Server-Sent Events, ideal for remote clients and multi-client access

## Quick Start

```bash
# Start the server (Ollama with embedding model required)
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
- **Auto-key generation** — pass `auto_key=true` to generate keys automatically as `memory/auto/YYYY-MM-DD/<hash>`
- **Context injection** — aggregate relevant memories and active goals for AI context
- **Fact extraction** — extract structured facts from conversation text using local LLM
- **Goal tracking** — create, list, and update goals with progress and status
- **Timeline** — view memory events over time periods
- **Proactive suggestions** — analyze context + goals + history to suggest next actions
- **CLI client** — `memory-cli` wraps the MCP server for shell usage
- **Telegram bot** — optional notebook/assistant mode via `--telegram`
- **MCP Resources** — dynamic resources auto-pulled by the assistant: current context, active goals, today's timeline, recent insights
- **System instructions** — built-in instructions that tell the assistant to auto-save, auto-search, and suggest proactively
- **JSON values** — structured data with content, summary, tags, source, status, priority, goal_id

## Tools

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `memory_save` | Save a memory with auto-generated embedding | key (optional if auto_key=true), value (required), text (required), auto_key (optional) |
| `memory_get` | Retrieve a memory by key | key (required) |
| `memory_delete` | Delete a memory by key | key (required) |
| `memory_search` | Semantic search across memories | query (required), limit (optional) |
| `memory_list` | List memories by prefix | prefix (required) |
| `memory_get_context` | Get aggregated context for AI injection | query (required), limit (optional) |
| `memory_extract` | Extract structured facts from conversation text | text (required), auto_save (optional) |
| `memory_goal_create` | Create a goal | title (required), description, priority, deadline, labels |
| `memory_goal_list` | List goals by status and labels | status, labels |
| `memory_goal_update` | Update a goal | id (required), title, description, status, deadline, priority, progress, labels |
| `memory_goal_delete` | Delete a goal | id (required) |
| `memory_timeline` | Get timeline events for a period | from, to, limit |
| `memory_suggest` | Get proactive suggestions | context, limit |

## MCP Resources

| Resource | Description |
|----------|-------------|
| `memory://context/current` | Aggregated relevant context from memory for current conversation |
| `memory://goals/active` | JSON list of currently active goals |
| `memory://timeline/today` | Memory events from today |
| `memory://insights/recent` | Recently noticed patterns and insights |
| `memory://awareness` | Aggregated awareness: context, active goals, and recent activity |

## System Instructions

The server includes built-in system prompt instructions that tell the AI assistant to:

1. **Auto-Save**: After each meaningful exchange, automatically extract and save key facts without asking permission
2. **Context Injection**: Before each response, call `memory_get_context` to retrieve relevant context
3. **Proactive Suggestions**: At session start or when user asks "what should I do", call `memory_suggest`
4. **Goal Tracking**: Automatically create goals when user expresses intentions, update progress

## Installation

### From source

```bash
git clone https://github.com/kirill-scherba/memory-store-mcp.git
cd memory-store-mcp
go build -o memory-store-mcp .
go build -o memory-cli ./cmd/memory-cli
sudo cp memory-store-mcp /usr/local/bin/
sudo cp memory-cli /usr/local/bin/
```

### Dependencies

- **Go 1.26+** for building
- **Ollama** with embedding model (required for semantic search)
- **Ollama chat model** for extraction/suggest features (default: `phi4-mini`)

## Options

```
Usage: memory-store-mcp [options]

MCP server for persistent AI memory with semantic search.
Communicates via JSON-RPC 2.0 over stdin/stdout.

Options:
  -db string
        Path to the database (default: ~/.config/memory-store-mcp/memory.db)
  -chat-model string
        Ollama chat model for extraction/suggest (default: phi4-mini)
  -llm-url string
        LLM API base URL (default: http://localhost:11434)
  -llm-api-key string
        LLM API key for OpenAI-compatible APIs (e.g. OpenRouter, OpenAI)
  -http string
        HTTP listen address (enables HTTP/SSE transport, e.g. :8080)
  -telegram string
        Telegram bot token (enables Telegram bot mode)
  -h    Show help

Environment variables:
  TELEGRAM_ALLOWED_USERS  Optional comma-separated Telegram user IDs for access control
```

Note: the embedding model is configured by the `keyvalembd` storage layer. The server exposes chat-model and LLM URL flags for features that call Ollama chat APIs directly.

## Examples

### Auto-key save

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"memory_save","arguments":{"auto_key":true,"value":"{\"content\":\"Using Go 1.26 with libSQL\",\"summary\":\"Tech stack decision\",\"tags\":[\"go\",\"libsql\"]}","text":"We decided to use Go 1.26 with libSQL for the project"}}}' | memory-store-mcp
```

### Extract and auto-save facts from conversation

```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"memory_extract","arguments":{"text":"User said: I want to use PostgreSQL for the new project because it has better replication. Also we should deploy on AWS EKS.","auto_save":true}}}' | memory-store-mcp
```

### Get context for AI injection

```bash
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"memory_get_context","arguments":{"query":"architecture decisions","limit":5}}}' | memory-store-mcp
```

### Goal tracking

```bash
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"memory_goal_create","arguments":{"title":"Deploy Cooksy to production","description":"Complete CI/CD and deploy to VPS","priority":8,"labels":"deploy,cooksy"}}}' | memory-store-mcp

echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"memory_goal_update","arguments":{"id":"<goal_id>","progress":50,"status":"active"}}}' | memory-store-mcp

echo '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"memory_goal_list","arguments":{"status":"active"}}}' | memory-store-mcp
```

### Proactive suggestions

```bash
echo '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"memory_suggest","arguments":{"context":"Working on deployment infrastructure","limit":5}}}' | memory-store-mcp
```

### Timeline

```bash
echo '{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"memory_timeline","arguments":{"from":"2026-05-01","to":"2026-05-03","limit":10}}}' | memory-store-mcp
```

### Semantic search

```bash
echo '{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"memory_search","arguments":{"query":"What architecture decisions were made?","limit":5}}}' | memory-store-mcp
```

### HTTP/SSE mode

Start the server in HTTP mode:

```bash
# Start with HTTP/SSE transport on port 8080
memory-store-mcp --http :8080

# Or with custom LLM settings
memory-store-mcp --http :8080 --chat-model qwen2.5-coder:7b --llm-url http://localhost:11434
```

Once running, clients connect via SSE. Example using `curl` to list tools:

```bash
# List available tools via the message endpoint
curl -X POST http://localhost:8080/message \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Save a memory via HTTP:

```bash
curl -X POST http://localhost:8080/message \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "memory_save",
      "arguments": {
        "auto_key": true,
        "value": "{\"content\":\"HTTP mode test\",\"summary\":\"Test entry via HTTP\",\"tags\":[\"test\",\"http\"]}",
        "text": "Testing the HTTP/SSE transport mode"
      }
    }
  }'
```

Search via HTTP:

```bash
curl -X POST http://localhost:8080/message \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "memory_search",
      "arguments": {
        "query": "HTTP mode test",
        "limit": 5
      }
    }
  }'
```

Connect to SSE stream:

```bash
curl -N http://localhost:8080/sse
```

## Memory Value Format

```json
{
  "content": "Main content of the memory",
  "summary": "Short summary for quick reference",
  "tags": ["tag1", "tag2"],
  "source": "conversation",
  "timestamp": "2026-04-30T10:00:00Z",
  "status": "active",
  "priority": 5,
  "goal_id": "goal-uuid"
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    AI Assistant (Claude, etc.)          │
│  System Prompt:                                         │
│  1. Before response: call memory_get_context            │
│  2. After response: call memory_save/extract (auto)     │
│  3. Periodically: check memory_suggest                  │
│  MCP Resources (auto-pulled):                           │
│   - memory://context/current                            │
│   - memory://goals/active                               │
└────────────────────┬────────────────────────────────────┘
                     │ JSON-RPC 2.0 over stdin/stdout
┌────────────────────▼────────────────────────────────────┐
│              memory-store-mcp (Go)                      │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │  13 tools: save, get, delete, search, list,       │   │
│  │  get_context, extract, goal_create, goal_list,   │   │
│  │  goal_update, goal_delete, timeline, suggest      │   │
│  └──────────────────────┬───────────────────────────┘   │
│                         │                                │
│  ┌──────────────────────▼───────────────────────────┐   │
│  │         Storage Layer (libSQL/SQLite)             │   │
│  │  • kv_data + kv_embeddings (keyvalembd)           │   │
│  │  • goals (title, progress, status, labels)        │   │
│  │  • timeline_events (key, event, created_at)       │   │
│  │  • Ollama Embedder (embeddinggemma)               │   │
│  │  • Ollama Chat (for extraction/suggest)           │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## MCP Integration

### stdin/stdout mode (default)

To use this server with an MCP-enabled AI assistant, add it to your MCP config:

```json
{
  "mcpServers": {
    "memory-store-mcp": {
      "command": "memory-store-mcp",
      "args": ["--chat-model", "phi4-mini", "--llm-url", "http://localhost:11434"]
    }
  }
}
```

### HTTP/SSE mode (remote clients)

For remote access or multi-client scenarios, run the server in HTTP mode and configure the client to connect via SSE:

```json
{
  "mcpServers": {
    "memory-store-mcp": {
      "command": "memory-store-mcp",
      "args": ["--http", ":8080", "--chat-model", "phi4-mini", "--llm-url", "http://localhost:11434"]
    }
  }
}
```

Then configure your MCP client to connect to the HTTP endpoint:

```json
{
  "mcpServers": {
    "memory-store-mcp": {
      "type": "sse",
      "url": "http://localhost:8080/sse"
    }
  }
}
```

> **Note:** When using HTTP/SSE mode, the `memory-store-mcp` process runs as a standalone HTTP server. Unlike stdin/stdout mode, it does not need to be spawned as a subprocess per client — multiple clients can connect to the same server instance.

## License

BSD-2-Clause. See LICENSE file.
