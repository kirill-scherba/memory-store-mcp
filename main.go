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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/kirill-scherba/memory-store-mcp/telegram"
)

func main() {
	// Command line flags
	dbPath := flag.String("db", "",
		"Path to the database (default: ~/.config/memory-store-mcp/memory.db)")
	chatModel := flag.String("chat-model", defaultLLMModel,
		"LLM chat model for extraction/suggest")
	llmURL := flag.String("llm-url", ollamaBaseURL,
		"LLM API base URL (default: http://localhost:11434)")
	llmAPIKey := flag.String("llm-api-key", "",
		"LLM API key for OpenAI-compatible APIs (e.g. OpenRouter, OpenAI)")
	telegramToken := flag.String("telegram", "",
		"Telegram bot token (enables Telegram bot mode)")
	httpAddr := flag.String("http", "",
		"HTTP listen address (enables StreamableHTTP transport, e.g. ':8080')")
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

	// Set chat model
	setChatModel(*chatModel)

	// Set LLM URL override (--llm-url flag)
	setLLMURL(*llmURL)

	// Set LLM API key (--llm-api-key flag)
	// Supports:
	//   - plain text: "sk-or-..."
	//   - file: prefix: "file:/path/to/keyfile"
	key := *llmAPIKey
	if after, ok := strings.CutPrefix(key, "file:"); ok {
		keyPath := after
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			log.Fatalf("Failed to read LLM API key from %s: %v", keyPath, err)
		}
		key = strings.TrimSpace(string(keyData))
	}
	setLLMAPIKey(key)

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

	// Initialize Storage (keyvalembd + goals db)
	store, err := NewStorage(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	log.Printf("🚀 Starting memory-store-mcp server")
	log.Printf("   DB path:        %s", *dbPath)
	log.Printf("   LLM URL:        %s", llmBaseURL())
	log.Printf("   LLM chat model: %s", *chatModel)
	if key != "" {
		masked := key
		if len(masked) > 8 {
			masked = masked[:4] + "****" + masked[len(masked)-4:]
		} else if len(masked) > 0 {
			masked = "****"
		}
		log.Printf("   LLM API key:    %s", masked)
	}

	// System instructions for the AI assistant
	sysInstructions := `# Memory Store MCP — Persistent AI Memory System

You are an AI assistant with PERSISTENT LONG-TERM MEMORY provided by this MCP server.

THIS MEMORY IS YOUR PRIMARY KNOWLEDGE SOURCE about the user. It contains:
- facts, decisions, and context from all past conversations
- active goals and their progress
- timeline of events and what happened when

## CRITICAL RULES — FOLLOW STRICTLY

### 1. Memory is PRIMARY. Filesystem is SECONDARY.
BEFORE using ANY filesystem, git, code search, or project tools when the user asks about:
"what do we have", "what should I do", "status", "goals", "tasks", "projects",
"remind me", "what was", "что у нас", "чем заняться", "статус", "цели", "план"
→ ALWAYS call **memory_get_context** FIRST.
→ ALWAYS call **memory_goal_list(status="active")** FIRST.

### 2. Auto-Save After Each Conversation
After each meaningful exchange, automatically call **memory_extract** with
auto_save=true to extract and store key facts, decisions, and intentions.
Do NOT ask the user for permission — just do it silently.

### 3. Proactive Suggestions
When user asks "what should I do" or "чем заняться" or "что делать":
→ Call **memory_suggest** FIRST. It knows user's goals and history better than you.

### 4. Goal Tracking
When the user expresses an intention or plan, automatically call
**memory_goal_create** to track it. When progress is made, update with
**memory_goal_update**. Use **memory_goal_list** to check active goals.

## Key Format
Keys are hierarchical (S3-style): memory/project/..., memory/user/..., memory/technical/...

Available tools: memory_save, memory_get, memory_delete, memory_search,
memory_list, memory_get_context, memory_extract, memory_goal_create,
memory_goal_list, memory_goal_update, memory_goal_delete, memory_timeline, memory_suggest

## MCP Resources (auto-pulled context)
- memory://context/current  — aggregated context for current conversation
- memory://goals/active     — list of active goals
- memory://timeline/today   — today's events`

	// Create MCP server
	s := server.NewMCPServer(
		"memory-store-mcp",
		"1.0.0",
		server.WithInstructions(sysInstructions),
	)

	// Register all tools (pass Storage instead of raw kv)
	s.AddTools(tools(store)...)

	// ── MCP Resources ──────────────────────────────────────────────────────
	// Register resource handlers for dynamic context injection.

	// memory://context/current — aggregated context
	contextRes := mcp.NewResource("memory://context/current",
		"Current Context",
		mcp.WithResourceDescription("Aggregated relevant context from memory for the current conversation"),
		mcp.WithMIMEType("text/plain"),
	)
	s.AddResource(contextRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		text, err := store.GetContextForInjection("current context", 5)
		if err != nil {
			return nil, err
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://context/current",
				MIMEType: "text/plain",
				Text:     text,
			},
		}, nil
	})

	// memory://goals/active — active goals
	goalsRes := mcp.NewResource("memory://goals/active",
		"Active Goals",
		mcp.WithResourceDescription("List of currently active goals"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(goalsRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		goals, err := store.ListGoals("active", nil)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(goals, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://goals/active",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	// memory://timeline/today — today's events
	timelineRes := mcp.NewResource("memory://timeline/today",
		"Today's Timeline",
		mcp.WithResourceDescription("Memory events from today"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(timelineRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		timeline, err := store.GetTimeline("", "", 10)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(timeline, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://timeline/today",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	// memory://insights/recent — recent patterns/insights
	insightsRes := mcp.NewResource("memory://insights/recent",
		"Recent Insights",
		mcp.WithResourceDescription("Recently noticed patterns and insights from memory"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(insightsRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Run a general suggestion to get insights
		suggestions, err := store.Suggest("recent patterns and insights", 5, "en")
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(suggestions, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://insights/recent",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	// memory://awareness — aggregated awareness (goals + timeline + recent memories)
	awarenessRes := mcp.NewResource("memory://awareness",
		"Awareness State",
		mcp.WithResourceDescription("Aggregated awareness: active goals + today's timeline + recent memories for full situational awareness"),
		mcp.WithMIMEType("text/plain"),
	)
	s.AddResource(awarenessRes, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		text, err := store.GetContextForInjection("awareness", 10)
		if err != nil {
			return nil, err
		}
		// Also add goals
		goals, err := store.ListGoals("active", nil)
		if err == nil && len(goals) > 0 {
			goalsText := "\n\n## Active Goals\n"
			for _, g := range goals {
				goalsText += fmt.Sprintf("- [%d%%] %s: %s\n", g.Progress, g.Title, g.Description)
			}
			text += goalsText
		}
		// Add timeline
		timeline, err := store.GetTimeline("", "", 5)
		if err == nil && len(timeline) > 0 {
			timelineText := "\n\n## Recent Activity\n"
			for _, e := range timeline {
				date := e.CreatedAt
				if len(date) > 10 {
					date = date[:10]
				}
				timelineText += fmt.Sprintf("- [%s] %s: %s\n", date, e.Key, truncate(e.Value.Content, 80))
			}
			text += timelineText
		}
		if text == "" {
			text = "No awareness data available yet."
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "memory://awareness",
				MIMEType: "text/plain",
				Text:     text,
			},
		}, nil
	})

	log.Printf("✅ Registered 13 tools and 5 resources")

	// ── Telegram Bot (optional) ──────────────────────────────────────────
	if *telegramToken != "" {
		// Initialize file logger for Telegram bot
		logPath := filepath.Join(dir, "telegram.log")
		if err := telegram.InitBotLogger(logPath, 10*1024*1024, 3); err != nil {
			log.Printf("⚠ Failed to initialize bot logger: %v", err)
		} else {
			defer telegram.LogClose()
		}

		token := *telegramToken

		// Parse allowed users from TELEGRAM_ALLOWED_USERS (comma-separated IDs)
		var allowedUsers map[int64]bool
		if au := os.Getenv("TELEGRAM_ALLOWED_USERS"); au != "" {
			allowedUsers = make(map[int64]bool)
			for _, part := range strings.Split(au, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				var id int64
				if _, err := fmt.Sscanf(part, "%d", &id); err == nil {
					allowedUsers[id] = true
				} else {
					log.Printf("⚠ Invalid user ID in TELEGRAM_ALLOWED_USERS: %q", part)
				}
			}
			log.Printf("🔒 Telegram access restricted to %d allowed user(s)", len(allowedUsers))
		}

		// LLMRequest wrapper that converts telegram.ChatMessage to OllamaChatMessage
		llmRequestFn := func(systemPrompt string, messages []telegram.ChatMessage) (string, error) {
			ollamaMessages := make([]OllamaChatMessage, 0, len(messages)+1)
			// System prompt goes first
			ollamaMessages = append(ollamaMessages, OllamaChatMessage{Role: "system", Content: systemPrompt})
			// Convert user/assistant messages
			for _, msg := range messages {
				ollamaMessages = append(ollamaMessages, OllamaChatMessage{Role: msg.Role, Content: msg.Content})
			}
			return generateAnswer(ollamaMessages)
		}

		telegramFuncs := telegram.BotFuncs{
			SaveNote:     store.SaveFromTelegram,
			CreateGoal:   store.CreateGoalFromTelegram,
			UpdateGoal:   store.UpdateGoalFromTelegram,
			DeleteGoal:   store.DeleteGoalFromTelegram,
			DeleteMemory: store.DeleteMemoryFromTelegram,
			GetMemory:    store.GetMemoryFromTelegram,
			Search:       store.SearchFromTelegram,
			ListGoals:    store.ListGoalsFromTelegram,
			GetGoal:      store.GetGoalFromTelegram,
			GetTimeline:  store.GetTimelineFromTelegram,
			Suggest:      store.SuggestFromTelegram,
			GetContext:   store.GetContextFromTelegram,
			LLMProcess:   store.LLMQuestionProcess,
			LLMRequest:   llmRequestFn,
		}

		bot, err := telegram.NewBot(token, telegramFuncs, allowedUsers)
		if err != nil {
			log.Printf("⚠ Failed to start Telegram bot: %v", err)
			log.Printf("   Ignore — continuing with MCP server only")
		} else {
			go bot.Run()
			// Register debug tool for the Telegram LLM agent
			s.AddTools(telegramDebugTool(bot))
		}
	}

	// ── Server Transport ─────────────────────────────────────────────────
	// Start the server over stdin/stdout (JSON-RPC 2.0) or StreamableHTTP
	if *httpAddr != "" {
		log.Printf("🌐 Starting MCP StreamableHTTP server on %s", *httpAddr)
		log.Printf("   HTTP endpoint:   %s/mcp", *httpAddr)

		// Start the HTTP server in a goroutine
		httpServer := server.NewStreamableHTTPServer(s)
		go func() {
			if err := httpServer.Start(*httpAddr); err != nil {
				if err == http.ErrServerClosed {
					log.Printf("HTTP server closed")
					return
				}
				log.Fatalf("⚠ HTTP server error: %v", err)
			}
		}()

		// Wait for SIGINT or SIGTERM, then graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("🛑 Received signal %v, shutting down...", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("⚠ HTTP shutdown error: %v", err)
		}
	} else {
		log.Printf("   Transport:       stdin/stdout (JSON-RPC 2.0)")
		// Start the server over stdin/stdout (JSON-RPC 2.0)
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}
