// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newContextCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var limit int

	cmd := &cobra.Command{
		Use:   "context <query>",
		Short: "Get aggregated relevant context from memory for the current conversation",
		Long:  "Retrieves aggregated relevant context from persistent memory including facts, decisions, and active goals.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_get_context", map[string]any{
				"query": args[0],
				"limit": limit,
			})
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().IntVar(&limit, "limit", 5, "Maximum number of memory results (max: 20)")

	return cmd
}
