// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var dbPath, chatModel string
	var limit int
	var output string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Semantic search across memories",
		Long: `Search memories by meaning (not just keywords). Uses Ollama embeddings for vector similarity.

Use -o to control output format: json (default), table, summary.

Examples:
  memory-cli search "important facts"
  memory-cli search "important facts" --limit 5
  memory-cli search "important facts" -o table`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_search", map[string]any{
				"query": args[0],
				"limit": limit,
			})
			if err != nil {
				return err
			}
			format := ParseOutputFormat(output)
			fmt.Println(formatSearch(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of results (max: 50)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}
