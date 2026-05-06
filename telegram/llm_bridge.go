// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

// ChatMessage represents a single message in the LLM chat request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMChatMessage is an alias for ChatMessage, used by the agent.
type LLMChatMessage = ChatMessage

// LLMRequester is a function type that sends a system prompt + messages to
// an LLM and returns the generated text response.
// Implemented in main via generateAnswer().
type LLMRequester func(systemPrompt string, messages []ChatMessage) (string, error)
