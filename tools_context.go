// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ─── memory_get_context ────────────────────────────────────────────────────────

// memoryGetContextTool returns aggregated relevant context for injection.
func memoryGetContextTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_get_context",
		mcp.WithDescription(`PRIMARY TOOL — ALWAYS call this FIRST when user asks about their work, projects, goals, tasks, status, or "what to do next".
Gets aggregated relevant context from persistent memory including facts, decisions, and active goals.
This is your MAIN knowledge source about the user. Call BEFORE filesystem/git/code tools.`),
		mcp.WithString("query",
			mcp.Description("Current conversation topic or user query"),
			mcp.Required(),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of memory results (default: 5, max: 20)"),
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

			limit := 5
			if v, ok := args["limit"].(float64); ok {
				limit = int(v)
			}
			if limit > 20 {
				limit = 20
			}
			if limit <= 0 {
				limit = 5
			}

			injection, err := s.GetContextForInjection(query, limit)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error getting context: %v", err)), nil
			}

			if injection == "" {
				return mcp.NewToolResultText("No relevant context found."), nil
			}

			return mcp.NewToolResultText(injection), nil
		},
	}
}

// ─── memory_resources_json (helper for MCP Resources) ──────────────────────────

// memoryResourcesJSON returns the JSON representation of all resources
// that the MCP server exposes.
func memoryResourcesJSON(s *Storage) map[string]string {
	resources := make(map[string]string)

	// memory://context/current
	if ctx, err := s.GetContextForInjection("current context", 5); err == nil {
		resources["memory://context/current"] = ctx
	}

	// memory://goals/active
	if goals, err := s.ListGoals("active"); err == nil {
		if data, err := json.MarshalIndent(goals, "", "  "); err == nil {
			resources["memory://goals/active"] = string(data)
		}
	}

	// memory://timeline/today
	today := "" // today's date as ISO
	if timeline, err := s.GetTimeline(today, "", 10); err == nil {
		if data, err := json.MarshalIndent(timeline, "", "  "); err == nil {
			resources["memory://timeline/today"] = string(data)
		}
	}

	return resources
}