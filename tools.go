// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// tools returns all MCP tools for memory-store-mcp.
func tools(kv *keyvalembd.KeyValueEmbd) []server.ServerTool {
	return []server.ServerTool{
		memorySaveTool(kv),
		memoryGetTool(kv),
		memoryDeleteTool(kv),
		memorySearchTool(kv),
		memoryListTool(kv),
	}
}

// ─── memory_save ────────────────────────────────────────────────────────────────

// memorySaveTool saves a memory with auto-generated embedding.
func memorySaveTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("memory_save",
		mcp.WithDescription("Save a memory with auto-generated embedding for semantic search."),
		mcp.WithString("key",
			mcp.Description("Hierarchical key (e.g. memory/project/cooksy/architecture)"),
			mcp.Required(),
		),
		mcp.WithString("value",
			mcp.Description("JSON value with content, summary, tags, source, timestamp"),
			mcp.Required(),
		),
		mcp.WithString("text",
			mcp.Description("Text to generate embedding for semantic search"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			key, _ := args["key"].(string)
			value, _ := args["value"].(string)
			text, _ := args["text"].(string)

			if key == "" || value == "" || text == "" {
				return mcp.NewToolResultText("Error: key, value, and text are required"), nil
			}

			info, err := kv.SetWithEmbedding(key, []byte(value), text)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error saving memory: %v", err)), nil
			}

			result := fmt.Sprintf("Memory saved\nKey: %s\nChecksum: %s\nSize: %d bytes",
				key, info.Checksum, info.ContentLength)
			return mcp.NewToolResultText(result), nil
		},
	}
}

// ─── memory_get ─────────────────────────────────────────────────────────────────

// memoryGetTool retrieves a memory by its key.
func memoryGetTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
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
			value, err := kv.Get(key)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			return mcp.NewToolResultText(string(value)), nil
		},
	}
}

// ─── memory_delete ──────────────────────────────────────────────────────────────

// memoryDeleteTool deletes a memory by key.
func memoryDeleteTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
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
			if err := kv.Del(key); err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error deleting memory: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Deleted memory: %s", key)), nil
		},
	}
}

// ─── memory_search ──────────────────────────────────────────────────────────────

// memorySearchTool performs semantic search across memories.
func memorySearchTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("memory_search",
		mcp.WithDescription(`Semantic search across memories. Finds relevant memories by meaning,
not just keywords. Uses Ollama embeddings for vector similarity.`),
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

			results, err := kv.SearchSemantic(query, limit)
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
func memoryListTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
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

			var keys []string
			for key := range kv.List(prefix) {
				keys = append(keys, key)
			}
			if len(keys) == 0 {
				return mcp.NewToolResultText("No memories found."), nil
			}
			result := fmt.Sprintf("Found %d memories:\n%s", len(keys), strings.Join(keys, "\n"))
			return mcp.NewToolResultText(result), nil
		},
	}
}
