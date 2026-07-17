// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// saveTimeout is the maximum duration allowed for memory_save operations,
// including embedding generation. Set via the --save-timeout flag.
var saveTimeout = 60 * time.Second

// setSaveTimeout sets the save timeout. The default is 60 seconds.
func setSaveTimeout(d time.Duration) {
	if d > 0 {
		saveTimeout = d
	}
}

// logWrap wraps an MCP handler to automatically log usage events.
func logWrap(name string, s *Storage, fn server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		// Extract key/summary from request arguments
		key := ""
		summary := ""
		for _, field := range []string{"key", "project", "query", "id"} {
			if v, ok := args[field].(string); ok && v != "" {
				key = v
				break
			}
		}
		if v, ok := args["summary"].(string); ok && v != "" {
			summary = v
		} else if v, ok := args["text"].(string); ok && v != "" {
			summary = truncate(v, 80)
		}

		s.LogEvent(name, key, summary, "")
		return fn(ctx, request)
	}
}

// tools returns all MCP tools for memory-store-mcp.
func tools(s *Storage) []server.ServerTool {
	return []server.ServerTool{
		{Tool: memorySaveTool(s).Tool, Handler: logWrap("memory_save", s, memorySaveTool(s).Handler)},
		{Tool: memoryGetTool(s).Tool, Handler: logWrap("memory_get", s, memoryGetTool(s).Handler)},
		{Tool: memoryDeleteTool(s).Tool, Handler: logWrap("memory_delete", s, memoryDeleteTool(s).Handler)},
		{Tool: memorySearchTool(s).Tool, Handler: logWrap("memory_search", s, memorySearchTool(s).Handler)},
		{Tool: memoryListTool(s).Tool, Handler: logWrap("memory_list", s, memoryListTool(s).Handler)},
		{Tool: memoryGetContextTool(s).Tool, Handler: logWrap("memory_get_context", s, memoryGetContextTool(s).Handler)},
		{Tool: memoryExtractTool(s).Tool, Handler: logWrap("memory_extract", s, memoryExtractTool(s).Handler)},
		{Tool: memoryGoalCreateTool(s).Tool, Handler: logWrap("memory_goal_create", s, memoryGoalCreateTool(s).Handler)},
		{Tool: memoryGoalListTool(s).Tool, Handler: logWrap("memory_goal_list", s, memoryGoalListTool(s).Handler)},
		{Tool: memoryGoalUpdateTool(s).Tool, Handler: logWrap("memory_goal_update", s, memoryGoalUpdateTool(s).Handler)},
		{Tool: memoryGoalDeleteTool(s).Tool, Handler: logWrap("memory_goal_delete", s, memoryGoalDeleteTool(s).Handler)},
		{Tool: memoryTimelineTool(s).Tool, Handler: logWrap("memory_timeline", s, memoryTimelineTool(s).Handler)},
		{Tool: memorySuggestTool(s).Tool, Handler: logWrap("memory_suggest", s, memorySuggestTool(s).Handler)},
		{Tool: sessionSaveTool(s).Tool, Handler: logWrap("session_save", s, sessionSaveTool(s).Handler)},
		{Tool: sessionGetTool(s).Tool, Handler: logWrap("session_get", s, sessionGetTool(s).Handler)},
		{Tool: sessionListTool(s).Tool, Handler: logWrap("session_list", s, sessionListTool(s).Handler)},
		{Tool: sessionCompactTool(s).Tool, Handler: logWrap("session_compact", s, sessionCompactTool(s).Handler)},
		{Tool: memoryFindTool(s).Tool, Handler: logWrap("memory_find", s, memoryFindTool(s).Handler)},
		{Tool: memoryDigTool(s).Tool, Handler: logWrap("memory_dig", s, memoryDigTool(s).Handler)},
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

			// Preserve arbitrary JSON as Content when no explicit content or summary
			// fields were provided, preventing silent data loss for callers that store
			// raw JSON instead of the MemoryValue structure.
			if memVal.Content == "" && memVal.Summary == "" {
				memVal.Content = value
			}

			res := s.SaveWithTimeout(saveTimeout, key, &memVal, text, autoKey)
			if res.Err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error saving memory: %v", res.Err)), nil
			}

			result := fmt.Sprintf("Memory saved\nKey: %s\nElapsed: %v", res.Key, res.Elapsed)
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

			// Enrich results with full value data (key, score, content snippet)
			type enrichedResult struct {
				Key     string  `json:"key"`
				Score   float64 `json:"score"`
				Content string  `json:"content,omitempty"`
			}
			enriched := make([]enrichedResult, 0, len(results))
			for _, r := range results {
				er := enrichedResult{Key: r.Key, Score: r.Score}
				if val, err := s.Get(r.Key); err == nil && val != nil {
					er.Content = val.Content
					if er.Content == "" {
						er.Content = val.Summary
					}
					er.Content = truncate(er.Content, 200)
				}
				enriched = append(enriched, er)
			}

			resultJSON, _ := json.MarshalIndent(enriched, "", "  ")
			return mcp.NewToolResultText(string(resultJSON)), nil
		},
	}
}

