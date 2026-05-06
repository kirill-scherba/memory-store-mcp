// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newTimelineCmd() *cobra.Command {
	var dbPath, chatModel string
	var from, to string
	var limit int
	var output string

	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Get timeline of memory events for a date range",
		Long: `Get timeline of memory events. Shows what happened and when.

By default shows events from today.

Use -o to control output format: json (default), table, summary.

Examples:
  memory-cli timeline
  memory-cli timeline --from 2026-05-01 --to 2026-05-05
  memory-cli timeline --limit 50
  memory-cli timeline -o table`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_timeline", map[string]any{
				"from":  from,
				"to":    to,
				"limit": limit,
			})
			if err != nil {
				return err
			}
			format := ParseOutputFormat(output)
			fmt.Println(formatTimeline(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO 8601 or YYYY-MM-DD). Empty for beginning.")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO 8601 or YYYY-MM-DD). Empty for now.")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of entries (max: 100)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}
