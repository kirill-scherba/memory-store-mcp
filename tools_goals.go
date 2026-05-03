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

// ─── memory_goal_create ────────────────────────────────────────────────────────

// memoryGoalCreateTool creates a new tracked goal.
func memoryGoalCreateTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_goal_create",
		mcp.WithDescription("Create a new tracked goal with title, description, priority and optional deadline."),
		mcp.WithString("title",
			mcp.Description("Goal title"),
			mcp.Required(),
		),
		mcp.WithString("description",
			mcp.Description("Goal description"),
		),
		mcp.WithNumber("priority",
			mcp.Description("Priority (0-10, default: 5)"),
		),
		mcp.WithString("deadline",
			mcp.Description("Deadline (ISO 8601 or empty)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			title, _ := args["title"].(string)
			description, _ := args["description"].(string)
			deadline, _ := args["deadline"].(string)

			priority := 5
			if v, ok := args["priority"].(float64); ok {
				priority = int(v)
			}
			if priority < 0 {
				priority = 0
			}
			if priority > 10 {
				priority = 10
			}

			if title == "" {
				return mcp.NewToolResultText("Error: title is required"), nil
			}

			goal, err := s.CreateGoal(title, description, deadline, priority)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error creating goal: %v", err)), nil
			}

			data, _ := json.MarshalIndent(goal, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Goal created:\n%s", string(data))), nil
		},
	}
}

// ─── memory_goal_list ──────────────────────────────────────────────────────────

// memoryGoalListTool lists goals filtered by status.
func memoryGoalListTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_goal_list",
		mcp.WithDescription("List tracked goals. Filter by status: active, completed, archived, or empty for all."),
		mcp.WithString("status",
			mcp.Description("Filter by status: active, completed, archived, or empty for all"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			status, _ := args["status"].(string)

			goals, err := s.ListGoals(status)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error listing goals: %v", err)), nil
			}

			if len(goals) == 0 {
				return mcp.NewToolResultText("No goals found."), nil
			}

			data, _ := json.MarshalIndent(goals, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Found %d goals:\n%s", len(goals), string(data))), nil
		},
	}
}

// ─── memory_goal_update ────────────────────────────────────────────────────────

// memoryGoalUpdateTool updates an existing goal's fields.
func memoryGoalUpdateTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_goal_update",
		mcp.WithDescription("Update an existing goal: title, description, status, deadline, priority, progress."),
		mcp.WithString("id",
			mcp.Description("Goal ID"),
			mcp.Required(),
		),
		mcp.WithString("title",
			mcp.Description("New title (leave empty to keep current)"),
		),
		mcp.WithString("description",
			mcp.Description("New description (leave empty to keep current)"),
		),
		mcp.WithString("status",
			mcp.Description("New status: active, completed, archived"),
		),
		mcp.WithString("deadline",
			mcp.Description("New deadline (ISO 8601)"),
		),
		mcp.WithNumber("priority",
			mcp.Description("New priority (0-10, -1 to keep current)"),
		),
		mcp.WithNumber("progress",
			mcp.Description("New progress (0-100, -1 to keep current)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			id, _ := args["id"].(string)
			title, _ := args["title"].(string)
			description, _ := args["description"].(string)
			status, _ := args["status"].(string)
			deadline, _ := args["deadline"].(string)

			priority := -1
			if v, ok := args["priority"].(float64); ok {
				priority = int(v)
			}
			progress := -1
			if v, ok := args["progress"].(float64); ok {
				progress = int(v)
			}

			if id == "" {
				return mcp.NewToolResultText("Error: id is required"), nil
			}

			goal, err := s.UpdateGoal(id, title, description, status, deadline, priority, progress)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error updating goal: %v", err)), nil
			}

			data, _ := json.MarshalIndent(goal, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Goal updated:\n%s", string(data))), nil
		},
	}
}

// ─── memory_timeline ───────────────────────────────────────────────────────────

// memoryTimelineTool returns timeline events for a period.
func memoryTimelineTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_timeline",
		mcp.WithDescription("Get timeline of memory events for a date range. Shows what happened and when."),
		mcp.WithString("from",
			mcp.Description("Start date (ISO 8601 or YYYY-MM-DD). Empty for beginning."),
		),
		mcp.WithString("to",
			mcp.Description("End date (ISO 8601 or YYYY-MM-DD). Empty for now."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of entries (default: 20, max: 100)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			from, _ := args["from"].(string)
			to, _ := args["to"].(string)

			limit := 20
			if v, ok := args["limit"].(float64); ok {
				limit = int(v)
			}
			if limit > 100 {
				limit = 100
			}
			if limit <= 0 {
				limit = 20
			}

			entries, err := s.GetTimeline(from, to, limit)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error getting timeline: %v", err)), nil
			}

			if len(entries) == 0 {
				return mcp.NewToolResultText("No timeline entries found."), nil
			}

			data, _ := json.MarshalIndent(entries, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Found %d timeline entries:\n%s", len(entries), string(data))), nil
		},
	}
}