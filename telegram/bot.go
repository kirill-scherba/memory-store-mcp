// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package telegram provides a Telegram bot interface for the memory store.
// It supports notebook mode (save notes and goals via LLM classification)
// and assistant mode (commands for search, goals, timeline, suggest).
package telegram

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotFuncs holds all functional callbacks injected from main.
type BotFuncs struct {
	// SaveNote saves a note and returns the raw memory key.
	SaveNote func(title, description string, tags []string) (string, error)
	// CreateGoal creates a goal -> goalID or error.
	CreateGoal func(title, description, deadline string, priority int, labels []string) (string, error)
	// Search does semantic search -> JSON string array of results.
	Search func(query string, limit int) (string, error)
	// ListGoals lists goals -> JSON string.
	ListGoals func(status string, labelsFilter []string) (string, error)
	// GetGoal gets a goal by ID -> JSON string.
	GetGoal func(id string) (string, error)
	// GetTimeline -> JSON string.
	GetTimeline func(from, to string, limit int) (string, error)
	// Suggest -> JSON string.
	Suggest func(currentContext string, limit int, lang string) (string, error)
	// GetContext -> JSON string.
	GetContext func(query string, limit int) (string, error)
	// LLMProcess answers a question using the LLM given context. Returns HTML-safe text.
	LLMProcess func(question string, context string, lang string) (string, error)
	// UpdateGoal updates a goal. Returns JSON string.
	UpdateGoal func(id, title, description, status, deadline string, priority, progress int, labels []string) (string, error)
	// DeleteMemory deletes a memory by key.
	DeleteMemory func(key string) error
	// LLMRequest sends messages to the LLM and returns the raw text response.
	LLMRequest LLMRequester
}

// Bot wraps the Telegram bot API and links to the memory store via functions.
type Bot struct {
	api          *tgbotapi.BotAPI
	funcs        BotFuncs
	done         chan struct{}
	allowedUsers map[int64]bool // empty = open to all
	mu           sync.RWMutex
	userLang     map[int64]string // chatID -> language code ("ru"/"en")

	debugMode     bool   // debug mode — captures responses instead of sending to Telegram
	debugResponse string
	debugMu       sync.Mutex
}

// NewBot creates a new Telegram bot linked to the given functional callbacks.
// allowedUsers is a set of allowed Telegram user IDs; if nil/empty, all users are allowed.
func NewBot(token string, funcs BotFuncs, allowedUsers ...map[int64]bool) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}
	api.Debug = false

	log.Printf("✅ Telegram bot authorised as @%s", api.Self.UserName)

	var au map[int64]bool
	if len(allowedUsers) > 0 {
		au = allowedUsers[0]
	}

	b := &Bot{
		api:          api,
		funcs:        funcs,
		done:         make(chan struct{}),
		allowedUsers: au,
		userLang:     make(map[int64]string),
	}

	// Register bot commands in BotFather for both languages
	if err := b.setCommands(); err != nil {
		log.Printf("⚠ Failed to set bot commands: %v", err)
	}

	return b, nil
}

// setCommands registers the command list with Telegram via setMyCommands.
func (b *Bot) setCommands() error {
	// Register for both languages and default (fallback)
	type langCmds struct {
		lang string
		cmds []tgbotapi.BotCommand
	}

	for _, lc := range []langCmds{
		{lang: "ru", cmds: toTgCommands(commandDescriptions("ru"))},
		{lang: "en", cmds: toTgCommands(commandDescriptions("en"))},
		{lang: "", cmds: toTgCommands(commandDescriptions("en"))}, // default fallback
	} {
		cfg := tgbotapi.NewSetMyCommands(lc.cmds...)
		if lc.lang != "" {
			cfg.LanguageCode = lc.lang
		}
		if _, err := b.api.Request(cfg); err != nil {
			return fmt.Errorf("set commands for lang %q: %w", lc.lang, err)
		}
	}
	log.Printf("✅ Registered bot commands for ru, en, and default")
	return nil
}

// toTgCommands converts our internal BotCommand slice to tgbotapi.BotCommand slice.
func toTgCommands(cmds []BotCommand) []tgbotapi.BotCommand {
	tg := make([]tgbotapi.BotCommand, len(cmds))
	for i, c := range cmds {
		tg[i] = tgbotapi.BotCommand{
			Command:     c.Command,
			Description: c.Description,
		}
	}
	return tg
}

// Run starts the long-polling loop. Blocks until SIGINT/SIGTERM.
func (b *Bot) Run() {
	defer close(b.done)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	// Handle OS signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("🤖 Telegram bot started, polling...")

	for {
		select {
		case <-sigCh:
			log.Printf("📴 Telegram bot shutting down")
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			b.handleUpdate(update)
		}
	}
}

