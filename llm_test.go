package main

import (
	"strings"
	"testing"
)

func TestParseLLMResponseNonStreaming(t *testing.T) {
	got, err := parseLLMResponse([]byte(`{"message":{"role":"assistant","content":"  hello  "},"done":true}`))
	if err != nil {
		t.Fatalf("parseLLMResponse() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("parseLLMResponse() = %q, want %q", got, "hello")
	}
}

func TestParseLLMResponseStreaming(t *testing.T) {
	body := strings.Join([]string{
		`{"message":{"role":"assistant","content":"hel"},"done":false}`,
		`{"message":{"role":"assistant","content":"lo"},"done":false}`,
		`{"done":true}`,
	}, "\n")

	got, err := parseLLMResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseLLMResponse() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("parseLLMResponse() = %q, want %q", got, "hello")
	}
}

func TestParseLLMResponseInvalid(t *testing.T) {
	if _, err := parseLLMResponse([]byte(`not json`)); err == nil {
		t.Fatal("parseLLMResponse() error = nil, want error")
	}
}

func TestParseOpenAIResponse(t *testing.T) {
	resp := `{"choices":[{"message":{"role":"assistant","content":"OpenAI result"}}]}`
	got, err := parseLLMResponse([]byte(resp))
	if err != nil {
		t.Fatalf("parseLLMResponse(OpenAI) error = %v", err)
	}
	if got != "OpenAI result" {
		t.Fatalf("parseLLMResponse(OpenAI) = %q, want OpenAI result", got)
	}
}

func TestParseOpenAIError(t *testing.T) {
	resp := `{"error":{"message":"rate limit exceeded","type":"rate_limit"}}`
	_, err := parseLLMResponse([]byte(resp))
	if err == nil {
		t.Fatal("parseLLMResponse(OpenAI error) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("parseLLMResponse(OpenAI error) = %q, want rate limit exceeded", err)
	}
}

func TestParseOpenAIStreamingDelta(t *testing.T) {
	resp := `{"choices":[{"delta":{"role":"assistant","content":"delta content"}}]}`
	got, err := parseLLMResponse([]byte(resp))
	if err != nil {
		t.Fatalf("parseLLMResponse(OpenAI delta) error = %v", err)
	}
	if got != "delta content" {
		t.Fatalf("parseLLMResponse(OpenAI delta) = %q, want delta content", got)
	}
}

func TestChatModel(t *testing.T) {
	// Reset override
	chatModelOverride = ""
	if got := chatModel(); got != defaultLLMModel {
		t.Fatalf("chatModel() = %q, want %q", got, defaultLLMModel)
	}

	setChatModel("custom-model")
	if got := chatModel(); got != "custom-model" {
		t.Fatalf("chatModel() = %q, want custom-model", got)
	}

	// Reset for other tests
	chatModelOverride = ""
}

func TestExtractModel(t *testing.T) {
	chatModelOverride = ""
	extractModelOverride = ""

	// Default falls back to chat model.
	if got := extractModel(); got != defaultLLMModel {
		t.Fatalf("extractModel() = %q, want %q", got, defaultLLMModel)
	}

	// Override extraction model independently of chat model.
	setExtractModel("extract-only")
	if got := extractModel(); got != "extract-only" {
		t.Fatalf("extractModel() = %q, want extract-only", got)
	}

	// If chat model is also overridden, extraction override wins.
	setChatModel("chat-only")
	if got := extractModel(); got != "extract-only" {
		t.Fatalf("extractModel() = %q, want extract-only", got)
	}

	// Reset for other tests
	chatModelOverride = ""
	extractModelOverride = ""
}

func TestLlmBaseURL(t *testing.T) {
	llmURLOverride = ""
	if got := llmBaseURL(); got != ollamaBaseURL {
		t.Fatalf("llmBaseURL() = %q, want %q", got, ollamaBaseURL)
	}

	setLLMURL("http://custom:11434")
	if got := llmBaseURL(); got != "http://custom:11434" {
		t.Fatalf("llmBaseURL() = %q, want http://custom:11434", got)
	}

	llmURLOverride = ""
}

func TestLlmEndpoint(t *testing.T) {
	llmAPIKeyOverride = ""
	if got := llmEndpoint(); got != "/api/chat" {
		t.Fatalf("llmEndpoint(no key) = %q, want /api/chat", got)
	}

	setLLMAPIKey("test-key")
	if got := llmEndpoint(); got != "/v1/chat/completions" {
		t.Fatalf("llmEndpoint(with key) = %q, want /v1/chat/completions", got)
	}

	llmAPIKeyOverride = ""
}

func TestLlmFullURL(t *testing.T) {
	tests := []struct {
		base     string
		endpoint string
		want     string
	}{
		{"http://localhost:11434", "/api/chat", "http://localhost:11434/api/chat"},
		{"https://api.openai.com/v1", "/v1/chat/completions", "https://api.openai.com/v1/chat/completions"},
		{"http://host", "/api", "http://host/api"},
	}

	for _, tt := range tests {
		got := llmFullURL(tt.base, tt.endpoint)
		if got != tt.want {
			t.Fatalf("llmFullURL(%q, %q) = %q, want %q", tt.base, tt.endpoint, got, tt.want)
		}
	}
}
