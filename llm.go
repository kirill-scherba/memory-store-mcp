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
	"net/url"
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

// llmAPIKeyOverride overrides the LLM API key for OpenAI-compatible APIs.
var llmAPIKeyOverride string

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

// setLLMAPIKey sets the LLM API key override for OpenAI-compatible APIs.
func setLLMAPIKey(k string) {
	if k != "" {
		llmAPIKeyOverride = k
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

// OpenAIResponse represents a response from OpenAI-compatible API (/v1/chat/completions).
type OpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Choices []struct {
		Index        int `json:"index"`
		Message      *OllamaChatMessage `json:"message,omitempty"`
		Delta        *OllamaChatMessage `json:"delta,omitempty"`
		FinishReason string             `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// parseLLMResponse parses either Ollama or OpenAI-compatible LLM responses.
func parseLLMResponse(data []byte) (string, error) {
	// 1. Try OpenAI format first (when API key is set)
	var openAIResp OpenAIResponse
	if err := json.Unmarshal(data, &openAIResp); err == nil {
		if openAIResp.Error != nil {
			return "", fmt.Errorf("API error: %s", openAIResp.Error.Message)
		}
		if len(openAIResp.Choices) > 0 {
			msg := openAIResp.Choices[0].Message
			if msg != nil && msg.Content != "" {
				return strings.TrimSpace(msg.Content), nil
			}
			// Streaming delta
			delta := openAIResp.Choices[0].Delta
			if delta != nil && delta.Content != "" {
				return strings.TrimSpace(delta.Content), nil
			}
		}
	}

	// 2. Try Ollama format (non-streaming)
	var singleResp OllamaChatResponse
	if err := json.Unmarshal(data, &singleResp); err == nil {
		if singleResp.Message != nil {
			return strings.TrimSpace(singleResp.Message.Content), nil
		}
	}

	// 3. Try Ollama NDJSON (streaming)
	var answerParts []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var chunk OllamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
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

	return "", fmt.Errorf("failed to parse LLM response (body: %s)", string(data))
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

// llmEndpoint returns the correct API endpoint path.
// Uses /v1/chat/completions for OpenAI-compatible APIs (when API key is set),
// /api/chat for Ollama.
func llmEndpoint() string {
	if llmAPIKeyOverride != "" {
		return "/v1/chat/completions"
	}
	return "/api/chat"
}

// llmFullURL joins baseURL and endpoint, avoiding duplicate path segments.
// For example: "https://api.deepseek.com/v1" + "/v1/chat/completions"
// results in "https://api.deepseek.com/v1/chat/completions" (no double /v1).
func llmFullURL(baseURL, endpoint string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		// Fall back to simple concatenation
		return baseURL + endpoint
	}
	// ResolveReference handles path joining correctly
	ref, err := url.Parse(endpoint)
	if err != nil {
		return baseURL + endpoint
	}
	return u.ResolveReference(ref).String()
}

// LLMChatRequestOpenAI is the request for OpenAI-compatible API (/v1/chat/completions).
type LLMChatRequestOpenAI struct {
	Model    string              `json:"model"`
	Messages []OllamaChatMessage `json:"messages"`
	Stream   *bool               `json:"stream,omitempty"`
}

// generateAnswer sends a non-streaming chat request to Ollama and returns
// the generated answer.
func generateAnswer(messages []OllamaChatMessage) (string, error) {
	baseURL := llmBaseURL()
	endpoint := llmEndpoint()

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

	req, err := http.NewRequest("POST", llmFullURL(baseURL, endpoint), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if llmAPIKeyOverride != "" {
		req.Header.Set("Authorization", "Bearer "+llmAPIKeyOverride)
	}

	resp, err := ollamaClient.Do(req)
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

	return parseLLMResponse(data)
}

// generateAnswerStream sends a streaming chat request and returns
// the full answer. Tokens are written to stderr (like in rag-mcp).
// Supports both Ollama (NDJSON) and OpenAI-compatible (SSE) APIs.
func generateAnswerStream(messages []OllamaChatMessage) (string, error) {
	baseURL := llmBaseURL()
	endpoint := llmEndpoint()
	isOpenAI := llmAPIKeyOverride != ""

	reqBody := LLMChatRequest{
		Model:    chatModel(),
		Messages: messages,
		Stream:   boolPtr(true),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", llmFullURL(baseURL, endpoint), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if llmAPIKeyOverride != "" {
		req.Header.Set("Authorization", "Bearer "+llmAPIKeyOverride)
	}

	resp, err := ollamaClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM returned error %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var answer strings.Builder
	streamStarted := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if isOpenAI {
			// OpenAI SSE format: "data: {...}" or "data: [DONE]"
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				fmt.Fprintf(os.Stderr, "\n")
				break
			}
			var chunk OpenAIResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Error != nil {
				return answer.String(), fmt.Errorf("API error: %s", chunk.Error.Message)
			}
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta != nil && delta.Content != "" {
					if !streamStarted {
						streamStarted = true
						fmt.Fprintf(os.Stderr, "---LLM---")
					}
					fmt.Fprintf(os.Stderr, "%s", delta.Content)
					answer.WriteString(delta.Content)
				}
			}
		} else {
			// Ollama NDJSON format: {"message":{"role":"assistant","content":"..."},"done":false}
			line = strings.TrimSpace(line)
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
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading LLM stream: %w", err)
	}

	return strings.TrimSpace(answer.String()), nil
}