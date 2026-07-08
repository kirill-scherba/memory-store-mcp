// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newFindCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var limit int
	var output string

	cmd := &cobra.Command{
		Use:   "find <keyword>",
		Short: "Keyword search across memories (exact match)",
		Long: `Search memories by exact keyword match (SQL LIKE) in both keys and values.
Use this when you know the exact word or phrase — complements the semantic 'search' command.

Supports Russian and other UTF-8 text. Note: SQLite LIKE is case-sensitive for non-ASCII characters,
so try both "сварня" and "Сварня" if needed.

Examples:
  memory-cli find сварня
  memory-cli find "Шашлычная 1957"
  memory-cli find "issue #5" --limit 10
  memory-cli find Тоша -o table`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_find", map[string]any{
				"keyword": args[0],
				"limit":   limit,
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
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of results (max: 100)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}
