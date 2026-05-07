// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/kirill-scherba/memory-store-mcp/telegram"
)

// telegramDebugTool creates an MCP tool that sends text to the Telegram LLM agent
// and returns the response. This allows testing the LLM agent without Telegram.
func telegramDebugTool(bot *telegram.Bot) server.ServerTool {
	opt := mcp.NewTool("telegram_debug",
		mcp.WithDescription("Debug the Telegram LLM agent by sending text and getting the agent's response. "+
			"Use this to test the LLM agent's ability to understand user intent, save notes, create goals, "+
			"answer questions, and dispatch commands."),
		mcp.WithString("text",
			mcp.Description("The text to send to the Telegram LLM agent"),
			mcp.Required(),
		),
		mcp.WithString("lang",
			mcp.Description("Language code: 'ru' or 'en' (default: 'ru')"),
		),
		mcp.WithNumber("chat_id",
			mcp.Description("Simulated chat ID (default: 123456789)"),
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

			lang, _ := args["lang"].(string)
			if lang == "" {
				lang = "ru"
			}

			chatID := int64(123456789)
			if v, ok := args["chat_id"].(float64); ok {
				chatID = int64(v)
			}

			resp, err := bot.DebugProcess(chatID, text, lang)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}

			return mcp.NewToolResultText(resp), nil
		},
	}
}