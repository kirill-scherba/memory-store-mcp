// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGraphCmd() *cobra.Command {
	var serverURL string
	var depth, limit int

	cmd := &cobra.Command{
		Use:   "graph <entity>",
		Short: "Query the knowledge graph for connections",
		Long: `Query the knowledge graph. Finds all entities connected to the given entity
via graph edges using Prolog inference through the prolog-mcp server.

Examples:
  memory-cli graph Сварня
  memory-cli graph "Кирилл" --depth 3
  memory-cli graph "img_1784222682.png" --limit 20
`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entity := args[0]

			client, err := newMemoryClient("", "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"entity": entity,
				"depth":  float64(depth),
				"limit":  float64(limit),
			}

			result, err := client.callTool("graph_query", argsMap)
			if err != nil {
				return fmt.Errorf("graph_query call: %w", err)
			}

			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().IntVar(&depth, "depth", 2, "Maximum traversal depth")
	cmd.Flags().IntVar(&limit, "limit", 500, "Maximum number of edges to consider")

	return cmd
}