// ─── memory_find ────────────────────────────────────────────────────────────────

// memoryFindTool performs exact keyword search across memories (complements semantic search).
func memoryFindTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_find",
		mcp.WithDescription(`Keyword search across memories. Uses SQL LIKE to find exact matches
in both keys and values. Use this when you know the exact word or phrase
(e.g. a name, place, or project) — complements memory_search which uses semantic/vector search.

Examples: "сварня", "Шашлычная 1957", "Тоша", "issue #5", "email-bridge"`),
		mcp.WithString("keyword",
			mcp.Description("Keyword or phrase to search for (case-insensitive)"),
			mcp.Required(),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default: 20, max: 100)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			keyword, _ := args["keyword"].(string)
			if keyword == "" {
				return mcp.NewToolResultText("Error: keyword is required"), nil
			}

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

			results, err := s.Find(keyword, limit)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			if len(results) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No results found for keyword: %s", keyword)), nil
			}

			var out strings.Builder
			out.WriteString(fmt.Sprintf("Found %d result(s) for %q:\n\n", len(results), keyword))
			for i, r := range results {
				out.WriteString(fmt.Sprintf("%d. %s\n   📅 %s\n   📝 %s\n\n",
					i+1, r.Key, r.CreatedAt, r.Value))
			}
			return mcp.NewToolResultText(out.String()), nil
		},
	}
}

// ─── memory_dig ─────────────────────────────────────────────────────────────────

