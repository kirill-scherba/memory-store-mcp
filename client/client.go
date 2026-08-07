// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style license.

// Package client is a minimal Go client for memory-store-mcp.
//
// It speaks MCP JSON-RPC over HTTP (StreamableHTTP transport) to the
// memory-store-mcp server and exposes the storage primitives used by
// applications: Save, Get, List, Delete and Search.
//
// The endpoint defaults to http://127.0.0.1:7708/mcp and can be
// overridden with the MEMORY_URL environment variable. This lets any
// host reach a remote memory store — tasks, facts and state are data
// living in the shared memory, not in local files.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultURL is the local memory-store-mcp StreamableHTTP endpoint.
const DefaultURL = "http://127.0.0.1:7708/mcp"

// Client is a minimal MCP client for memory-store-mcp.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a client. The base URL is taken from MEMORY_URL or
// defaults to the local memory-store-mcp endpoint.
func New() *Client {
	u := os.Getenv("MEMORY_URL")
	if u == "" {
		u = DefaultURL
	}
	return &Client{
		baseURL: u,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// jsonrpcRequest is a single JSON-RPC call envelope.
type jsonrpcRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

// jsonrpcResponse is the response envelope.
type jsonrpcResponse struct {
	Result *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call performs a single tools/call against memory-store-mcp and returns
// the text of the first content item.
func (c *Client) call(ctx context.Context, tool string, args map[string]interface{}) (string, error) {
	reqBody := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      tool,
			"arguments": args,
		},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("memory %s: status %d: %s", tool, resp.StatusCode, truncate(string(body), 300))
	}

	var r jsonrpcResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("memory %s: decode: %w", tool, err)
	}
	if r.Error != nil {
		return "", fmt.Errorf("memory %s: %s", tool, r.Error.Message)
	}
	if r.Result == nil || len(r.Result.Content) == 0 {
		return "", nil
	}
	return r.Result.Content[0].Text, nil
}

// Entry is a stored memory as returned by Get.
type Entry struct {
	Content   string `json:"content"`
	Value     string `json:"value,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Save stores a value (JSON string) with a searchable text into memory.
func (c *Client) Save(ctx context.Context, key, value, text string) error {
	_, err := c.call(ctx, "memory_save", map[string]interface{}{
		"key":   key,
		"value": value,
		"text":  text,
	})
	return err
}

// Get returns the memory entry for a key. Returns nil if the key does not exist.
func (c *Client) Get(ctx context.Context, key string) (*Entry, error) {
	text, err := c.call(ctx, "memory_get", map[string]interface{}{"key": key})
	if err != nil {
		return nil, err
	}
	if text == "" || strings.Contains(text, "not found") {
		return nil, nil
	}
	var entry Entry
	if err := json.Unmarshal([]byte(text), &entry); err != nil {
		// The value may be returned as-is in some versions.
		entry.Content = text
	}
	return &entry, nil
}

// List returns all memory keys under a prefix.
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	text, err := c.call(ctx, "memory_list", map[string]interface{}{"prefix": prefix})
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		// Keys are hierarchical (contain '/'); skip headers and empty lines.
		if line == "" || !strings.Contains(line, "/") {
			continue
		}
		keys = append(keys, line)
	}
	return keys, nil
}

// Delete removes a memory key.
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.call(ctx, "memory_delete", map[string]interface{}{"key": key})
	return err
}

// SearchResult is one hit from a semantic memory search.
type SearchResult struct {
	Key     string  `json:"key"`
	Score   float64 `json:"score,omitempty"`
	Content string  `json:"content"`
}

// Search performs a semantic search across memories and returns the hits.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	text, err := c.call(ctx, "memory_search", map[string]interface{}{
		"query": query,
		"limit": limit,
	})
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}
	var results []SearchResult
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		// Fall back to returning the raw text as a single result.
		results = []SearchResult{{Content: text}}
	}
	return results, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
