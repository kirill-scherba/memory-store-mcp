// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var output string

	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Retrieve a memory by its key",
		Long: `Retrieve a stored memory by its key.

Returns the full JSON value including content, summary, tags, and metadata.

Use -o to control output format: json (default), table, summary.

Examples:
  memory-cli get memory/auto/2026-05-05/abc123
  memory-cli get memory/auto/2026-05-05/abc123 -o table
  memory-cli get memory/auto/2026-05-05/abc123 -o summary`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_get", map[string]any{
				"key": args[0],
			})
			if err != nil {
				return err
			}

			format := ParseOutputFormat(output)
			fmt.Println(formatGet(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format: json, table, summary")

	return cmd
}