// memoryDigTool performs contextual deep-search with time windows and keyword
// intersection. Returns structured scenes with context before/after each match.
func memoryDigTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_dig",
		mcp.WithDescription(`Deep contextual search across memories.
Finds entries matching the query, builds scenes with context windows (entries
before and after each match), and optionally intersects with additional keywords
for relevance ranking.

Use this when you need to understand the full picture around a memory event:
- "what happened when we ate khinkali" — shows scenes around khinkali events
- "when I removed the car, what did we discuss" — query + keyword for context

Examples:
  query="khinkali" — finds all khinkali scenes with context
  query="car wash" keywords=["khinkali","soaring plate","story"] — finds car
  wash scenes and ranks by how many of those keywords appear nearby`),
		mcp.WithString("query",
			mcp.Description("Primary search query"),
			mcp.Required(),
		),
		mcp.WithArray("keywords",
			mcp.Description("Additional keywords for relevance ranking (optional)"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("window",
			mcp.Description("Context window duration: '2h', '30m', '1d' (default: '2h')"),
		),
		mcp.WithNumber("max",
			mcp.Description("Maximum number of scenes to return (default: 10, max: 50)"),
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

			// Parse keywords (array of strings)
			var keywords []string
			if kw, ok := args["keywords"].([]interface{}); ok {
				for _, v := range kw {
					if s, ok := v.(string); ok && s != "" {
						keywords = append(keywords, s)
					}
				}
			}

			window := "2h"
			if w, ok := args["window"].(string); ok && w != "" {
				window = w
			}

			max := 10
			if v, ok := args["max"].(float64); ok {
				max = int(v)
			}
			if max <= 0 {
				max = 10
			}
			if max > 50 {
				max = 50
			}

			result, err := s.Dig(query, keywords, window, max)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			if result.Total == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No scenes found for query %q.", query)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Found %d scene(s) for %q:\n%s",
				result.Total, query, string(data))), nil
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
- Use "" to list all top-level keys
- Optional from_date / to_date filters by created_at (ISO 8601, e.g. "2026-07-17T00:00:00Z")`),
		mcp.WithString("prefix",
			mcp.Description("Key prefix to filter by (e.g. 'memory/project/' or '' for all)"),
			mcp.Required(),
		),
		mcp.WithString("from_date",
			mcp.Description("Optional: only keys created at or after this time (ISO 8601)")),
		mcp.WithString("to_date",
			mcp.Description("Optional: only keys created at or before this time (ISO 8601)")),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			prefix, _ := args["prefix"].(string)

			var opts []keyvalembd.ListOption
			if fd, ok := args["from_date"].(string); ok && fd != "" {
				t, err := time.Parse(time.RFC3339, fd)
				if err == nil {
					opts = append(opts, keyvalembd.ListWithDateFrom(t))
				}
			}
			if td, ok := args["to_date"].(string); ok && td != "" {
				t, err := time.Parse(time.RFC3339, td)
				if err == nil {
					opts = append(opts, keyvalembd.ListWithDateTo(t))
				}
			}

			keys, err := s.List(prefix, opts...)
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

// ─── session_save ──────────────────────────────────────────────────────────────

// sessionSaveTool saves session state for a project.
func sessionSaveTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("session_save",
		mcp.WithDescription(`Save session state for a project. Stores a latest copy (for restore) and a timestamp snapshot (for history).
Use before disconnecting to preserve current working state: open files, todo progress, pending decisions, context usage.
Session data is stored WITHOUT embedding (exact-key lookup only).`),
		mcp.WithString("project",
			mcp.Description("Project name, e.g. memory-store-mcp"),
			mcp.Required(),
		),
		mcp.WithString("data",
			mcp.Description("JSON blob with session state (open files, current task, todo state, pending decisions, model info, memory_refs)"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			project, _ := args["project"].(string)
			dataStr, _ := args["data"].(string)

			if project == "" || dataStr == "" {
				return mcp.NewToolResultText("Error: project and data are required"), nil
			}

			key, err := s.SessionSave(project, []byte(dataStr))
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error saving session: %v", err)), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf("Session saved\nKey: %s", key)), nil
		},
	}
}

// ─── session_get ─────────────────────────────────────────────────────────────────

// sessionGetTool retrieves the latest session state for a project.
func sessionGetTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("session_get",
		mcp.WithDescription(`Retrieve the latest saved session state for a project. Call at the start of a new session to pick up where you left off.`),
		mcp.WithString("project",
			mcp.Description("Project name, e.g. memory-store-mcp"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			project, _ := args["project"].(string)
			if project == "" {
				return mcp.NewToolResultText("Error: project is required"), nil
			}

			data, err := s.SessionGet(project)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}

			return mcp.NewToolResultText(string(data)), nil
		},
	}
}

// ─── session_list ────────────────────────────────────────────────────────────────

// sessionListTool lists session keys by prefix.
func sessionListTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("session_list",
		mcp.WithDescription(`List session keys by prefix. S3-style folder semantics:
- Keys ending with '/' are folders
- Sub-folders collapsed into single entries
- Use "" to list all top-level session keys`),
		mcp.WithString("prefix",
			mcp.Description("Key prefix to filter by (e.g. 'session/project/' or '' for all)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			prefix, _ := args["prefix"].(string)
			if prefix == "" {
				prefix = "session/"
			}

			keys, err := s.SessionList(prefix)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error listing sessions: %v", err)), nil
			}
			if len(keys) == 0 {
				return mcp.NewToolResultText("No sessions found."), nil
			}

			result := fmt.Sprintf("Found %d session keys:\n%s", len(keys), strings.Join(keys, "\n"))
			return mcp.NewToolResultText(result), nil
		},
	}
}

// ─── session_compact ─────────────────────────────────────────────────────────────

// sessionCompactTool cleans up old timestamped session entries.
func sessionCompactTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("session_compact",
		mcp.WithDescription(`Delete timestamped session snapshots older than max_age. Never deletes */latest keys.
Runs automatically on server startup. Can be called manually to reclaim space.`),
		mcp.WithNumber("max_age_hours",
			mcp.Description("Maximum age in hours (default: 168 = 7 days)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			maxAgeHours := 168
			if v, ok := args["max_age_hours"].(float64); ok {
				maxAgeHours = int(v)
			}
			if maxAgeHours <= 0 {
				maxAgeHours = 168
			}

			deleted, err := s.SessionCompact(time.Duration(maxAgeHours) * time.Hour)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Compact error: %v", err)), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf("Compacted sessions: %d old entries removed", deleted)), nil
		},
	}
}
