// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The command is a thin shell around the memory package: the same flags, the same defaults, the same
// transports. Anything that runs memory-store-mcp as a separate process (and several things do) must
// not notice that the code became a library.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kirill-scherba/memory-store-mcp/memory"
)

func main() {
	dbPath := flag.String("db", "", "Path to the database (default: ~/.config/memory-store-mcp/memory.db)")
	chatModel := flag.String("chat-model", "", "LLM chat model for extraction/suggest")
	extractModel := flag.String("extract-model", "", "LLM model for fact extraction (default: same as --chat-model)")
	llmURL := flag.String("llm-url", "", "LLM API base URL (default: http://localhost:11434)")
	llmAPIKey := flag.String("llm-api-key", "", "LLM API key for OpenAI-compatible APIs")
	telegramToken := flag.String("telegram", "", "Telegram bot token (enables Telegram bot mode)")
	httpAddr := flag.String("http", "", "HTTP listen address (enables StreamableHTTP transport, e.g. ':8080')")
	saveTimeout := flag.Duration("save-timeout", 60*time.Second, "Maximum duration for memory_save operations")
	timelineRetention := flag.Duration("timeline-retention", 30*24*time.Hour, "Retention window for timeline_events (0 keeps everything)")
	showHelp := flag.Bool("h", false, "Show help")
	flag.Parse()

	if *showHelp {
		fmt.Fprintf(os.Stderr, "Usage: memory-store-mcp [options]\n\n")
		fmt.Fprintf(os.Stderr, "MCP server for persistent AI memory with semantic search.\n")
		fmt.Fprintf(os.Stderr, "By default, communicates via JSON-RPC 2.0 over stdin/stdout.\n")
		fmt.Fprintf(os.Stderr, "Use --http to enable StreamableHTTP transport.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  TELEGRAM_ALLOWED_USERS  Comma-separated Telegram user IDs (required in Telegram mode)\n")
		os.Exit(0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := memory.Run(ctx, memory.Options{
		DBPath:            strings.TrimSpace(*dbPath),
		ChatModel:         *chatModel,
		ExtractModel:      *extractModel,
		LLMURL:            *llmURL,
		LLMAPIKey:         *llmAPIKey,
		TelegramToken:     *telegramToken,
		HTTPAddr:          *httpAddr,
		SaveTimeout:       *saveTimeout,
		TimelineRetention: *timelineRetention,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "memory-store-mcp: %v\n", err)
		os.Exit(1)
	}
}
