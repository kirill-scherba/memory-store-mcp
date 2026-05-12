// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// memoryClient wraps an MCP client (stdio or HTTP) to memory-store-mcp server.
type memoryClient struct {
	cl    *client.Client
	ctx   context.Context
	cfunc context.CancelFunc
}

// newMemoryClient creates and initializes a client connected to memory-store-mcp.
// When serverURL is non-empty, it connects via Streamable HTTP; otherwise via stdio.
func newMemoryClient(dbPath, chatModel, serverURL string) (*memoryClient, error) {
	var (
		c     *client.Client
		ctx   context.Context
		cfunc context.CancelFunc
	)

	if serverURL != "" {
		// ---- HTTP client mode ----
		var err error
		c, err = client.NewStreamableHttpClient(serverURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP MCP client: %w", err)
		}
		ctx, cfunc = context.WithTimeout(context.Background(), 120*time.Second)
		if err := c.Start(ctx); err != nil {
			cfunc()
			c.Close()
			return nil, fmt.Errorf("failed to start HTTP MCP client: %w", err)
		}
	} else {
		// ---- Stdio client mode ----
		mcpPath, err := findMemoryMCP()
		if err != nil {
			return nil, err
		}

		var serverArgs []string
		if dbPath != "" {
			serverArgs = append(serverArgs, "--db", dbPath)
		}
		if chatModel != "" {
			serverArgs = append(serverArgs, "--chat-model", chatModel)
		}

		c, err = client.NewStdioMCPClient(
			mcpPath,
			os.Environ(), // inherit environment for the server process
			serverArgs...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create MCP client: %w", err)
		}

		ctx, cfunc = context.WithTimeout(context.Background(), 120*time.Second)
	}

	// Initialize MCP session (common to both stdio and HTTP modes)
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "memory-cli",
		Version: "0.1.0",
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		cfunc()
		c.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return &memoryClient{cl: c, ctx: ctx, cfunc: cfunc}, nil
}

// close shuts down the client.
func (r *memoryClient) close() {
	r.cfunc()
	r.cl.Close()
}

// callTool calls an MCP tool and returns the result text.
func (r *memoryClient) callTool(name string, args map[string]any) (string, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := r.cl.CallTool(r.ctx, req)
	if err != nil {
		return "", fmt.Errorf("tool %s call failed: %w", name, err)
	}

	var parts []string
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			parts = append(parts, textContent.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// proxyStderr copies the memory-store-mcp server's stderr to the CLI's stderr.
// Runs in background goroutine. Stops automatically when the client closes.
func (r *memoryClient) proxyStderr() {
	stderr, ok := client.GetStderr(r.cl)
	if !ok || stderr == nil {
		return
	}
	// Copy stderr (where memory-store-mcp writes LLM stream tokens) to our stderr
	go func() {
		_, _ = io.Copy(os.Stderr, stderr)
	}()
}

// proxyStderrWithThinking prints "Thinking..." on stderr, then waits for
// the "---LLM---" marker (emitted by the server before the first LLM token).
// When the marker arrives, it clears "Thinking...", skips the marker, and
// copies the rest of stderr (LLM stream tokens) as-is.
// Server log lines before the marker are silently discarded.
// Runs in background goroutine.
func (r *memoryClient) proxyStderrWithThinking() {
	stderr, ok := client.GetStderr(r.cl)
	if !ok || stderr == nil {
		fmt.Fprint(os.Stderr, "Thinking...\n")
		return
	}

	go func() {
		const marker = "---LLM---"

		// Show "Thinking..." without newline
		fmt.Fprint(os.Stderr, "Thinking...")

		// Accumulate incoming bytes; discard them until we find the marker
		var buf []byte
		tmp := make([]byte, 4096)
		for {
			n, err := stderr.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				idx := bytes.Index(buf, []byte(marker))
				if idx >= 0 {
					// Found the marker — clear "Thinking..."
					fmt.Fprintf(os.Stderr, "\r\033[K")
					// Write everything after the marker to stderr
					tail := buf[idx+len(marker):]
					if len(tail) > 0 {
						os.Stderr.Write(tail)
					}
					// Copy the rest of stderr as-is
					_, _ = io.Copy(os.Stderr, stderr)
					return
				}
			}
			if err != nil {
				// stderr closed before marker — just print newline
				fmt.Fprint(os.Stderr, "\n")
				return
			}
		}
	}()
}

// findMemoryMCP locates the memory-store-mcp binary in PATH or next to the memory-cli binary.
func findMemoryMCP() (string, error) {
	// Check same directory as the memory-cli binary
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "memory-store-mcp")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Check PATH
	if p, err := execLookPath("memory-store-mcp"); err == nil {
		return p, nil
	}

	// Check common Go workspace locations
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), "go", "bin", "memory-store-mcp"),
		filepath.Join(os.Getenv("GOPATH"), "bin", "memory-store-mcp"),
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		candidates = append(candidates, filepath.Join(gopath, "bin", "memory-store-mcp"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("memory-store-mcp not found in PATH, same directory, or GOPATH/bin\n" +
		"Build it with: go build ./...")
}

// execLookPath wraps os/exec.LookPath so we don't import os/exec in client.go
// (it's not used elsewhere, but LookPath is necessary for PATH resolution).
var execLookPath = func(file string) (string, error) {
	// Use a direct PATH resolution to avoid importing os/exec in this file
	// while still allowing cross-platform PATH search.
	pathEnv := os.Getenv("PATH")
	dirs := filepath.SplitList(pathEnv)
	for _, dir := range dirs {
		candidate := filepath.Join(dir, file)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate, nil
		}
		// Also try with .exe on Windows (though we're on Linux)
		if ext := filepath.Ext(candidate); ext == "" {
			withExt := candidate + ".exe"
			if fi, err := os.Stat(withExt); err == nil && !fi.IsDir() {
				return withExt, nil
			}
		}
	}
	return "", fmt.Errorf("not found in PATH: %s", file)
}
