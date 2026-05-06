// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"encoding/json"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleText processes a non-command text message using the LLM agent.
// The agent understands user intent, decides on actions (note, goal, question, etc.),
// and returns structured JSON actions that are dispatched automatically.
// Falls back to the old classifier if LLM is unavailable.
func (b *Bot) handleText(msg *tgbotapi.Message) {
	text := msg.Text
	if text == "" {
		return
	}

	lang := b.getLang(msg.Chat.ID)

	// ── Try LLM agent first ──────────────────────────────────────
	if b.funcs.LLMRequest != nil {
		b.handleTextWithAgent(text, msg.Chat.ID, lang)
		return
	}

	// ── Fallback: old classifier (notebook mode) ────────────────
	b.handleTextClassifierFallback(msg, lang)
}

// handleNote saves a text message as a memory note.
func (b *Bot) handleNote(msg *tgbotapi.Message, cls *ClassificationResult, lang string) {
	key, err := b.funcs.SaveNote(cls.Title, cls.Description, cls.Labels)
	if err != nil {
		log.Printf("⚠ Error saving memory: %v", err)
		b.sendText(msg.Chat.ID, t("note_error", lang))
		return
	}

	log.Printf("✅ Saved note: %s", key)
	b.sendText(msg.Chat.ID, fmt.Sprintf(
		t("note_saved", lang),
		escapeHTML(cls.Title),
		formatLabels(cls.Labels),
		key,
	))
}

// handleGoal creates a goal from a user message.
func (b *Bot) handleGoal(msg *tgbotapi.Message, cls *ClassificationResult, lang string) {
	goalID, err := b.funcs.CreateGoal(cls.Title, cls.Description, "", cls.Priority, cls.Labels)
	if err != nil {
		log.Printf("⚠ Error creating goal: %v", err)
		b.sendText(msg.Chat.ID, t("goal_create_error", lang))
		return
	}

	log.Printf("✅ Created goal: %s", goalID)
	b.sendText(msg.Chat.ID, fmt.Sprintf(
		t("goal_created", lang),
		escapeHTML(cls.Title),
		cls.Priority,
		formatLabels(cls.Labels),
		goalID,
	))
}

// handleCommandRequest processes a natural-language command by dispatching it
// to the corresponding bot command handler.
func (b *Bot) handleCommandRequest(msg *tgbotapi.Message, cls *ClassificationResult, lang string) {
	log.Printf("🔧 Executing natural-language command: %s", cls.Command)

	// Create a synthetic command message
	cmdMsg := *msg
	switch cls.Command {
	case "goals":
		b.cmdGoals(&cmdMsg, lang)
	case "suggest":
		b.cmdSuggest(&cmdMsg, lang)
	case "context":
		b.cmdContext(&cmdMsg, lang)
	case "timeline":
		b.cmdTimeline(&cmdMsg, lang)
	case "digest":
		// Default to daily digest
		cmdMsg.Text = "/digest"
		cmdMsg.Entities = nil
		b.cmdDigest(&cmdMsg, lang)
	case "search":
		// Use the description as the search query
		cmdMsg.Text = "/search " + cls.Description
		cmdMsg.Entities = nil
		b.cmdSearch(&cmdMsg, lang)
	default:
		b.sendText(msg.Chat.ID, fmt.Sprintf(
			t("command_unknown", lang),
			escapeHTML(cls.Description),
		))
	}
}

// handleQuestion processes a question using semantic search + LLM answer.
func (b *Bot) handleQuestion(msg *tgbotapi.Message, cls *ClassificationResult, lang string) {
	b.sendText(msg.Chat.ID, t("question_searching", lang))

	// 1. Search for relevant context
	jsonStr, err := b.funcs.GetContext(cls.Description, 8)
	if err != nil {
		log.Printf("⚠ Error searching: %v", err)
		b.sendText(msg.Chat.ID, t("question_error", lang))
		return
	}

	var ctx ContextResult
	if err := json.Unmarshal([]byte(jsonStr), &ctx); err != nil {
		log.Printf("⚠ Error parsing context JSON: %v", err)
		b.sendText(msg.Chat.ID, t("question_parse_error", lang))
		return
	}

	// 2. If LLM processor is available, use it to generate a natural answer
	if b.funcs.LLMProcess != nil {
		b.handleQuestionWithLLM(msg, cls, &ctx, lang)
		return
	}

	// 3. Fallback: show raw context results
	if len(ctx.Memories) == 0 {
		b.sendText(msg.Chat.ID, t("question_no_results", lang))
		return
	}

	var sb builder
	sb.writeln(t("question_knowledge_title", lang))
	for i, mem := range ctx.Memories {
		summary := mem.Value.Summary
		if summary == "" {
			summary = truncateText(mem.Value.Content, 100)
		}
		sb.writef("%d. <b>%s</b>\n", i+1, escapeHTML(summary))
		if len(mem.Value.Tags) > 0 {
			sb.writef("   🏷 %s\n", formatLabels(mem.Value.Tags))
		}
		sb.writef("   📅 %s\n", mem.CreatedAt)
	}

	b.sendText(msg.Chat.ID, sb.String())
}

