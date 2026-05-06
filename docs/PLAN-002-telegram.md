# PLAN-002: Telegram-бот для memory-store-mcp

**Goal**: `goal/2026-05-05/8b70384d` — «Сделать Telegram канал/бота для памяти»

**Status**: Plan accepted, not yet implemented

**Created**: 2026-05-05

---

## Архитектурное решение

Добавляем Telegram-сервер прямо в memory-store-mcp через флаг `--telegram`. Бот получает прямой доступ к storage, LLM и всем 13 инструментам. Никакого отдельного процесса — всё в одном бинарнике.

## Этапы реализации

### Этап 1: Базовая инфраструктура Telegram-бота
- Новый пакет `telegram/` внутри репозитория
- Зависимость: `github.com/go-telegram-bot-api/telegram-bot-api/v5`
- Структура `TelegramBot` с каналами для команд
- Флаг `--telegram` и переменная окружения `TELEGRAM_BOT_TOKEN`
- Стартовый long-polling loop в `main.go`

### Этап 2: Режим «Блокнот» (сохранение в память)
- При получении текстового сообщения → `memory_save` с `auto_key=true`
- Текст сообщения → `text` для эмбеддинга, JSON с content/summary/tags/timestamp → `value`
- Ответ бота: «🧠 Сохранено: [ключ]»

### Этап 3: Режим «Ассистент» (поиск и управление)
- Команды бота:
  - `/search <запрос>` — `memory_search` → топ-5 результатов
  - `/context <запрос>` — `memory_get_context` → агрегированный контекст
  - `/goals` — `memory_goal_list` → список активных целей
  - `/goal <заголовок>` — `memory_goal_create`
  - `/timeline` — `memory_timeline` за сегодня
  - `/suggest` — `memory_suggest` → рекомендации
  - `/help` — список команд

### Этап 4: Режим «Сводки» (автоматическая публикация)
- Периодический ticker (каждые N часов/утром/вечером)
- Сводка дня: сегодняшний timeline + активные цели + suggest'ы
- Формат: красивое Markdown-сообщение в чат/канал
- Конфигурация: `TELEGRAM_CHANNEL_ID`, `TELEGRAM_SUMMARY_INTERVAL`

### Этап 5: Полировка и безопасность
- Белый список пользователей: `TELEGRAM_ALLOWED_USERS`
- Обработка ошибок и retry
- Логирование в базу (timeline events для действий бота)
- README с инструкцией по развёртыванию на VPS

## Файлы, которые будут созданы/изменены

| Файл | Изменение |
|------|-----------|
| `main.go` | Добавить флаг `--telegram`, запуск бота |
| `telegram/bot.go` | Основная структура бота, инициализация, Start |
| `telegram/handlers.go` | Обработчики команд и сообщений |
| `telegram/notebook.go` | Логика сохранения сообщений в память |
| `telegram/assistant.go` | Логика search/context/goals/suggest команд |
| `telegram/digest.go` | Автоматическая генерация сводок |
| `go.mod` | Добавить зависимость `go-telegram-bot-api` |
| `README.md` | Документация по запуску и использованию |

## Ожидаемый результат

Один бинарь `memory-store-mcp`, который:
- Без `--telegram` работает как MCP-сервер (как сейчас)
- С `--telegram` дополнительно запускает Telegram-бота, который:
  - Принимает сообщения → сохраняет в память
  - Отвечает на `/search`, `/goals`, `/suggest` и т.д.
  - Присылает утренние/вечерние сводки