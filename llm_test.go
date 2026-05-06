package main

import (
	"strings"
	"testing"
)

func TestParseOllamaResponseNonStreaming(t *testing.T) {
	got, err := parseOllamaResponse([]byte(`{"message":{"role":"assistant","content":"  hello  "},"done":true}`))
	if err != nil {
		t.Fatalf("parseOllamaResponse() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("parseOllamaResponse() = %q, want %q", got, "hello")
	}
}

func TestParseOllamaResponseStreaming(t *testing.T) {
	body := strings.Join([]string{
		`{"message":{"role":"assistant","content":"hel"},"done":false}`,
		`{"message":{"role":"assistant","content":"lo"},"done":false}`,
		`{"done":true}`,
	}, "\n")

	got, err := parseOllamaResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseOllamaResponse() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("parseOllamaResponse() = %q, want %q", got, "hello")
	}
}

func TestParseOllamaResponseInvalid(t *testing.T) {
	if _, err := parseOllamaResponse([]byte(`not json`)); err == nil {
		t.Fatal("parseOllamaResponse() error = nil, want error")
	}
}
