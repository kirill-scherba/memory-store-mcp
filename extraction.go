// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Extracted fact
// ---------------------------------------------------------------------------

// ExtractedFact is a single fact extracted from conversation text.
type ExtractedFact struct {
	Content string   `json:"content"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags,omitempty"`
}

// extractSystemPrompt returns the system prompt for the extraction LLM call.
func extractSystemPrompt() string {
	return `You are a fact extraction system. Given a conversation text, extract important facts, decisions, intentions, and key information.

For each fact, provide:
1. content: The full original text of the fact
2. summary: A one-line summary (max 100 chars)
3. tags: 2-5 relevant tags as a JSON array

Return ONLY a JSON array of fact objects, nothing else. Example:
[{"content": "Using Go 1.26 with libSQL for the project", "summary": "Tech stack: Go 1.26 + libSQL", "tags": ["go", "libsql", "tech-stack"]}]`
}

// ExtractFacts uses the LLM to extract structured facts from conversation text.
func ExtractFacts(text string) ([]ExtractedFact, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	msg := []OllamaChatMessage{
		{Role: "system", Content: extractSystemPrompt()},
		{Role: "user", Content: text},
	}

	answer, err := generateAnswer(msg)
	if err != nil {
		return nil, fmt.Errorf("LLM extract failed: %w", err)
	}

	// Parse JSON response
	var facts []ExtractedFact
	if err := json.Unmarshal([]byte(answer), &facts); err != nil {
		// Try to extract JSON array from the response
		cleaned := answer
		if idx := strings.Index(answer, "["); idx >= 0 {
			if end := strings.LastIndex(answer, "]"); end > idx {
				cleaned = answer[idx : end+1]
			}
		}
		if err := json.Unmarshal([]byte(cleaned), &facts); err != nil {
			return nil, fmt.Errorf("parse facts JSON: %w (response: %s)", err, answer)
		}
	}

	return facts, nil
}

// ---------------------------------------------------------------------------
// Suggest prompt builder
// ---------------------------------------------------------------------------

// suggestSystemPrompt returns the system prompt for the suggestion LLM call.
func suggestSystemPrompt() string {
	return `You are a proactive assistant that analyses context and goals to suggest next steps.
Return ONLY a JSON array of suggestion objects. Each suggestion has:
- type: one of "reminder", "followup", "goal_next_step", "insight"
- title: short title (max 60 chars)
- description: brief description (max 200 chars)
- priority: integer 0-10

Example:
[{"type":"goal_next_step","title":"Setup CI/CD pipeline","description":"You discussed setting up CI/CD for Cooksy. A good next step would be to define the deployment workflow.","priority":8}]`
}

// SuggestPrompt builds a structured prompt for the suggest LLM call.
func SuggestPrompt(context string) string {
	// The context is already formatted, just ensure it's reasonable
	if len(context) > 4000 {
		context = context[:4000] + "..."
	}
	return context
}

// ---------------------------------------------------------------------------
// Auto-save from conversation
// ---------------------------------------------------------------------------

// ProcessConversation analyses a conversation exchange and saves extracted
// facts automatically. It's called after each user message.
func (s *Storage) ProcessConversation(userMessage, assistantMessage string) error {
	combined := ""
	if userMessage != "" {
		combined += "User: " + userMessage + "\n"
	}
	if assistantMessage != "" {
		combined += "Assistant: " + assistantMessage + "\n"
	}

	if strings.TrimSpace(combined) == "" {
		return nil
	}

	keys, err := s.ExtractAndSave(combined)
	if err != nil {
		return fmt.Errorf("auto-save failed: %w", err)
	}

	if len(keys) > 0 {
		log.Printf("📝 auto-saved %d facts from conversation", len(keys))
	}

	return nil
}

// ---------------------------------------------------------------------------
// Memory tool helper: get context for injection
// ---------------------------------------------------------------------------

// GetContextForInjection retrieves relevant context and formats it for
// injection into the system prompt.
func (s *Storage) GetContextForInjection(query string, limit int) (string, error) {
	ctx, err := s.GetContext(query, limit)
	if err != nil {
		return "", err
	}

	if len(ctx.Memories) == 0 && len(ctx.Goals) == 0 {
		return "", nil
	}

	var parts []string

	if len(ctx.Memories) > 0 {
		parts = append(parts, "=== Relevant memories ===")
		for i, mem := range ctx.Memories {
			parts = append(parts, fmt.Sprintf("%d. [%s] %s (score: %.2f)",
				i+1, mem.CreatedAt[:10], mem.Value.Content, mem.Score))
			if mem.Value.Summary != "" {
				parts = append(parts, fmt.Sprintf("   Summary: %s", mem.Value.Summary))
			}
			if len(mem.Value.Tags) > 0 {
				parts = append(parts, fmt.Sprintf("   Tags: %s", strings.Join(mem.Value.Tags, ", ")))
			}
			if mem.Value.GoalID != "" {
				parts = append(parts, fmt.Sprintf("   Goal: %s", mem.Value.GoalID))
			}
		}
	}

	if len(ctx.Goals) > 0 {
		parts = append(parts, "\n=== Active goals ===")
		for i, g := range ctx.Goals {
			parts = append(parts, fmt.Sprintf("%d. %s [%d%%] priority=%d deadline=%s",
				i+1, g.Title, g.Progress, g.Priority, g.Deadline))
			if g.Description != "" {
				parts = append(parts, fmt.Sprintf("   %s", g.Description))
			}
		}
	}

	return time.Now().UTC().Format("2006-01-02 15:04:05") + "\n" + strings.Join(parts, "\n"), nil
}