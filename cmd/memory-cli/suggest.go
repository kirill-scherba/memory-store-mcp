// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSuggestCmd() *cobra.Command {
	var dbPath, chatModel string
	var limit int
	var output string

	cmd := &cobra.Command{
		Use:   "suggest [context]",
		Short: "Get proactive suggestions based on goals, history, and current context",
		Long: `Analyzes active goals + timeline + recent memories to recommend next actions.

Useful when you are stuck and need ideas for what to do next.

Use -o to control output format: json (default), table, summary.

Examples:
  memory-cli suggest
  memory-cli suggest "working on memory-cli build"
  memory-cli suggest -o table`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := ""
			if len(args) > 0 {
				ctx = args[0]
			}

			rc, err := newMemoryClient(dbPath, chatModel)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_suggest", map[string]any{
				"context": ctx,
				"limit":   limit,
			})
			if err != nil {
				return err
			}
			format := ParseOutputFormat(output)
			fmt.Println(formatSuggest(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().IntVar(&limit, "limit", 5, "Maximum number of suggestions (max: 10)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}
