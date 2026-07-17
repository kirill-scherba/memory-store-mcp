// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSessionCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string

	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage AI session state (save, get, list, compact)",
		Long: `Manage session state for AI assistants.

Session tools are primarily used by AI agents to save and restore their working
state across conversations. The CLI provides administrative access to these
sessions.

Subcommands:
  session save    <project> <data-json>   Save session state for a project
  session get     <project>               Retrieve latest session state
  session list    [prefix]                List session keys by prefix
  session compact [max_age_hours]         Delete old session snapshots

Examples:
  memory-cli session save memory-store-mcp '{"open_files":["main.go"]}'
  memory-cli session get memory-store-mcp
  memory-cli session list session/project/
  memory-cli session compact 168`,
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")

	cmd.AddCommand(newSessionSaveCmd())
	cmd.AddCommand(newSessionGetCmd())
	cmd.AddCommand(newSessionListCmd())
	cmd.AddCommand(newSessionCompactCmd())

	return cmd
}

// newSessionSaveCmd creates the 'session save' subcommand.
func newSessionSaveCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string

	cmd := &cobra.Command{
		Use:   "save <project> <data-json>",
		Short: "Save session state for a project",
		Long: `Save session state for a project.

Stores a latest copy (for restore) and a timestamped snapshot (for history).
Session data is stored without embedding.

Example:
  memory-cli session save memory-store-mcp '{"open_files":["main.go"],"task":"#35"}'`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := args[0]
			data := args[1]

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("session_save", map[string]any{
				"project": project,
				"data":    data,
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

	return cmd
}

// newSessionGetCmd creates the 'session get' subcommand.
func newSessionGetCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var output string

	cmd := &cobra.Command{
		Use:   "get <project>",
		Short: "Retrieve the latest session state for a project",
		Long: `Retrieve the latest saved session state for a project.

Example:
  memory-cli session get memory-store-mcp`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := args[0]

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("session_get", map[string]any{
				"project": project,
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
	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format: json (default)")

	return cmd
}

// newSessionListCmd creates the 'session list' subcommand.
func newSessionListCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var output string

	cmd := &cobra.Command{
		Use:   "list [prefix]",
		Short: "List session keys by prefix",
		Long: `List session keys by prefix with S3-style folder semantics.

Examples:
  memory-cli session list
  memory-cli session list session/project/memory-store-mcp/`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix := ""
			if len(args) > 0 {
				prefix = args[0]
			}

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("session_list", map[string]any{
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

// newSessionCompactCmd creates the 'session compact' subcommand.
func newSessionCompactCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var maxAgeHours int

	cmd := &cobra.Command{
		Use:   "compact [max_age_hours]",
		Short: "Delete old timestamped session snapshots",
		Long: `Delete timestamped session snapshots older than max_age_hours.
Never deletes */latest keys.

Examples:
  memory-cli session compact
  memory-cli session compact 72`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				_, err := fmt.Sscanf(args[0], "%d", &maxAgeHours)
				if err != nil {
					return fmt.Errorf("invalid max_age_hours: %w", err)
				}
			}

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("session_compact", map[string]any{
				"max_age_hours": maxAgeHours,
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
	cmd.Flags().IntVar(&maxAgeHours, "max-age-hours", 168, "Maximum age in hours (default: 168 = 7 days)")

	return cmd
}
