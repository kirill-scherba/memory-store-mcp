# MCP-сервер memory-store-mcp

Сделать MCP-сервер memory-store-mcp — постоянную память для ИИ-ассистента Барона на основе keyvalembd (ключ-значение БД с векторным/embedding поиском).

## Суть

Барон (ИИ-ассистент) должен иметь долговременную память, которая переживает сессии. Принцип работы:

- Сохранять факты, наблюдения, знания — с авто-генерацией эмбеддинга.
- Находить релевантные воспоминания по текстовому запросу (семантический поиск).
- Ключи = иерархические пути (как S3): memory/project/..., memory/user/..., memory/technical/...
- Значения — JSON: content, summary, tags, timestamp, source.

## Что нужно сделать

### 1. Использовать keyvalembd для хранения и поиска данных

- Путь: /home/kirill/go/src/github.com/kirill-scherba/keyvalembd
- HTTP Git: github.com/kirill-scherba/keyvalembd.git
- Интерфейс KeyValueStore: Set, Get, Delete, List, Search.
- Иерархические ключи (как S3).

### 2. Сделать MCP-сервер memory-store-mcp

- JSON-RPC 2.0 через stdin/stdout.
- Инструменты: memory_save, memory_get, memory_delete, memory_search, memory_list.

### 3. Документация

- docs/ с CONTEXT.md, DESIGN.md, STATUS.md.
- README.md с примерами.

## Референсы

- keyvalembd
- db-tool-mcp, web-search-mcp — пример MCP-сервера.
- s3lite — интерфейс KeyValueStore.
- sqlh — работа с SQL.

## Язык: Go

## Критерии готовности

1. Пакет keyvalembd используется как Go-библиотека.
2. MCP-сервер запускается и отвечает на запросы.
3. Семантический поиск работает.
4. Документация написана.

## Этапы разработки

Сначала делаем и утверждаем план.
После этого начинаем работать с кодом и документацией.
