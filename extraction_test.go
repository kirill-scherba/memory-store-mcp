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
	if strings.Contains(prompt, "ВАЖНОЕ ПРАВИЛО") {
		t.Fatal("suggestSystemPrompt(en) contains Russian text")
	}
}

func TestSuggestSystemPromptRu(t *testing.T) {
	prompt := suggestSystemPrompt("ru")
	if !strings.Contains(prompt, "ВАЖНОЕ ПРАВИЛО") {
		t.Fatal("suggestSystemPrompt(ru) missing Russian rule text")
	}
	if !strings.Contains(prompt, "RUSSKOM") && !strings.Contains(prompt, "русском") {
		t.Log("suggestSystemPrompt(ru) may not contain expected Russian text")
	}
}

func TestSuggestPrompt(t *testing.T) {
	short := SuggestPrompt("hello")
	if short != "hello" {
		t.Fatalf("SuggestPrompt(short) = %q, want hello", short)
	}

	long := strings.Repeat("a", 5000)
	truncated := SuggestPrompt(long)
	if len(truncated) >= 5000 {
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
