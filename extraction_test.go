package main

import (
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
	if !strings.Contains(prompt, "JSON array") {
		t.Fatal("extractSystemPrompt() missing JSON mention")
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
