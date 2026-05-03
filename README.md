# memory-store-mcp 🧠

**MCP server for persistent AI memory with semantic search, goal tracking, and proactive suggestions.**

Long-term memory for AI assistants that survives sessions. Store facts, observations, goals, and knowledge — find them later by meaning, not just keywords. The assistant automatically saves context and injects relevant memories before each response.

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
| `memory_get_context` | Get aggregated context for AI injection | topic (required), limit (optional) |
| `memory_extract` | Extract structured facts from conversation text | text (required), auto_save (optional) |
| `memory_goal_create` | Create a goal | title (required), description (optional), priority (optional) |
| `memory_goal_list` | List goals by status | status (required: active/completed/archived/all) |
| `memory_goal_update` | Update goal progress | goal_id (required), progress (optional), status (optional), note (optional) |
| `memory_timeline` | Get timeline events for a period | start_date (optional), end_date (optional), limit (optional) |
| `memory_suggest` | Get proactive suggestions | context (required), limit (optional) |

## MCP Resources

| Resource | Description |
|----------|-------------|
| `memory://context/current` | Aggregated relevant context from memory for current conversation |
| `memory://goals/active` | JSON list of currently active goals |
| `memory://timeline/today` | Memory events from today |
| `memory://insights/recent` | Recently noticed patterns and insights |

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
sudo cp memory-store-mcp /usr/local/bin/
```

### Dependencies

- **Go 1.26+** for building
- **Ollama** with embedding model (required for semantic search)
- **Ollama chat model** (optional, for extraction/suggest features; defaults to embedding model)

## Options

```
Usage: memory-store-mcp [options]

MCP server for persistent AI memory with semantic search.
Communicates via JSON-RPC 2.0 over stdin/stdout.

Options:
  -db string
        Path to the database (default: ~/.config/memory-store-mcp/memory.db)
  -model string
        Ollama embedding model (default: embeddinggemma:latest)
  -chat-model string
        Ollama chat model for extraction/suggest (default: same as --model)
  -h    Show help

Environment variables:
  OLLAMA_BASE_URL     Ollama API URL (default: http://localhost:11434)
  EMBEDDING_MODEL     Embedding model (default: embeddinggemma:latest)
  LLM_MODEL           Chat model for extraction/suggest
```

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
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"memory_get_context","arguments":{"topic":"architecture decisions","limit":5}}}' | memory-store-mcp
```

### Goal tracking

```bash
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"memory_goal_create","arguments":{"title":"Deploy Cooksy to production","description":"Complete CI/CD and deploy to VPS","priority":"high"}}}' | memory-store-mcp

echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"memory_goal_update","arguments":{"goal_id":"<goal_id>","progress":50,"status":"active","note":"CI pipeline configured"}}}' | memory-store-mcp

echo '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"memory_goal_list","arguments":{"status":"active"}}}' | memory-store-mcp
```

### Proactive suggestions

```bash
echo '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"memory_suggest","arguments":{"context":"Working on deployment infrastructure","limit":5}}}' | memory-store-mcp
```

### Timeline

```bash
echo '{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"memory_timeline","arguments":{"start_date":"2026-05-01","end_date":"2026-05-03","limit":10}}}' | memory-store-mcp
```

### Semantic search

```bash
echo '{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"memory_search","arguments":{"query":"What architecture decisions were made?","limit":5}}}' | memory-store-mcp
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
  "priority": "medium",
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
│  │  12 tools: save, get, delete, search, list,       │   │
│  │  get_context, extract, goal_create, goal_list,   │   │
│  │  goal_update, timeline, suggest                   │   │
│  └──────────────────────┬───────────────────────────┘   │
│                         │                                │
│  ┌──────────────────────▼───────────────────────────┐   │
│  │         Storage Layer (libSQL/SQLite)             │   │
│  │  • kv_data + kv_embeddings (keyvalembd)           │   │
│  │  • goals (title, progress, status, priority)      │   │
│  │  • timeline_events (key, event, created_at)       │   │
│  │  • Ollama Embedder (embeddinggemma)               │   │
│  │  • Ollama Chat (for extraction/suggest)           │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## MCP Integration

To use this server with an MCP-enabled AI assistant, add it to your MCP config:

```json
{
  "mcpServers": {
    "memory-store-mcp": {
      "command": "memory-store-mcp",
      "args": ["--model", "embeddinggemma:latest", "--chat-model", "phi4-mini"],
      "env": {
        "OLLAMA_BASE_URL": "http://localhost:11434"
      }
    }
  }
}
```

## License

BSD-2-Clause. See LICENSE file.