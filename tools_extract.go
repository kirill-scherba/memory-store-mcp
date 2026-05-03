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

// ─── memory_extract ────────────────────────────────────────────────────────────

// memoryExtractTool extracts structured facts from conversation text using LLM.
func memoryExtractTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_extract",
		mcp.WithDescription("Extract structured facts from conversation text using LLM. Returns facts ready for saving with memory_save."),
		mcp.WithString("text",
			mcp.Description("Conversation text to analyse and extract facts from"),
			mcp.Required(),
		),
		mcp.WithBoolean("auto_save",
			mcp.Description("If true, automatically save extracted facts to memory"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			text, _ := args["text"].(string)
			if text == "" {
				return mcp.NewToolResultText("Error: text is required"), nil
			}

			autoSave, _ := args["auto_save"].(bool)

			if autoSave {
				keys, err := s.ExtractAndSave(text)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf("Error extracting and saving: %v", err)), nil
				}
				if len(keys) == 0 {
					return mcp.NewToolResultText("No facts extracted."), nil
				}
				result := fmt.Sprintf("Extracted and saved %d facts:\n", len(keys))
				for _, k := range keys {
					result += fmt.Sprintf("  - %s\n", k)
				}
				return mcp.NewToolResultText(result), nil
			}

			// Just extract without saving
			facts, err := ExtractFacts(text)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error extracting facts: %v", err)), nil
			}

			if len(facts) == 0 {
				return mcp.NewToolResultText("No facts extracted from the text."), nil
			}

			data, _ := json.MarshalIndent(facts, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Extracted %d facts:\n%s", len(facts), string(data))), nil
		},
	}
}