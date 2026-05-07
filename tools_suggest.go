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

// ─── memory_suggest ────────────────────────────────────────────────────────────

// memorySuggestTool analyses current context + active goals + history
// and returns proactive suggestions.
func memorySuggestTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_suggest",
		mcp.WithDescription(`Proactive suggestions based on user's goals, history, and current context.
USE THIS when user asks "what should I do", "чем заняться", "что делать", "what's next", "планы", or seems stuck.
Analyzes active goals + timeline + recent memories to recommend next actions.`),
		mcp.WithString("context",
			mcp.Description("Current conversation context for analysis"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of suggestions (default: 5, max: 10)"),
		),
		mcp.WithString("lang",
			mcp.Description("Language for suggestions: 'en' (English, default) or 'ru' (Russian)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			currentContext, _ := args["context"].(string)
			if currentContext == "" {
				currentContext = "current conversation"
			}

			lang, _ := args["lang"].(string)
			if lang == "" {
				lang = "en"
			}

			limit := 5
			if v, ok := args["limit"].(float64); ok {
				limit = int(v)
			}
			if limit > 10 {
				limit = 10
			}
			if limit <= 0 {
				limit = 5
			}

			suggestions, err := s.Suggest(currentContext, limit, lang)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error generating suggestions: %v", err)), nil
			}

			if len(suggestions) == 0 {
				return mcp.NewToolResultText("No suggestions available."), nil
			}

			data, _ := json.MarshalIndent(suggestions, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Suggestions (%d):\n%s", len(suggestions), string(data))), nil
		},
	}
}