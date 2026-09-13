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

func newRebuildIndexCmd() *cobra.Command {
	var dbPath, indexDir string

	cmd := &cobra.Command{
		Use:   "rebuild-index",
		Short: "Rebuild the in-process vector index from the database",
		Long: `Rebuild the memory-mapped vector index that answers semantic search.

The index is a derived artefact: the database is the source of truth. The
server builds it lazily and rebuilds it whenever the data changes, so this
command is only needed to force a rebuild (for example after moving the
database or deleting the index directory).

Examples:
  memory-cli rebuild-index
  memory-cli rebuild-index --db /path/to/memory.db --index-dir /path/to/memory.db-idx`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbPath == "" {
				configDir, err := os.UserConfigDir()
				if err != nil {
					return fmt.Errorf("determine config directory: %w", err)
				}
				dbPath = filepath.Join(configDir, "memory-store-mcp", "memory.db")
			}
			if indexDir == "" {
				indexDir = dbPath + "-idx"
			}

			kv, err := keyvalembd.New(dbPath)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer kv.Close()

			kv.SetVectorIndexDir(indexDir)
			fmt.Printf("building vector index in %s...\n", indexDir)
			if err := kv.RebuildVectorIndex(); err != nil {
				return fmt.Errorf("rebuild index: %w", err)
			}
			fmt.Println("done: vector index rebuilt")
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database (default: config dir)")
	cmd.Flags().StringVar(&indexDir, "index-dir", "", "Index directory (default: <db>-idx)")
	return cmd
}
