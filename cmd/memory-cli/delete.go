// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var dbPath, chatModel string

	cmd := &cobra.Command{
		Use:   "delete <key>",
		Short: "Delete a memory by its key. Also removes its embedding.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := newMemoryClient(dbPath, chatModel)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_delete", map[string]any{
				"key": args[0],
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

	return cmd
}