// handleQuestionWithLLM uses the LLM to generate a natural answer from context.
func (b *Bot) handleQuestionWithLLM(msg *tgbotapi.Message, cls *ClassificationResult, ctx *ContextResult, lang string) {
	if len(ctx.Memories) == 0 && len(ctx.Goals) == 0 {
		b.sendText(msg.Chat.ID, t("question_no_results", lang))
		return
	}

	// Build context summary for the LLM
	var contextStr string
	if len(ctx.Goals) > 0 {
		contextStr += "## Active Goals\n"
		for _, g := range ctx.Goals {
			contextStr += fmt.Sprintf("- [%d%%] %s: %s\n", g.Progress, g.Title, g.Description)
		}
		contextStr += "\n"
	}
	if len(ctx.Memories) > 0 {
		contextStr += "## Related Memories\n"
		for i, mem := range ctx.Memories {
			summary := mem.Value.Summary
			if summary == "" {
				summary = truncateText(mem.Value.Content, 200)
			}
			contextStr += fmt.Sprintf("%d. %s (relevance: %.0f%%)\n", i+1, summary, mem.Score*100)
			if mem.Key != "" {
				contextStr += fmt.Sprintf("   Key: %s\n", mem.Key)
			}
		}
	}

	answer, err := b.funcs.LLMProcess(cls.Description, contextStr, lang)
	if err != nil {
		log.Printf("⚠ LLM answer error: %v", err)
		// Fallback to raw results
		var sb builder
		sb.writeln(t("question_knowledge_title", lang))
		for i, mem := range ctx.Memories {
			summary := mem.Value.Summary
			if summary == "" {
				summary = truncateText(mem.Value.Content, 100)
			}
			sb.writef("%d. <b>%s</b>\n", i+1, escapeHTML(summary))
			if len(mem.Value.Tags) > 0 {
				sb.writef("   🏷 %s\n", formatLabels(mem.Value.Tags))
			}
		}
		b.sendText(msg.Chat.ID, sb.String())
		return
	}

	b.sendText(msg.Chat.ID, answer)
}

// builder is a simple string builder.
type builder struct {
	data string
}

func (b *builder) writeln(s string) {
	b.data += s + "\n"
}

func (b *builder) writef(format string, args ...interface{}) {
	b.data += fmt.Sprintf(format, args...)
}

func (b *builder) String() string {
	return b.data
}

// ---------------------------------------------------------------------------
// LLM Agent handler
// ---------------------------------------------------------------------------

// handleTextWithAgent sends the user text to the LLM agent using the
// full agent implementation from agent.go (buildAgentSystemPrompt,
// processWithLLMAgent, dispatchAgentCommand). The agent is a full
// assistant: it answers questions, saves notes, creates/updates goals,
// searches, lists goals/timeline, suggests, and deletes memories.
func (b *Bot) handleTextWithAgent(text string, chatID int64, lang string) {
	// Send typing action to show we're working
	b.sendChatAction(chatID)

	// 1. Get context for the agent (relevant memories + goals)
	contextStr := ""
	if b.funcs.GetContext != nil {
		jsonStr, err := b.funcs.GetContext(text, 5)
		if err == nil {
			var ctx ContextResult
			if err := json.Unmarshal([]byte(jsonStr), &ctx); err == nil {
				// Build a compact context string for the agent
				if len(ctx.Goals) > 0 {
					contextStr += "Active goals:\n"
					for _, g := range ctx.Goals {
						contextStr += fmt.Sprintf("- [%d%%] %s: %s\n", g.Progress, g.Title, g.Description)
					}
				}
				if len(ctx.Memories) > 0 {
					contextStr += "\nRecent memories:\n"
					for i, mem := range ctx.Memories {
						if i >= 3 {
							break
						}
						summary := mem.Value.Summary
						if summary == "" {
							summary = truncateText(mem.Value.Content, 100)
						}
						contextStr += fmt.Sprintf("- %s\n", summary)
					}
				}
			}
		}
	}

	// 2. Build user message with context
	userMessage := text
	if contextStr != "" {
		userMessage += "\n\nCurrent context:\n" + contextStr
	}

	// 3. Call the full LLM agent from agent.go
	cmd, err := processWithLLMAgent(userMessage, lang, b.funcs)
	if err != nil {
		log.Printf("⚠ LLM agent error: %v — falling back to classifier", err)
		synthMsg := &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID},
			Text: text,
		}
		b.handleTextClassifierFallback(synthMsg, lang)
		return
	}

	// 4. Dispatch the command
	result := dispatchAgentCommand(cmd, b.funcs, lang)
	if result != "" {
		b.sendText(chatID, result)
	}
}

// handleTextClassifierFallback is the old classifier-based handler, kept as
// a fallback when the LLM agent is unavailable.
func (b *Bot) handleTextClassifierFallback(msg *tgbotapi.Message, lang string) {
	text := msg.Text
	if text == "" {
		return
	}

	result := classifyMessage(text)
	log.Printf("📝 Classified message as: %s (title: %q)", result.Type, result.Title)

	switch result.Type {
	case "note":
		b.handleNote(msg, result, lang)
	case "goal":
		b.handleGoal(msg, result, lang)
	case "command":
		b.handleCommandRequest(msg, result, lang)
	case "question":
		b.handleQuestion(msg, result, lang)
	default:
		b.sendText(msg.Chat.ID, t("default_message", lang))
	}
}

// sendChatAction sends a "typing" action to the chat.
func (b *Bot) sendChatAction(chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	if _, err := b.api.Request(action); err != nil {
		log.Printf("⚠ sendChatAction error: %v", err)
	}
}

// Note: escapeHTML, formatLabels, truncateText are defined in assistant.go
