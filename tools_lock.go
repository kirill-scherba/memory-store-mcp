// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// durationParam converts a tool argument to a time.Duration. It accepts a
// number of seconds (float64) or a Go duration string such as "30s".
func durationParam(args map[string]any, name string, defaultDur time.Duration) (time.Duration, error) {
	if v, ok := args[name].(float64); ok {
		return time.Duration(v) * time.Second, nil
	}
	if v, ok := args[name].(string); ok && v != "" {
		return time.ParseDuration(v)
	}
	if defaultDur > 0 {
		return defaultDur, nil
	}
	return 0, fmt.Errorf("%s is required", name)
}

// lockResultJSON serialises a lock record for tool output.
func lockResultJSON(acquired bool, rec *LockRecord, err error) string {
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	out := map[string]any{"acquired": acquired}
	if rec != nil {
		out["owner"] = rec.Owner
		out["expires_at"] = rec.ExpiresAt
		out["acquired_at"] = rec.AcquiredAt
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data)
}

// memoryLockAcquireTool acquires a lease-based lock on a resource.
func memoryLockAcquireTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_lock_acquire",
		mcp.WithDescription(`Acquire a lease-based lock on a resource.

The lock is tied to an owner and has a TTL. If the owner crashes, the MCP
server terminates, or the pipe closes, the lock expires automatically after
the TTL and another owner can acquire it. Re-acquiring by the same owner is
allowed and resets the TTL.`),
		mcp.WithString("resource",
			mcp.Description("Resource identifier to lock (e.g. repository path or scenario name)"),
			mcp.Required(),
		),
		mcp.WithString("owner",
			mcp.Description("Unique owner identifier (e.g. process id or session name)"),
			mcp.Required(),
		),
		mcp.WithNumber("ttl_seconds",
			mcp.Description("Lease duration in seconds (default: 60)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			resource, _ := args["resource"].(string)
			owner, _ := args["owner"].(string)
			ttl, err := durationParam(args, "ttl_seconds", 60*time.Second)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}

			acquired, rec, err := s.LockAcquire(resource, owner, ttl)
			return mcp.NewToolResultText(lockResultJSON(acquired, rec, err)), nil
		},
	}
}

// memoryLockReleaseTool releases a lease-based lock held by owner.
func memoryLockReleaseTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_lock_release",
		mcp.WithDescription("Release a lease-based lock. Only the current owner can release it."),
		mcp.WithString("resource",
			mcp.Description("Resource identifier of the lock"),
			mcp.Required(),
		),
		mcp.WithString("owner",
			mcp.Description("Owner identifier that currently holds the lock"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			resource, _ := args["resource"].(string)
			owner, _ := args["owner"].(string)

			released, err := s.LockRelease(resource, owner)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			out := map[string]bool{"released": released}
			data, _ := json.MarshalIndent(out, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	}
}

// memoryLockExtendTool extends the TTL of an existing lock held by owner.
func memoryLockExtendTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_lock_extend",
		mcp.WithDescription(`Extend the TTL of a lease-based lock.
Only the current owner can extend it. The new TTL is added from the current
moment, not from the previous expiry.`),
		mcp.WithString("resource",
			mcp.Description("Resource identifier of the lock"),
			mcp.Required(),
		),
		mcp.WithString("owner",
			mcp.Description("Owner identifier that currently holds the lock"),
			mcp.Required(),
		),
		mcp.WithNumber("ttl_seconds",
			mcp.Description("New lease duration in seconds (default: 60)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			resource, _ := args["resource"].(string)
			owner, _ := args["owner"].(string)
			ttl, err := durationParam(args, "ttl_seconds", 60*time.Second)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}

			extended, rec, err := s.LockExtend(resource, owner, ttl)
			return mcp.NewToolResultText(lockResultJSON(extended, rec, err)), nil
		},
	}
}

// memoryLockStatusTool returns the current lock status for a resource.
func memoryLockStatusTool(s *Storage) server.ServerTool {
	opt := mcp.NewTool("memory_lock_status",
		mcp.WithDescription("Check whether a resource is currently locked, and if so by which owner and until when. Expired locks are reported as free."),
		mcp.WithString("resource",
			mcp.Description("Resource identifier to check"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			resource, _ := args["resource"].(string)

			rec, err := s.LockStatus(resource)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			out := map[string]any{"locked": rec != nil}
			if rec != nil {
				out["owner"] = rec.Owner
				out["expires_at"] = rec.ExpiresAt
				out["acquired_at"] = rec.AcquiredAt
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	}
}
