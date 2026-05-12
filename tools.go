// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// tools returns all MCP tools for memory-store-mcp.
func tools(s *Storage) []server.ServerTool {
	return []server.ServerTool{
		memorySaveTool(s),
		memoryGetTool(s),
		memoryDeleteTool(s),
		memorySearchTool(s),
		memoryListTool(s),
		memoryGetContextTool(s),
		memoryExtractTool(s),
		memoryGoalCreateTool(s),
		memoryGoalListTool(s),
		memoryGoalUpdateTool(s),
		memoryGoalDeleteTool(s),
		memoryTimelineTool(s),
		memorySuggestTool(s),
	}
}

// ─── memory_save ────────────────────────────────────────────────────────────────

// memorySaveTool saves a memory with auto-generated embedding and optional auto-key.
func memorySaveTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_save",
		mcp.WithDescription(`CALL AFTER COMPLETING EVERY SIGNIFICANT ACTION OR TASK — persist work results, decisions, and key facts.
Use for MANUAL structured facts. For auto-extraction from conversation, prefer memory_extract.
Save a memory with auto-generated embedding for semantic search. Supports auto-key generation.

Key pattern: memory/project/<name>/<category>/<id>
Examples:
  memory/project/cooksy/architecture/overview
  memory/project/ai-hub/features/tool-generation
  memory/user/kirill/preferences/editor

value (string) — structured, machine-readable data (JSON-like).
  Used for: configs, status flags, key-value facts.
text (string) — long-form content for semantic search embeddings.
  Used for: notes, observations, documentation, conversation summaries.
  If both provided: value = metadata, text = searchable content.`),
		mcp.WithString("key",
			mcp.Description("Hierarchical key (e.g. memory/project/cooksy/architecture). Optional if auto_key=true. Pattern: memory/project/<name>/<category>/<id>"),
		),
		mcp.WithString("value",
			mcp.Description("JSON value with content, summary, tags, source, timestamp, status, priority, goal_id. Structured data for machine reading."),
			mcp.Required(),
		),
		mcp.WithString("text",
			mcp.Description("Text for embedding + semantic search. Long-form content (notes, docs, observations). Combined with value as metadata."),
			mcp.Required(),
		),
		mcp.WithBoolean("auto_key",
			mcp.Description("If true, auto-generate key as memory/auto/YYYY-MM-DD/<hash>"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			key, _ := args["key"].(string)
			value, _ := args["value"].(string)
			text, _ := args["text"].(string)
			autoKey, _ := args["auto_key"].(bool)

			if value == "" || text == "" {
				return mcp.NewToolResultText("Error: value and text are required"), nil
			}

			// Parse value into MemoryValue
			var memVal MemoryValue
			if err := json.Unmarshal([]byte(value), &memVal); err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error parsing value JSON: %v", err)), nil
			}

			savedKey, err := s.Save(key, &memVal, text, autoKey)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error saving memory: %v", err)), nil
			}

			result := fmt.Sprintf("Memory saved\nKey: %s", savedKey)
			return mcp.NewToolResultText(result), nil
		},
	}
}

// ─── memory_get ─────────────────────────────────────────────────────────────────

// memoryGetTool retrieves a memory by its key.
func memoryGetTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_get",
		mcp.WithDescription("Retrieve a memory by its key. Returns the stored JSON value."),
		mcp.WithString("key",
			mcp.Description("Key of the memory to retrieve"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			key, _ := args["key"].(string)
			if key == "" {
				return mcp.NewToolResultText("Error: key is required"), nil
			}
			val, err := s.Get(key)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			data, _ := json.MarshalIndent(val, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	}
}

// ─── memory_delete ──────────────────────────────────────────────────────────────

// memoryDeleteTool deletes a memory by key.
func memoryDeleteTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_delete",
		mcp.WithDescription("Delete a memory by its key. Also removes its embedding."),
		mcp.WithString("key",
			mcp.Description("Key of the memory to delete"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			key, _ := args["key"].(string)
			if key == "" {
				return mcp.NewToolResultText("Error: key is required"), nil
			}
			if err := s.Delete(key); err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error deleting memory: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Deleted memory: %s", key)), nil
		},
	}
}

// ─── memory_search ──────────────────────────────────────────────────────────────

// memorySearchTool performs semantic search across memories.
func memorySearchTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_search",
		mcp.WithDescription(`Semantic search across memories. Finds relevant memories by meaning,
not just keywords. Uses Ollama embeddings for vector similarity.

Use for FINDING specific information you know exists.
For session overview / "what do we have", prefer memory_get_context.`),
		mcp.WithString("query",
			mcp.Description("Search query describing what you're looking for"),
			mcp.Required(),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default: 10, max: 50)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			query, _ := args["query"].(string)
			if query == "" {
				return mcp.NewToolResultText("Error: query is required"), nil
			}

			limit := 10
			if v, ok := args["limit"].(float64); ok {
				limit = int(v)
			}
			if limit > 50 {
				limit = 50
			}
			if limit <= 0 {
				limit = 10
			}

			results, err := s.Search(query, limit)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Search error: %v\nTip: Ensure Ollama is running and has the embedding model installed.", err)), nil
			}
			if len(results) == 0 {
				return mcp.NewToolResultText("No relevant memories found."), nil
			}

			resultJSON, _ := json.MarshalIndent(results, "", "  ")
			return mcp.NewToolResultText(string(resultJSON)), nil
		},
	}
}

// ─── memory_list ────────────────────────────────────────────────────────────────

// memoryListTool lists memories by prefix.
func memoryListTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_list",
		mcp.WithDescription(`List memories by key prefix. S3-style folder semantics:
- Keys ending with '/' are folders
- Sub-folders collapsed into single entries
- Use "" to list all top-level keys`),
		mcp.WithString("prefix",
			mcp.Description("Key prefix to filter by (e.g. 'memory/project/' or '' for all)"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			prefix, _ := args["prefix"].(string)

			keys, err := s.List(prefix)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error listing memories: %v", err)), nil
			}
			if len(keys) == 0 {
				return mcp.NewToolResultText("No memories found."), nil
			}
			result := fmt.Sprintf("Found %d memories:\n%s", len(keys), strings.Join(keys, "\n"))
			return mcp.NewToolResultText(result), nil
		},
	}
}
