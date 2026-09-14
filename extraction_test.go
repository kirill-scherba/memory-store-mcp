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

// TestSanitizeLLMJSONUnescapedQuotes uses the exact response phi4-mini produced
// next: quotes inside a string without escaping.
func TestSanitizeLLMJSONUnescapedQuotes(t *testing.T) {
	const raw = `[
    {"type":"insight","title":"Согласование","description":"Обсудите внедрение "smart search" в платформу.","priority":8}
]`

	out := sanitizeLLMJSON(raw)
	var suggestions []Suggestion
	if err := json.Unmarshal([]byte(out), &suggestions); err != nil {
		t.Fatalf("still invalid after sanitizing: %v\n%s", err, out)
	}
	if len(suggestions) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(suggestions))
	}
	want := `Обсудите внедрение "smart search" в платформу.`
	if suggestions[0].Description != want {
		t.Fatalf("description = %q, want %q", suggestions[0].Description, want)
	}
}

// TestRepairJSONStringsRecoversOverEscapedOutput uses the exact triples block
// deepseek-v4-flash produced: one slipped backslash before a closing quote, and
// every following quote escaped as a consequence. Patching only the first
// defect moves the parse error further down; the scanner has to see the string
// state of the whole document.
func TestRepairJSONStringsRecoversOverEscapedOutput(t *testing.T) {
	const raw = `{
  "facts": [{"content":"Кирилл показал тоннель","summary":"tunnel","tags":["метро"]}],
  "triples": [
    {
      "from": "Барон",
      "relation": "был_в",
      "to": "метро",
      "date": "2026-08-14\"
    },
    {
      \"from\": \"Кирилл\",
      \"relation\": \"был_в\",
      \"to\": \"метро\",
      \"date\": \"2026-08-14\"
    }
  ]
}`

	out := sanitizeLLMJSON(raw)
	var res ExtractResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("still invalid after sanitizing: %v\n%s", err, out)
	}
	if len(res.Triples) != 2 {
		t.Fatalf("got %d triples, want 2\n%s", len(res.Triples), out)
	}
	if res.Triples[0].From != "Барон" || res.Triples[0].Relation != "был_в" || res.Triples[0].Date != "2026-08-14" {
		t.Fatalf("first triple = %+v", res.Triples[0])
	}
	if res.Triples[1].From != "Кирилл" || res.Triples[1].To != "метро" {
		t.Fatalf("second triple = %+v", res.Triples[1])
	}
}

// TestRepairJSONStringsKeepsEscapedQuotes is the counterweight: a quote that is
// genuinely escaped inside a value must survive the scanner untouched.
func TestRepairJSONStringsKeepsEscapedQuotes(t *testing.T) {
	const raw = `{"content":"Он сказал \"привет\" и ушёл","summary":"s"}`
	out := sanitizeLLMJSON(raw)

	var v map[string]string
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("escaped quotes were broken: %v\n%s", err, out)
	}
	if v["content"] != `Он сказал "привет" и ушёл` {
		t.Fatalf("content = %q, want %q", v["content"], `Он сказал "привет" и ушёл`)
	}
}
