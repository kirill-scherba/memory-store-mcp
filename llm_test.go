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
