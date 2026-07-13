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
		mcp.WithDescription(`CALL AFTER EVERY MEANINGFUL EXCHANGE OR COMPLETED TASK — auto-extract key facts, decisions, and intentions from conversation.
Use WITH auto_save=true for automatic long-term memory storage.
Do NOT ask user for permission — just do it automatically.
For MANUAL structured facts, prefer memory_save. Use memory_save after memory_extract to persist extracted facts if auto_save was not used.`),
		mcp.WithString("text",
			mcp.Description("Conversation text to analyse and extract facts from"),
			mcp.Required(),
		),
		mcp.WithBoolean("auto_save",
			mcp.Description("If true, automatically save extracted facts to memory with auto-generated keys (memory/auto/...)"),
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

			// When auto_save is false we run extraction synchronously and return the
			// extracted facts directly. This preserves the documented manual workflow:
			// the caller immediately sees the facts and decides whether to call
			// memory_save to persist them.
			if !autoSave {
				facts, err := ExtractFacts(text)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf("Error extracting facts: %v", err)), nil
				}

				result := map[string]any{
					"status": "completed",
					"facts":  facts,
				}
				if len(facts) == 0 {
					result["message"] = "No facts extracted. Nothing to save."
				} else {
					result["message"] = "Extraction completed. Use memory_save to persist facts if needed."
				}

				data, _ := json.MarshalIndent(result, "", "  ")
				return mcp.NewToolResultText(string(data)), nil
			}

			// auto_save=true: submit extraction to the background worker and return
			// immediately. This avoids the double timeout (MCP gateway 30s + Ollama
			// HTTP 120s) that caused facts to be lost on long conversations.
			jobID, err := s.SubmitExtract(text, autoSave)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error submitting extraction: %v", err)), nil
			}

			result := map[string]any{
				"status":  "accepted",
				"job_id":  jobID,
				"message": "Extraction queued. Facts will be saved automatically when the job completes.",
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	}
}