// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/spf13/cobra"
)

func newCompactCmd() *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "compact",
		Short: "Reclaim space: drop the legacy embedding column and VACUUM",
		Long: `Compact a memory-store-mcp database.

Steps:
  1. drop the legacy embedding BLOB column, which duplicates embedding_vec
     (a no-op when it is already gone);
  2. VACUUM the database to reclaim space freed by deletions.

The timeline purge (read noise and aged events) is handled by the server on
startup and every 6 hours. Run the server once before compacting so that
VACUUM can reclaim that space too.

Examples:
  memory-cli compact
  memory-cli compact --db /path/to/memory.db`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbPath == "" {
				configDir, err := os.UserConfigDir()
				if err != nil {
					return fmt.Errorf("determine config directory: %w", err)
				}
				dbPath = filepath.Join(configDir, "memory-store-mcp", "memory.db")
			}

			kv, err := keyvalembd.New(dbPath)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer kv.Close()

			dropped, err := kv.DropLegacyEmbeddingColumn()
			if err != nil {
				return fmt.Errorf("drop legacy embedding column: %w", err)
			}
			if dropped {
				fmt.Println("dropped the legacy embedding column")
			} else {
				fmt.Println("legacy embedding column already absent")
			}

			fmt.Printf("vacuuming %s (this can take a while)...\n", dbPath)
			if err := kv.Vacuum(); err != nil {
				return fmt.Errorf("vacuum: %w", err)
			}
			fmt.Println("done: database compacted")
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database (default: config dir)")
	return cmd
}
