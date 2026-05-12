// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var dbPath, chatModel, serverURL string

	rootCmd := &cobra.Command{
		Use:   "memory-cli",
		Short: "CLI client for memory-store-mcp server",
		Long: `A command-line interface for interacting with the memory-store-mcp server.

memory-cli connects to the memory-store-mcp MCP server via stdio (local binary)
or Streamable HTTP (remote server via --server-url) and allows you to save,
retrieve, search, and manage memories, goals, and context.

Examples:
  memory-cli save memory/test/hello '{"content":"Hello"}' --text "hello world"
  memory-cli get memory/test/hello
  memory-cli search "important facts" --server-url http://localhost:8080/mcp
  memory-cli goals
  memory-cli suggest`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Make global flags available to subcommands via parent
			return nil
		},
	}

	// Global flags (passed through to subcommands via their own --db, --chat-model, --server-url flags)
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	rootCmd.PersistentFlags().StringVar(&chatModel, "chat-model", "", "Chat model")
	rootCmd.PersistentFlags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")

	// Register all subcommands
	rootCmd.AddCommand(newSaveCmd())
	rootCmd.AddCommand(newGetCmd())
	rootCmd.AddCommand(newDeleteCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newContextCmd())
	rootCmd.AddCommand(newExtractCmd())
	rootCmd.AddCommand(newGoalsCmd())
	rootCmd.AddCommand(newTimelineCmd())
	rootCmd.AddCommand(newSuggestCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
