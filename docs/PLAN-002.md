# PLAN-002: Telegram-бот + LLM-агент — доработки и тестирование

## Контекст

После завершения базового функционала memory-store-mcp (PLAN-001) требуется:
- Улучшить систему логирования (уровни verbosity)
- Провести ручное тестирование бота
- Написать автоматические тесты
- Настроить CI

## Code Review — обнаруженные проблемы

| # | Проблема | Файл | Серьёзность |
|---|----------|------|-------------|
| 1 | Нет `--debug` флага — логи пишутся всегда, нет уровней verbosity | `log.go`, `main.go` | Medium |
| 2 | `main.go` — нет флагов `--debug`, `--verbose`, `--log-level` | `main.go:36-44` | Medium |
| 3 | `agent.go` — промпт агента захардкожен в `agentSystemPromptRU` | `agent.go` | Low |
| 4 | `bot.go` — файл большой (~800+ строк), можно выделить `handlers.go` | `bot.go` | Low |
| 5 | Тесты используют mock-функции, но нет интеграционных/e2e | `assistant_test.go` | Low |
| 6 | Отсутствует CI (Makefile / GitHub Actions) | — | Low |

## Задачи

### Задача 1: Добавить `--debug` флаг и уровни логирования
- Добавить `--debug` / `--log-level` в `main.go`
- Модифицировать `BotLogger`: уровни (DEBUG, INFO, WARN, ERROR)
- Пакетно-уровневые хелперы: `Debugf()`, `Infof()`, `Warnf()`
- По умолчанию: INFO+ идут в stderr, DEBUG — только в файл при `--debug`

**Файлы:** `telegram/log.go`, `main.go`

### Задача 2: Ручное тестирование бота / LLM
- Запустить бота с тестовым токеном
- Проверить все команды агента в реальном чате
- Проверить graceful degradation (без Ollama)

### Задача 3: Автоматические тесты Telegram-бота
- Интеграционные тесты для `handleTextWithAgent` с mock-LLM
- Тесты для `handleCallback`
- Тесты для `notebook.go`

**Файлы:** `telegram/assistant_test.go` (расширить), новый `telegram/bot_test.go`

### Задача 4: Семантические тесты памяти (e2e)
- `memory_save` → `memory_search` (сквозной тест)
- `memory_goal_create` → `memory_goal_list` → `memory_goal_update`
- `memory_extract` с реальным LLM

**Файлы:** новый `e2e_test.go` в корне или `telegram/`

### Задача 5: CI и документация
- `Makefile` с целями: `test`, `lint`, `build`
- Обновить `docs/STATUS.md`

## Порядок выполнения

```
Задача 1 (--debug) → Задача 2 (ручное тестирование)
                   → Задача 3 (тесты бота)
                   → Задача 4 (e2e тесты)
                   → Задача 5 (CI/docs)
```

Задачи 3 и 4 можно делать параллельно.

## Статус

- [ ] Задача 1: `--debug` флаг
- [ ] Задача 2: Ручное тестирование
- [ ] Задача 3: Авто-тесты бота
- [ ] Задача 4: Семантические e2e тесты
- [ ] Задача 5: CI/documentation