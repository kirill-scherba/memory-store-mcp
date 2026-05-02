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
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Command line flags
	dbPath := flag.String("db", "",
		"Path to the database (default: ~/.config/memory-store-mcp/memory.db)")
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

	// Initialize keyvalembd (libSQL + Ollama embeddings)
	kv, err := keyvalembd.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize keyvalembd: %v", err)
	}
	defer kv.Close()

	log.Printf("🚀 Starting memory-store-mcp server")
	log.Printf("   DB path: %s", *dbPath)

	// Create MCP server
	s := server.NewMCPServer(
		"memory-store-mcp",
		"0.1.0",
		server.WithInstructions(`Memory Store MCP — persistent memory with semantic search for AI assistants.

Keys are hierarchical (S3-style): memory/project/..., memory/user/..., memory/technical/...

Available tools:
- memory_save:    Save a memory with auto-generated embedding for semantic search
- memory_get:     Retrieve a memory by key
- memory_delete:  Delete a memory by key
- memory_search:  Semantic search across memories (find by meaning, not keywords)
- memory_list:    List memories by key prefix (S3-style folder listing)`),
	)

	// Register all tools
	s.AddTools(tools(kv)...)

	log.Printf("✅ Registered 5 tools")

	// Start the server over stdin/stdout (JSON-RPC 2.0)
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
