// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLockCmd() *cobra.Command {
	var dbPath, serverURL string

	cmd := &cobra.Command{
		Use:   "lock",
		Short: "Lease-based distributed locks",
		Long: `Acquire, extend, release and inspect lease-based locks stored in memory-store-mcp.

Locks are tied to an owner and expire automatically after a TTL. This means a
crashed owner or a closed MCP pipe will not leave a lock blocked forever.`,
	}

	var resource, owner string
	var ttl float64

	acquireCmd := &cobra.Command{
		Use:   "acquire",
		Short: "Acquire a lock on a resource",
		Example: `  memory-cli lock acquire --resource kirill-scherba/memory-store-mcp --owner orch-1 --ttl 120
  memory-cli lock acquire --resource scenario/discovery --owner worker-a`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if resource == "" || owner == "" {
				return fmt.Errorf("--resource and --owner are required")
			}
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"resource":    resource,
				"owner":       owner,
				"ttl_seconds": ttl,
			}
			result, err := client.callTool("memory_lock_acquire", argsMap)
			if err != nil {
				return fmt.Errorf("memory_lock_acquire call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	acquireCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	acquireCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	acquireCmd.Flags().StringVar(&resource, "resource", "", "Resource identifier to lock")
	acquireCmd.Flags().StringVar(&owner, "owner", "", "Owner identifier")
	acquireCmd.Flags().Float64Var(&ttl, "ttl", 60, "Lease duration in seconds")
	_ = acquireCmd.MarkFlagRequired("resource")
	_ = acquireCmd.MarkFlagRequired("owner")

	releaseCmd := &cobra.Command{
		Use:     "release",
		Short:   "Release a lock held by owner",
		Example: `  memory-cli lock release --resource kirill-scherba/memory-store-mcp --owner orch-1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if resource == "" || owner == "" {
				return fmt.Errorf("--resource and --owner are required")
			}
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"resource": resource,
				"owner":    owner,
			}
			result, err := client.callTool("memory_lock_release", argsMap)
			if err != nil {
				return fmt.Errorf("memory_lock_release call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	releaseCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	releaseCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	releaseCmd.Flags().StringVar(&resource, "resource", "", "Resource identifier of the lock")
	releaseCmd.Flags().StringVar(&owner, "owner", "", "Owner identifier")
	_ = releaseCmd.MarkFlagRequired("resource")
	_ = releaseCmd.MarkFlagRequired("owner")

	extendCmd := &cobra.Command{
		Use:     "extend",
		Short:   "Extend the TTL of a lock held by owner",
		Example: `  memory-cli lock extend --resource kirill-scherba/memory-store-mcp --owner orch-1 --ttl 120`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if resource == "" || owner == "" {
				return fmt.Errorf("--resource and --owner are required")
			}
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"resource":    resource,
				"owner":       owner,
				"ttl_seconds": ttl,
			}
			result, err := client.callTool("memory_lock_extend", argsMap)
			if err != nil {
				return fmt.Errorf("memory_lock_extend call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	extendCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	extendCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	extendCmd.Flags().StringVar(&resource, "resource", "", "Resource identifier of the lock")
	extendCmd.Flags().StringVar(&owner, "owner", "", "Owner identifier")
	extendCmd.Flags().Float64Var(&ttl, "ttl", 60, "New lease duration in seconds")
	_ = extendCmd.MarkFlagRequired("resource")
	_ = extendCmd.MarkFlagRequired("owner")

	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Check the current status of a lock",
		Example: `  memory-cli lock status --resource kirill-scherba/memory-store-mcp`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if resource == "" {
				return fmt.Errorf("--resource is required")
			}
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"resource": resource,
			}
			result, err := client.callTool("memory_lock_status", argsMap)
			if err != nil {
				return fmt.Errorf("memory_lock_status call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	statusCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	statusCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	statusCmd.Flags().StringVar(&resource, "resource", "", "Resource identifier to check")
	_ = statusCmd.MarkFlagRequired("resource")

	cmd.AddCommand(acquireCmd)
	cmd.AddCommand(releaseCmd)
	cmd.AddCommand(extendCmd)
	cmd.AddCommand(statusCmd)
	return cmd
}