// handleUpdate routes an incoming update to the appropriate handler.
func (b *Bot) handleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	// Ignore messages from channels that the bot itself sent
	if update.Message.From != nil && update.Message.From.ID == b.api.Self.ID {
		return
	}

	msg := update.Message

	// ── Access control ──────────────────────────────────
	if b.allowedUsers != nil {
		if !b.allowedUsers[msg.From.ID] {
			lang := b.getLang(msg.Chat.ID)
			b.sendText(msg.Chat.ID, t("access_denied", lang))
			return
		}
	}

	// Auto-detect language from Telegram user locale on first message
	b.detectAndSetLang(msg)

	switch {
	case msg.IsCommand():
		b.handleCommand(msg)
	default:
		b.handleText(msg)
	}
}

// detectAndSetLang sets the user's language from Telegram's LanguageCode on first message.
func (b *Bot) detectAndSetLang(msg *tgbotapi.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.userLang[msg.Chat.ID]; exists {
		return // already set
	}

	lang := msg.From.LanguageCode
	if lang == "" {
		lang = "ru"
	} else {
		// Normalise: take first two chars, default to "en"
		if len(lang) >= 2 {
			lang = strings.ToLower(lang[:2])
		} else {
			lang = "en"
		}
		// Only support ru/en
		if lang != "ru" && lang != "en" {
			lang = "en"
		}
	}
	b.userLang[msg.Chat.ID] = lang
}

// getLang returns the language for a given chat ID. Defaults to "ru".
func (b *Bot) getLang(chatID int64) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if lang, ok := b.userLang[chatID]; ok {
		return lang
	}
	return "ru"
}

// setLang sets the language for a given chat ID.
func (b *Bot) setLang(chatID int64, lang string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.userLang[chatID] = lang
}

// handleCommand processes bot commands (/search, /goals, etc.)
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	lang := b.getLang(msg.Chat.ID)

	switch msg.Command() {
	case "start", "help":
		b.cmdHelp(msg, lang)
	case "search":
		b.cmdSearch(msg, lang)
	case "goals":
		b.cmdGoals(msg, lang)
	case "goal":
		b.cmdGoal(msg, lang)
	case "timeline":
		b.cmdTimeline(msg, lang)
	case "suggest":
		b.cmdSuggest(msg, lang)
	case "context":
		b.cmdContext(msg, lang)
	case "digest":
		b.cmdDigest(msg, lang)
	case "language":
		b.cmdLanguage(msg, lang)
	default:
		help := b.buildHelp(lang)
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("unknown_command", lang), escapeHTML(msg.Command()), help))
	}
}

// cmdLanguage handles /language <code> — change bot language.
func (b *Bot) cmdLanguage(msg *tgbotapi.Message, lang string) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.sendText(msg.Chat.ID, fmt.Sprintf(t("language_usage", lang), lang))
		return
	}

	args = strings.ToLower(strings.TrimSpace(args))
	switch args {
	case "ru", "en":
		b.setLang(msg.Chat.ID, args)
		b.sendText(msg.Chat.ID, t("language_changed", args))
	default:
		b.sendText(msg.Chat.ID, t("language_bad_code", lang))
	}
}

// cmdHelp handles /start and /help commands.
func (b *Bot) cmdHelp(msg *tgbotapi.Message, lang string) {
	b.sendText(msg.Chat.ID, b.buildHelp(lang))
}

// buildHelp builds the help message for the given language.
func (b *Bot) buildHelp(lang string) string {
	help := t("help_title", lang)
	cmds := commandDescriptions(lang)

	// Sort alphabetically for consistent output
	sorted := make([]BotCommand, len(cmds))
	copy(sorted, cmds)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Command < sorted[j].Command
	})

	for _, c := range sorted {
		help += fmt.Sprintf("/%s — %s\n", c.Command, c.Description)
	}
	return help
}

// sendText sends a plain text message with HTML parse mode.
// In debug mode, the response is captured instead of sent to Telegram.
func (b *Bot) sendText(chatID int64, text string) {
	b.debugMu.Lock()
	if b.debugMode {
		b.debugResponse += text + "\n---\n"
		b.debugMu.Unlock()
		return
	}
	b.debugMu.Unlock()

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("⚠ telegram send error: %v", err)
	}
}

// sendChatAction sends a "typing" action to the chat.
// In debug mode it's a no-op since there's no real chat connection.
func (b *Bot) sendChatAction(chatID int64) {
	b.debugMu.Lock()
	debug := b.debugMode
	b.debugMu.Unlock()
	if debug {
		return
	}

	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	if _, err := b.api.Request(action); err != nil {
		log.Printf("⚠ sendChatAction error: %v", err)
	}
}

// DebugProcess simulates a text message from chatID in the given language,
// routes it through the LLM agent (handleTextWithAgent), captures the
// response instead of sending to Telegram, and returns it.
// The response is also logged to the file logger (if configured).
func (b *Bot) DebugProcess(chatID int64, text string, lang string) (string, error) {
	b.mu.Lock()
	b.userLang[chatID] = lang
	b.mu.Unlock()

	b.debugMu.Lock()
	b.debugMode = true
	b.debugResponse = ""
	b.debugMu.Unlock()

	err := b.handleTextWithAgent(text, chatID, lang)

	b.debugMu.Lock()
	b.debugMode = false
	resp := b.debugResponse
	b.debugResponse = ""
	b.debugMu.Unlock()

	return resp, err
}
