// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Default LLM model and Ollama settings for extraction and suggestion.
const (
	defaultLLMModel = "qwen2.5-coder:7b"
	ollamaBaseURL   = "http://localhost:11434"
	generateTimeout = 120 * time.Second
)

// ollamaClient is a reusable HTTP client with keep-alive transport.
var ollamaClient = &http.Client{
	Timeout: generateTimeout,
	Transport: &http.Transport{
		MaxIdleConns:    5,
		IdleConnTimeout: 90 * time.Second,
	},
}

// chatModelOverride overrides the chat model (for extraction/suggest).
var chatModelOverride string

// llmURLOverride overrides the LLM API base URL when set via --llm-url flag.
var llmURLOverride string

// setLLMURL sets the LLM base URL override.
func setLLMURL(u string) {
	if u != "" {
		llmURLOverride = u
	}
}

// setChatModel sets the chat model override.
func setChatModel(m string) {
	if m != "" {
		chatModelOverride = m
	}
}

// OllamaChatMessage represents a message in the chat API.
type OllamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMChatRequest is the request to Ollama /api/chat.
type LLMChatRequest struct {
	Model    string              `json:"model"`
	Messages []OllamaChatMessage `json:"messages"`
	Stream   *bool               `json:"stream,omitempty"`
}

// OllamaChatResponse is the response from Ollama /api/chat.
type OllamaChatResponse struct {
	Message *OllamaChatMessage `json:"message,omitempty"`
	Done    bool               `json:"done"`
}

// boolPtr returns a pointer to the given boolean value.
func boolPtr(b bool) *bool { return &b }

// parseOllamaResponse handles both streaming (NDJSON) and non-streaming JSON
// responses from the Ollama /api/chat endpoint.
func parseOllamaResponse(data []byte) (string, error) {
	// Try parsing as single JSON object first (non-streaming response)
	var singleResp OllamaChatResponse
	if err := json.Unmarshal(data, &singleResp); err == nil {
		if singleResp.Message != nil {
			return strings.TrimSpace(singleResp.Message.Content), nil
		}
	}

	// Fallback: parse as NDJSON (streaming response with one JSON object per line)
	var answerParts []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var chunk OllamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue // skip malformed lines
		}
		if chunk.Message != nil {
			answerParts = append(answerParts, chunk.Message.Content)
		}
		if chunk.Done {
			break
		}
	}
	if len(answerParts) > 0 {
		answer := strings.Join(answerParts, "")
		return strings.TrimSpace(answer), nil
	}

	return "", fmt.Errorf("failed to parse Ollama response (body: %s)", string(data))
}

// chatModel returns the effective chat model to use, selecting from
// override -> default.
func chatModel() string {
	if m := chatModelOverride; m != "" {
		return m
	}
	return defaultLLMModel
}

// llmBaseURL returns the effective LLM base URL, checking override first,
// then default.
func llmBaseURL() string {
	if u := llmURLOverride; u != "" {
		return u
	}
	return ollamaBaseURL
}

// generateAnswer sends a non-streaming chat request to Ollama and returns
// the generated answer.
func generateAnswer(messages []OllamaChatMessage) (string, error) {
	baseURL := llmBaseURL()

	// Non-streaming request
	reqBody := LLMChatRequest{
		Model:    chatModel(),
		Messages: messages,
		Stream:   boolPtr(false),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := ollamaClient.Post(baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("Ollama chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned error %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Ollama response: %w", err)
	}

	return parseOllamaResponse(data)
}

// generateAnswerStream sends a streaming chat request to Ollama and returns
// the full answer. Tokens are written to stderr (like in rag-mcp).
func generateAnswerStream(messages []OllamaChatMessage) (string, error) {
	baseURL := llmBaseURL()

	reqBody := LLMChatRequest{
		Model:    chatModel(),
		Messages: messages,
		Stream:   boolPtr(true),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := ollamaClient.Post(baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("Ollama chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned error %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var answer strings.Builder
	streamStarted := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chunk OllamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		if chunk.Message != nil {
			token := chunk.Message.Content
			if !streamStarted {
				streamStarted = true
				fmt.Fprintf(os.Stderr, "---LLM---")
			}
			fmt.Fprintf(os.Stderr, "%s", token)
			answer.WriteString(token)
		}

		if chunk.Done {
			fmt.Fprintf(os.Stderr, "\n")
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading Ollama stream: %w", err)
	}

	return strings.TrimSpace(answer.String()), nil
}