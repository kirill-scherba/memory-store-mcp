// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

// i18nMap holds all translatable strings for the Telegram bot.
// Key: messageKey, value: map[languageCode]translatedString.
// Supported languages: "ru" (default), "en".
type i18nMap map[string]map[string]string

// i18n is the internationalisation dictionary.
var i18n = i18nMap{
	// ── General ────────────────────────────────────────────
	"unknown_command": {
		"ru": "❓ Неизвестная команда: /%s\n\n%s",
		"en": "❓ Unknown command: /%s\n\n%s",
	},
	"searching": {
		"ru": "🔍 Ищу...",
		"en": "🔍 Searching...",
	},
	"search_usage": {
		"ru": "ℹ️ Использование: /search <запрос>",
		"en": "ℹ️ Usage: /search <query>",
	},
	"search_error": {
		"ru": "❌ Ошибка поиска: %v",
		"en": "❌ Search error: %v",
	},
	"search_parse_error": {
		"ru": "❌ Ошибка при обработке результатов.",
		"en": "❌ Error processing results.",
	},
	"search_no_results": {
		"ru": "🤷 Ничего не найдено по запросу: %s",
		"en": "🤷 Nothing found for query: %s",
	},
	"search_results_title": {
		"ru": "🔍 <b>Результаты поиска</b> по \"%s\":\n\n",
		"en": "🔍 <b>Search results</b> for \"%s\":\n\n",
	},
	"loading_timeline": {
		"ru": "📅 Загружаю ленту...",
		"en": "📅 Loading timeline...",
	},
	"timeline_error": {
		"ru": "❌ Ошибка: %v",
		"en": "❌ Error: %v",
	},
	"timeline_parse_error": {
		"ru": "❌ Ошибка при обработке ленты.",
		"en": "❌ Error processing timeline.",
	},
	"timeline_empty": {
		"ru": "📭 Нет активности в ленте.",
		"en": "📭 No activity in timeline.",
	},
	"timeline_title": {
		"ru": "<b>📅 Последние события:</b>\n\n",
		"en": "<b>📅 Recent events:</b>\n\n",
	},

	// ── Goals ──────────────────────────────────────────────
	"goals_error": {
		"ru": "❌ Ошибка: %v",
		"en": "❌ Error: %v",
	},
	"goals_parse_error": {
		"ru": "❌ Ошибка при обработке целей.",
		"en": "❌ Error processing goals.",
	},
	"goals_empty": {
		"ru": "📭 Нет активных целей.",
		"en": "📭 No active goals.",
	},
	"goals_title": {
		"ru": "<b>🎯 Активные цели:</b>\n\n",
		"en": "<b>🎯 Active goals:</b>\n\n",
	},
	"goal_usage": {
		"ru": "ℹ️ Использование: /goal <id>",
		"en": "ℹ️ Usage: /goal <id>",
	},
	"goal_error": {
		"ru": "❌ Ошибка: %v",
		"en": "❌ Error: %v",
	},
	"goal_parse_error": {
		"ru": "❌ Ошибка при обработке цели.",
		"en": "❌ Error processing goal.",
	},

	// ── Suggest ────────────────────────────────────────────
	"suggest_thinking": {
		"ru": "💡 Думаю...",
		"en": "💡 Thinking...",
	},
	"suggest_error": {
		"ru": "❌ Ошибка: %v",
		"en": "❌ Error: %v",
	},
	"suggest_parse_error": {
		"ru": "❌ Ошибка при обработке предложений.",
		"en": "❌ Error processing suggestions.",
	},
	"suggest_empty": {
		"ru": "💭 Нет предложений на данный момент.",
		"en": "💭 No suggestions at the moment.",
	},
	"suggest_title": {
		"ru": "<b>💡 Предложения:</b>\n\n",
		"en": "<b>💡 Suggestions:</b>\n\n",
	},

	// ── Context ────────────────────────────────────────────
	"context_loading": {
		"ru": "🧠 Получаю контекст...",
		"en": "🧠 Getting context...",
	},
	"context_error": {
		"ru": "❌ Ошибка: %v",
		"en": "❌ Error: %v",
	},
	"context_parse_error": {
		"ru": "❌ Ошибка при обработке контекста.",
		"en": "❌ Error processing context.",
	},
	"context_title": {
		"ru": "<b>🧠 Текущий контекст</b>\n\n",
		"en": "<b>🧠 Current context</b>\n\n",
	},
	"context_goals_title": {
		"ru": "<b>🎯 Активные цели:</b>\n",
		"en": "<b>🎯 Active goals:</b>\n",
	},
	"context_memories_title": {
		"ru": "<b>📚 Релевантные воспоминания:</b>\n\n",
		"en": "<b>📚 Relevant memories:</b>\n\n",
	},
	"context_no_data": {
		"ru": "📭 Нет данных.",
		"en": "📭 No data.",
	},

	// ── Notebook ───────────────────────────────────────────
	"note_saved": {
		"ru": "✅ Запомнил!\n📝 <b>%s</b>\n🏷 %s\n🔑 <code>%s</code>",
		"en": "✅ Saved!\n📝 <b>%s</b>\n🏷 %s\n🔑 <code>%s</code>",
	},
	"note_error": {
		"ru": "❌ Не удалось сохранить заметку.",
		"en": "❌ Failed to save note.",
	},
	"goal_created": {
		"ru": "🎯 <b>Цель создана!</b>\n📌 %s\n🔥 Приоритет: %d/10\n🏷 %s\n🆔 <code>%s</code>",
		"en": "🎯 <b>Goal created!</b>\n📌 %s\n🔥 Priority: %d/10\n🏷 %s\n🆔 <code>%s</code>",
	},
	"goal_create_error": {
		"ru": "❌ Не удалось создать цель.",
		"en": "❌ Failed to create goal.",
	},
	"question_searching": {
		"ru": "🔍 Ищу ответ...",
		"en": "🔍 Searching for answer...",
	},
	"question_error": {
		"ru": "❌ Не удалось выполнить поиск.",
		"en": "❌ Search failed.",
	},
	"question_parse_error": {
		"ru": "❌ Ошибка при обработке результата.",
		"en": "❌ Error processing result.",
	},
	"question_no_results": {
		"ru": "🤷 Не нашёл ничего по этому вопросу.",
		"en": "🤷 Found nothing for this question.",
	},
	"question_knowledge_title": {
		"ru": "📚 <b>Вот что я знаю:</b>\n\n",
		"en": "📚 <b>Here's what I know:</b>\n\n",
	},
	"command_unknown": {
		"ru": "❓ Неизвестная команда: «%s».\n\nИспользуй /goals, /suggest, /context, /timeline, /digest, /search.",
		"en": "❓ Unknown command: \"%s\".\n\nUse /goals, /suggest, /context, /timeline, /digest, /search.",
	},
	"default_message": {
		"ru": "👋 Привет! Напиши что-нибудь — заметку, цель, вопрос.",
		"en": "👋 Hi! Write something — a note, a goal, or a question.",
	},

	// ── Digest ─────────────────────────────────────────────
	"digest_period_day": {
		"ru": "день",
		"en": "day",
	},
	"digest_period_week": {
		"ru": "неделю",
		"en": "week",
	},
	"digest_period_month": {
		"ru": "месяц",
		"en": "month",
	},
	"digest_usage": {
		"ru": "ℹ️ Использование: /digest [day|week|month]",
		"en": "ℹ️ Usage: /digest [day|week|month]",
	},
	"digest_loading": {
		"ru": "📊 Готовлю сводку за %s...",
		"en": "📊 Preparing digest for %s...",
	},
	"digest_error": {
		"ru": "❌ Ошибка: %v",
		"en": "❌ Error: %v",
	},
	"digest_parse_error": {
		"ru": "❌ Ошибка при обработке ленты.",
		"en": "❌ Error processing timeline.",
	},
	"digest_empty": {
		"ru": "📭 Нет событий за этот период.",
		"en": "📭 No events for this period.",
	},
	"digest_title": {
		"ru": "<b>📊 Сводка за %s</b>\n\n",
		"en": "<b>📊 Summary for %s</b>\n\n",
	},
	"digest_notes_count": {
		"ru": "📝 <b>Новых заметок:</b> %d\n",
		"en": "📝 <b>New notes:</b> %d\n",
	},
	"digest_goals_count": {
		"ru": "🎯 <b>Активных целей:</b> %d\n",
		"en": "🎯 <b>Active goals:</b> %d\n",
	},
	"digest_total_events": {
		"ru": "🔍 <b>Всего событий:</b> %d\n\n",
		"en": "🔍 <b>Total events:</b> %d\n\n",
	},
	"digest_recent_title": {
		"ru": "<b>Последние записи:</b>\n",
		"en": "<b>Recent entries:</b>\n",
	},
	"digest_title_prefix": {
		"ru": "Сводка за",
		"en": "Summary for",
	},
	"digest_notes_label": {
		"ru": "Заметки",
		"en": "Notes",
	},
	"digest_goals_label": {
		"ru": "Активных целей:",
		"en": "Active goals:",
	},
	"digest_events_label": {
		"ru": "Всего событий:",
		"en": "Total events:",
	},
	"digest_recent_label": {
		"ru": "Последние записи:",
		"en": "Recent entries:",
	},

	// ── Language command ───────────────────────────────────
	"language_usage": {
		"ru": "ℹ️ Использование: /language <код_языка>\nНапример: /language ru или /language en\n\nТекущий язык: %s\nПоддерживаются: ru, en",
		"en": "ℹ️ Usage: /language <language_code>\nExample: /language en or /language ru\n\nCurrent language: %s\nSupported: ru, en",
	},
	"language_changed": {
		"ru": "✅ Язык изменён на <b>русский</b> 🇷🇺",
		"en": "✅ Language changed to <b>English</b> 🇬🇧",
	},
	"language_bad_code": {
		"ru": "❌ Неподдерживаемый язык. Поддерживаются: ru, en",
		"en": "❌ Unsupported language. Supported: ru, en",
	},

	// ── Access control ─────────────────────────────────────
	"access_denied": {
		"ru": "⛔ Доступ запрещён. Этот бот предназначен только для авторизованных пользователей.",
		"en": "⛔ Access denied. This bot is for authorised users only.",
	},

	// ── Memory command ─────────────────────────────────────
	"memory_usage": {
		"ru": "ℹ️ Использование: /memory <ключ>\nНапример: /memory memory/auto/2026-05-06/932d3dd3",
		"en": "ℹ️ Usage: /memory <key>\nExample: /memory memory/auto/2026-05-06/932d3dd3",
	},
	"memory_error": {
		"ru": "❌ Ошибка: %v",
		"en": "❌ Error: %v",
	},
	"memory_not_found": {
		"ru": "🤷 Запись не найдена.",
		"en": "🤷 Memory not found.",
	},
	"memory_parse_error": {
		"ru": "❌ Ошибка при обработке записи.",
		"en": "❌ Error processing memory.",
	},

	// ── Help ───────────────────────────────────────────────
	"help_title": {
		"ru": "<b>🧠 Memory Bot — Assistant</b>\n\nПросто напишите что-нибудь — бот сам поймёт, заметка это или цель.\n\n<b>Команды:</b>\n",
		"en": "<b>🧠 Memory Bot — Assistant</b>\n\nJust write something — the bot will figure out if it's a note, goal, or question.\n\n<b>Commands:</b>\n",
	},
}

