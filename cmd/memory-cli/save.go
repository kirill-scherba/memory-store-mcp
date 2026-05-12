// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSaveCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var autoKey bool

	cmd := &cobra.Command{
		Use:   "save <key> <value> --text <text>",
		Short: "Save a memory with embedding for semantic search",
		Long: `Save a memory with optional auto-key generation.

Provide either --key or --auto-key.
If --auto-key is set, the key argument is optional.

Examples:
  memory-cli save memory/test/hello '{"content":"Hello world"}' --text "world"
  memory-cli save --auto-key '{"content":"Auto generated"}' --text "auto text"`,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var key, value string

			if autoKey {
				if len(args) > 0 {
					value = args[0]
				} else {
					value = `{"content":"auto saved memory"}`
				}
			} else {
				if len(args) < 2 {
					return fmt.Errorf("requires key and value arguments (or use --auto-key)")
				}
				key = args[0]
				value = args[1]
			}

			text, _ := cmd.Flags().GetString("text")
			if text == "" && !autoKey {
				return fmt.Errorf("--text is required for embedding")
			}

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			argsMap := map[string]any{
				"key":   key,
				"value": value,
				"text":  text,
			}
			if autoKey {
				argsMap["auto_key"] = true
			}

			result, err := rc.callTool("memory_save", argsMap)
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
	cmd.Flags().String("text", "", "Text to generate embedding for semantic search")
	cmd.Flags().BoolVar(&autoKey, "auto-key", false, "Auto-generate key")

	return cmd
}
