// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newExtractCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var autoSave bool

	cmd := &cobra.Command{
		Use:   "extract <text>",
		Short: "Auto-extract key facts, decisions, and intentions from conversation text",
		Long: `Analyzes text and extracts structured facts using LLM.

With --auto-save, stores extracted knowledge into long-term memory automatically.
The server shows "Thinking..." on stderr while the LLM processes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			rc.proxyStderrWithThinking()

			result, err := rc.callTool("memory_extract", map[string]any{
				"text":      args[0],
				"auto_save": autoSave,
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
	cmd.Flags().BoolVar(&autoSave, "auto-save", false, "Automatically save extracted facts to memory")

	return cmd
}