// t returns the translated string for the given key and language.
// Falls back to "ru" if the language or key is missing.
func t(key, lang string) string {
	if dict, ok := i18n[key]; ok {
		if msg, ok := dict[lang]; ok {
			return msg
		}
		// Fall back to Russian
		if msg, ok := dict["ru"]; ok {
			return msg
		}
	}
	return key // last resort: return the key itself
}

// commandDescriptions returns all bot commands with descriptions for a given language.
func commandDescriptions(lang string) []BotCommand {
	// Localised descriptions
	desc := map[string]map[string]string{
		"start": {
			"ru": "Показать приветствие и справку",
			"en": "Show welcome and help",
		},
		"help": {
			"ru": "Показать справку",
			"en": "Show help",
		},
		"search": {
			"ru": "Семантический поиск по памяти",
			"en": "Semantic search across memory",
		},
		"memory": {
			"ru": "Просмотр записи по ключу",
			"en": "View memory entry by key",
		},
		"goals": {
			"ru": "Список активных целей",
			"en": "List active goals",
		},
		"goal": {
			"ru": "Детали цели по ID",
			"en": "Goal details by ID",
		},
		"timeline": {
			"ru": "Лента последних событий",
			"en": "Recent events timeline",
		},
		"suggest": {
			"ru": "Предложения что делать дальше",
			"en": "Suggestions for next steps",
		},
		"context": {
			"ru": "Текущий контекст и память",
			"en": "Current context and memory",
		},
		"digest": {
			"ru": "Сводка за день/неделю/месяц",
			"en": "Daily/weekly/monthly summary",
		},
		"language": {
			"ru": "Сменить язык (ru/en)",
			"en": "Change language (en/ru)",
		},
	}

	commands := []BotCommand{
		{Command: "start", Description: desc["start"][lang]},
		{Command: "help", Description: desc["help"][lang]},
		{Command: "search", Description: desc["search"][lang]},
		{Command: "memory", Description: desc["memory"][lang]},
		{Command: "goals", Description: desc["goals"][lang]},
		{Command: "goal", Description: desc["goal"][lang]},
		{Command: "timeline", Description: desc["timeline"][lang]},
		{Command: "suggest", Description: desc["suggest"][lang]},
		{Command: "context", Description: desc["context"][lang]},
		{Command: "digest", Description: desc["digest"][lang]},
		{Command: "language", Description: desc["language"][lang]},
	}
	return commands
}

// BotCommand represents a Telegram bot command with description.
type BotCommand struct {
	Command     string
	Description string
}
