package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractSystemPrompt(t *testing.T) {
	prompt := extractSystemPrompt()
	if prompt == "" {
		t.Fatal("extractSystemPrompt() returned empty string")
	}
	if !strings.Contains(prompt, "fact extraction") {
		t.Fatal("extractSystemPrompt() missing expected content")
	}
	if !strings.Contains(prompt, "JSON object") {
		t.Fatal("extractSystemPrompt() missing JSON mention")
	}
	if !strings.Contains(prompt, "triples") {
		t.Fatal("extractSystemPrompt() missing graph triples instruction")
	}
}

// TestSanitizeLLMJSON covers the artefacts small models emit: Markdown code
// fences and trailing commas, both of which make the response invalid JSON.
func TestSanitizeLLMJSON(t *testing.T) {
	const raw = "```json\n{\n  \"facts\": [\n    {\"content\": \"x\"},\n  ],\n  \"triples\": []\n}\n```"
	out := sanitizeLLMJSON(raw)
	if strings.Contains(out, "```") {
		t.Fatalf("code fence not stripped: %q", out)
	}
	var res ExtractResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("sanitized JSON is still invalid: %v\n%s", err, out)
	}
	if len(res.Facts) != 1 {
		t.Fatalf("facts = %+v, want 1", res.Facts)
	}
}

func TestSuggestSystemPromptEn(t *testing.T) {
	prompt := suggestSystemPrompt("en")
	if prompt == "" {
		t.Fatal("suggestSystemPrompt(en) returned empty")
	}
	if !strings.Contains(prompt, "reminder") {
		t.Fatal("suggestSystemPrompt(en) missing expected content")
	}
	if strings.Contains(prompt, "RUSSIAN") {
		t.Fatal("suggestSystemPrompt(en) contains the Russian-language rule")
	}
}

func TestSuggestSystemPromptRu(t *testing.T) {
	prompt := suggestSystemPrompt("ru")
	if !strings.Contains(prompt, "RUSSIAN") {
		t.Fatal("suggestSystemPrompt(ru) missing the language rule")
	}
}

// TestDetectLang: the goals are Russian, and hardcoding "en" used to produce
// English suggestions for them.
func TestDetectLang(t *testing.T) {
	if got := detectLang("Завершить пазл", "Срочно получить миллион"); got != "ru" {
		t.Fatalf("detectLang(ru) = %q, want ru", got)
	}
	if got := detectLang("Finalize the dream weaver", "Investor pitch"); got != "en" {
		t.Fatalf("detectLang(en) = %q, want en", got)
	}
}

func TestSuggestPrompt(t *testing.T) {
	short := SuggestPrompt("hello")
	if short != "hello" {
		t.Fatalf("SuggestPrompt(short) = %q, want hello", short)
	}

	long := strings.Repeat("a", 9000)
	truncated := SuggestPrompt(long)
	if len(truncated) >= 9000 {
		t.Fatal("SuggestPrompt(long) not truncated")
	}
	if !strings.HasSuffix(truncated, "...") {
		t.Fatal("SuggestPrompt(long) missing ... suffix")
	}
}

func TestProcessConversationEmpty(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	err := store.ProcessConversation("", "")
	if err != nil {
		t.Fatalf("ProcessConversation(empty) error = %v, want nil", err)
	}
}

func TestGetContextForInjectionEmpty(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ctx, err := store.GetContextForInjection("test query", 5)
	if err != nil {
		t.Fatalf("GetContextForInjection() error = %v", err)
	}
	if ctx != "" {
		t.Fatalf("GetContextForInjection(empty) = %q, want empty", ctx)
	}
}

// TestSanitizeLLMJSONBrokenQuote uses the exact malformed response phi4-mini
// produced for memory_suggest: the closing quote after the title disappeared
// and the comma slipped inside the string.
func TestSanitizeLLMJSONBrokenQuote(t *testing.T) {
	const raw = "```json\n" + `[
  {"type":"followup","title":"Обеспечить безопасность js-yaml","description":"Обновите js-yaml.","priority":9},
  {"type":"followup","title":"Запланировать сны,"description":"Запишите сон.","priority":9}
]` + "\n```"

	out := sanitizeLLMJSON(raw)
	if strings.Contains(out, "```") {
		t.Fatalf("fence not stripped:\n%s", out)
	}

	var suggestions []Suggestion
	if err := json.Unmarshal([]byte(out), &suggestions); err != nil {
		t.Fatalf("still invalid after sanitizing: %v\n%s", err, out)
	}
	if len(suggestions) != 2 {
		t.Fatalf("got %d suggestions, want 2", len(suggestions))
	}
	if suggestions[1].Title != "Запланировать сны" {
		t.Fatalf("title = %q, want %q", suggestions[1].Title, "Запланировать сны")
	}
	if suggestions[0].Title != "Обеспечить безопасность js-yaml" {
		t.Fatalf("first title = %q", suggestions[0].Title)
	}
}

// TestSanitizeLLMJSONKeepsValidJSON: the repair patterns must not touch
// well-formed JSON.
func TestSanitizeLLMJSONKeepsValidJSON(t *testing.T) {
	const valid = `[{"type":"insight","title":"ok","description":"fine","priority":5}]`
	if out := sanitizeLLMJSON(valid); out != valid {
		t.Fatalf("valid JSON was modified:\n in: %s\nout: %s", valid, out)
	}
}
