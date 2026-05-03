// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// memory-store-mcp — MCP server for persistent AI memory with semantic search.
//
// This server provides long-term memory for AI assistants (like Baron) that
// survives sessions. It stores facts, observations, and knowledge with
// auto-generated embeddings for semantic search.
//
// Architecture:
//   - Uses keyvalembd (libSQL + Ollama embeddings) as storage backend
//   - Implements MCP (Model Context Protocol) via JSON-RPC 2.0 over stdin/stdout
//   - Keys are hierarchical (S3-style): memory/project/..., memory/user/...
//   - Values are JSON: {content, summary, tags, timestamp, source}
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Command line flags
	dbPath := flag.String("db", "",
		"Path to the database (default: ~/.config/memory-store-mcp/memory.db)")
	model := flag.String("model", "embeddinggemma:latest",
		"Ollama embedding model (default: embeddinggemma:latest)")
	chatModel := flag.String("chat-model", "",
		"Ollama chat model for extraction/suggest (default: same as --model)")
	showHelp := flag.Bool("h", false, "Show help")
	flag.Parse()

	if *showHelp {
		fmt.Fprintf(os.Stderr, "Usage: memory-store-mcp [options]\n\n")
		fmt.Fprintf(os.Stderr, "MCP server for persistent AI memory with semantic search.\n")
		fmt.Fprintf(os.Stderr, "Communicates via JSON-RPC 2.0 over stdin/stdout.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  OLLAMA_BASE_URL     Ollama API URL (default: http://localhost:11434)\n")
		fmt.Fprintf(os.Stderr, "  EMBEDDING_MODEL     Embedding model (default: embeddinggemma:latest)\n")
		os.Exit(0)
	}

	// Set model from flags (env vars take precedence)
	if m := os.Getenv("EMBEDDING_MODEL"); m != "" {
		*model = m
	}
	setOllamaModel(*model)

	if *chatModel == "" {
		*chatModel = *model
	}
	setChatModel(*chatModel)

	// Default db path
	if *dbPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			log.Fatalf("Could not determine config directory: %v", err)
		}
		*dbPath = filepath.Join(configDir, "memory-store-mcp", "memory.db")
	}

	// Ensure directory exists
	dir := filepath.Dir(*dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("Could not create database directory %s: %v", dir, err)
	}

	// Initialize Storage (keyvalembd + goals db)
	store, err := NewStorage(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	log.Printf("🚀 Starting memory-store-mcp server")
	log.Printf("   DB path:        %s", *dbPath)
	log.Printf("   Embedding model: %s", *model)
	log.Printf("   Chat model:      %s", *chatModel)

	// System instructions for the AI assistant
	sysInstructions := `# Memory Store MCP — Persistent AI Memory System

This server provides long-term memory for AI assistants, surviving sessions. 
It stores facts, observations, goals, and knowledge with auto-generated 
embeddings for semantic search.

## Key Behaviors (REQUIRED — follow these automatically)

### 1. Auto-Save After Each Conversation
After each meaningful exchange, automatically call **memory_extract** with 
auto_save=true to extract and store key facts, decisions, and intentions. 
Do NOT ask the user for permission — just do it silently.

### 2. Context Injection Before Each Response
Before responding to the user, call **memory_get_context** with the current 
topic to retrieve relevant context from memory. Use this context to inform 
your answer. The context includes:
  - Relevant past memories (from semantic search)
  - Active goals with progress

### 3. Proactive Suggestions
At the beginning of a session, or when the user pauses/asks "what should I do", 
call **memory_suggest** to get proactive suggestions based on:
  - Current conversation context
  - Active goals
  - Recent activity timeline

### 4. Goal Tracking
When the user expresses an intention or plan, automatically call 
**memory_goal_create** to track it. When progress is made, update with 
**memory_goal_update**. Use **memory_goal_list** to check active goals.

## Key Format
Keys are hierarchical (S3-style): memory/project/..., memory/user/..., memory/technical/...

Available tools: memory_save, memory_get, memory_delete, memory_search, 
memory_list, memory_get_context, memory_extract, memory_goal_create, 
memory_goal_list, memory_goal_update, memory_timeline, memory_suggest

## MCP Resources (auto-pulled context)
- memory://context/current  — aggregated context for current conversation
- memory://goals/active     — list of active goals
- memory://timeline/today   — today's events`

	// Create MCP server
	s := server.NewMCPServer(
		"memory-store-mcp",
		"1.0.0",
		server.WithInstructions(sysInstructions),
	)

	// Register all tools (pass Storage instead of raw kv)
	s.AddTools(tools(store)...)

	// ── MCP Resources ──────────────────────────────────────────────────────
	// Register resource handlers for dynamic context injection.

	// memory://context/current — aggregated context
	contextRes := mcp.NewResource("memory://context/current",
		"Current Context",
		mcp.WithResourceDescription("Aggregated relevant context from memory for the current conversation"),
		mcp.WithMIMEType("text/plain"),
	)
	s.AddResource(contextRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		text, err := store.GetContextForInjection("current context", 5)
		if err != nil {
			return nil, err
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://context/current",
				MIMEType: "text/plain",
				Text:     text,
			},
		}, nil
	})

	// memory://goals/active — active goals
	goalsRes := mcp.NewResource("memory://goals/active",
		"Active Goals",
		mcp.WithResourceDescription("List of currently active goals"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(goalsRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		goals, err := store.ListGoals("active")
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(goals, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://goals/active",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	// memory://timeline/today — today's events
	timelineRes := mcp.NewResource("memory://timeline/today",
		"Today's Timeline",
		mcp.WithResourceDescription("Memory events from today"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(timelineRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		timeline, err := store.GetTimeline("", "", 10)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(timeline, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://timeline/today",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	// memory://insights/recent — recent patterns/insights
	insightsRes := mcp.NewResource("memory://insights/recent",
		"Recent Insights",
		mcp.WithResourceDescription("Recently noticed patterns and insights from memory"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(insightsRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Run a general suggestion to get insights
		suggestions, err := store.Suggest("recent patterns and insights", 5)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(suggestions, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://insights/recent",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	log.Printf("✅ Registered 12 tools and 4 resources")

	// Start the server over stdin/stdout (JSON-RPC 2.0)
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

