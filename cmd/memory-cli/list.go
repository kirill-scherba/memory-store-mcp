// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var dbPath, chatModel, serverURL, prefix string
	var output string

	cmd := &cobra.Command{
		Use:   "list [prefix]",
		Short: "List keys by prefix (S3-style folder semantics)",
		Long: `List memory keys by prefix.

With no prefix, lists all top-level keys.
With prefix ending in '/', shows contents of that "folder".
Sub-folders are collapsed into single entries.

Use -o to control output format: json (default), table, summary.

Examples:
  memory-cli list
  memory-cli list memory/project/
  memory-cli list memory/
  memory-cli list -o json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				prefix = args[0]
			}

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_list", map[string]any{
				"prefix": prefix,
			})
			if err != nil {
				return err
			}
			format := ParseOutputFormat(output)
			fmt.Println(formatList(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}
