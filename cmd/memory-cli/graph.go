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

	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Knowledge graph operations",
		Long:  `Query and manage the knowledge graph: find connections or add new edges.`,
	}

	// graph query subcommand
	var depth, limit int
	queryCmd := &cobra.Command{
		Use:   "query <entity>",
		Short: "Query connections for an entity",
		Example: `  memory-cli graph query Сварня
  memory-cli graph query "Кирилл" --depth 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newMemoryClient("", "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"entity": args[0],
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
	queryCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	queryCmd.Flags().IntVar(&depth, "depth", 2, "Maximum traversal depth")
	queryCmd.Flags().IntVar(&limit, "limit", 500, "Maximum number of edges")

	// graph add subcommand
	var from, to, relation, date string
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add an edge to the knowledge graph",
		Example: `  memory-cli graph add --from Кирилл --to Сварня --relation был_в
  memory-cli graph add --from Сварня --to плесковица --relation заказал --date 2026-07-23`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" || to == "" || relation == "" {
				return fmt.Errorf("--from, --to, and --relation are required")
			}
			client, err := newMemoryClient("", "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"from":     from,
				"to":       to,
				"relation": relation,
			}
			if date != "" {
				argsMap["date"] = date
			}
			result, err := client.callTool("graph_add_edge", argsMap)
			if err != nil {
				return fmt.Errorf("graph_add_edge call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	addCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	addCmd.Flags().StringVar(&from, "from", "", "Source entity")
	addCmd.Flags().StringVar(&to, "to", "", "Target entity")
	addCmd.Flags().StringVar(&relation, "relation", "", "Relation type")
	addCmd.Flags().StringVar(&date, "date", "", "Date (YYYY-MM-DD)")

	// graph get-edges subcommand
	getEdgesCmd := &cobra.Command{
		Use:   "get-edges <entity>",
		Short: "Get all raw edges for an entity",
		Long:  `Returns all edges for an entity as structured JSON (from, to, relation, date). No Prolog inference.`,
		Example: `  memory-cli graph get-edges Сварня`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newMemoryClient("", "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			argsMap := map[string]any{
				"entity": args[0],
			}
			result, err := client.callTool("graph_get_edges", argsMap)
			if err != nil {
				return fmt.Errorf("graph_get_edges call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	getEdgesCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")

	cmd.AddCommand(queryCmd)
	cmd.AddCommand(addCmd)
	cmd.AddCommand(getEdgesCmd)
	return cmd
}
