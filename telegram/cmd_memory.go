// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// cmdMemory handles /memory <key> — shows a memory entry by its key.
func (b *Bot) cmdMemory(msg *tgbotapi.Message, lang string) {
	key := strings.TrimSpace(msg.CommandArguments())
	if key == "" {
		b.sendText(msg.Chat.ID, t("memory_usage", lang))
		return
	}

	jsonStr, err := b.funcs.GetMemory(key)
	if err != nil {
		log.Printf("⚠ GetMemory error: %v", err)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("memory_error", lang), err))
		return
	}
	if jsonStr == "" {
		b.sendText(msg.Chat.ID, t("memory_not_found", lang))
		return
	}

	var mv MemoryValue
	if err := json.Unmarshal([]byte(jsonStr), &mv); err != nil {
		log.Printf("⚠ Error parsing memory JSON: %v", err)
		b.sendText(msg.Chat.ID, t("memory_parse_error", lang))
		return
	}

	// Build formatted response
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>📝 %s</b>\n\n", escapeHTML(mv.Summary)))

	if mv.Content != "" && mv.Content != mv.Summary {
		sb.WriteString(fmt.Sprintf("%s\n\n", escapeHTML(mv.Content)))
	}

	if len(mv.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("🏷 %s\n", formatLabels(mv.Tags)))
	}

	sb.WriteString(fmt.Sprintf("🔑 <code>%s</code>\n", key))

	if mv.Timestamp != "" {
		sb.WriteString(fmt.Sprintf("📅 %s\n", mv.Timestamp))
	}

	result := sb.String()
	if len(result) > 4000 {
		result = result[:4000] + "\n..."
	}

	b.sendText(msg.Chat.ID, result)
}