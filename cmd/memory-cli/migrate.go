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

func newMigrateVectorIndexCmd() *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "migrate-vector-index",
		Short: "Add and build the native libSQL vector index (one-off migration)",
		Long: `Prepare a memory-store-mcp database for native vector search.

The migration is idempotent and performs three steps:
  1. adds the embedding_vec F32_BLOB column if missing,
  2. backfills it from the existing embedding column,
  3. creates the DiskANN vector index if missing.

It is safe to re-run: the index is only built once, while the backfill step
also repairs any rows written without the vector column (for example by an
older server process), so they become visible to the index.

Building the index can take a couple of minutes on large databases, so this is
an explicit command rather than something done on server startup.

Examples:
  memory-cli migrate-vector-index
  memory-cli migrate-vector-index --db /path/to/memory.db`,
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

			fmt.Printf("preparing vector index in %s...\n", dbPath)
			if err := kv.MigrateVectorIndex(); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			if !kv.VectorIndexReady() {
				return fmt.Errorf("migration finished but vector index is not ready")
			}
			fmt.Println("done: vector index is ready")
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database (default: config dir)")
	return cmd
}
