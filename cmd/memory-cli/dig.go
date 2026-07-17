// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDigCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var keywords []string
	var window string
	var max int
	var output string

	cmd := &cobra.Command{
		Use:   "dig <query>",
		Short: "Deep contextual search across memories (scenes with time windows)",
		Long: `Deep contextual search across memories.

Finds entries matching the query, builds scenes with context windows (entries
before and after each match), and optionally intersects with additional keywords
for relevance ranking.

Use this when you need to understand the full picture around a memory event:
- "what happened when we ate khinkali" — shows scenes around khinkali events
- "when I removed the car, what did we discuss" — query + keyword for context

Examples:
  memory-cli dig "what happened when we ate khinkali"
  memory-cli dig "car wash" --keywords "khinkali,plate" --window "1d" --max 10`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_dig", map[string]any{
				"query":    args[0],
				"keywords": keywords,
				"window":   window,
				"max":      max,
			})
			if err != nil {
				return err
			}
			format := ParseOutputFormat(output)
			fmt.Println(formatDig(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().StringSliceVar(&keywords, "keywords", nil, "Additional keywords for relevance ranking (comma-separated)")
	cmd.Flags().StringVar(&window, "window", "2h", "Context window duration: '2h', '30m', '1d'")
	cmd.Flags().IntVar(&max, "max", 10, "Maximum number of scenes to return (max: 50)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}
