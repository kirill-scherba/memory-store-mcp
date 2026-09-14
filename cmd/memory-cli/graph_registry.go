// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// newGraphRegistryCmds returns the entity-registry subcommands of `graph`:
// repair, merge, alias and relations. They all go through the MCP server, which
// owns the graph tables — the CLI never opens the database itself.
func newGraphRegistryCmds() []*cobra.Command {
	var dbPath, serverURL string

	// graph repair
	var dryRun bool
	repairCmd := &cobra.Command{
		Use:   "repair",
		Short: "Repair the graph: types, duplicate docs, relation spellings",
		Long: `Assign entity types, fold duplicate document nodes and canonicalise
relation spellings. Idempotent and non-destructive: no fact is deleted, and
relations outside the vocabulary are reported rather than rewritten.

Use --dry-run to see the numbers without writing anything.`,
		Example: `  memory-cli graph repair --dry-run
  memory-cli graph repair`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			result, err := client.callTool("graph_repair", map[string]any{"dry_run": dryRun})
			if err != nil {
				return fmt.Errorf("graph_repair call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	repairCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	repairCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	repairCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would change without writing")

	// graph merge
	var keep, drop string
	mergeCmd := &cobra.Command{
		Use:   "merge --keep <entity> --drop <entity>",
		Short: "Merge two entities into one node",
		Long: `Fold one entity into another: edges are re-pointed, duplicates collapse,
and the retired name becomes an alias so the old spelling keeps resolving.`,
		Example: `  memory-cli graph merge --keep Кирилл --drop Kirill
  memory-cli graph merge --keep chapter-08-baron-sees --drop chapter-08-baron-sees.md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keep == "" || drop == "" {
				return fmt.Errorf("both --keep and --drop are required")
			}
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			result, err := client.callTool("graph_merge", map[string]any{"keep": keep, "drop": drop})
			if err != nil {
				return fmt.Errorf("graph_merge call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	mergeCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	mergeCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	mergeCmd.Flags().StringVar(&keep, "keep", "", "Entity to keep (canonical node)")
	mergeCmd.Flags().StringVar(&drop, "drop", "", "Entity to fold into keep")

	// graph alias
	var aliasEntity, aliasName string
	aliasCmd := &cobra.Command{
		Use:     "alias --entity <entity> --alias <spelling>",
		Short:   "Register an alternative spelling for an entity",
		Long:    `After this, the alias resolves to the same node instead of creating a duplicate.`,
		Example: `  memory-cli graph alias --entity Кирилл --alias Kirill`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if aliasEntity == "" || aliasName == "" {
				return fmt.Errorf("both --entity and --alias are required")
			}
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			result, err := client.callTool("graph_alias", map[string]any{"entity": aliasEntity, "alias": aliasName})
			if err != nil {
				return fmt.Errorf("graph_alias call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	aliasCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	aliasCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	aliasCmd.Flags().StringVar(&aliasEntity, "entity", "", "Canonical entity name")
	aliasCmd.Flags().StringVar(&aliasName, "alias", "", "Alternative spelling")

	// graph relations
	var vocabularyOnly bool
	relationsCmd := &cobra.Command{
		Use:   "relations",
		Short: "Report the relation vocabulary and the graph state",
		Long: `Show every relation present in the data, its canonical form, whether it
belongs to the closed vocabulary, and the entity type distribution.`,
		Example: `  memory-cli graph relations
  memory-cli graph relations --vocabulary`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			result, err := client.callTool("graph_relations", map[string]any{"vocabulary_only": vocabularyOnly})
			if err != nil {
				return fmt.Errorf("graph_relations call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	relationsCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	relationsCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	relationsCmd.Flags().BoolVar(&vocabularyOnly, "vocabulary", false, "Show only the closed vocabulary")

	// graph backfill
	backfillCmd := &cobra.Command{
		Use:   "backfill",
		Short: "Extract relations from structured memory entries (no LLM)",
		Long: `Read the whole memory store and add the relations its structured entries
already state: visits, dishes, family relations, mail senders. New saves are
extracted as they happen; this is for the history written before the extractors
existed. Idempotent — re-running changes nothing.`,
		Example: `  memory-cli graph backfill`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			result, err := client.callTool("graph_backfill", map[string]any{})
			if err != nil {
				return fmt.Errorf("graph_backfill call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	backfillCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	backfillCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")

	// graph backfill-llm
	var llmLimit int
	var llmReset bool
	var llmTimeout int
	var llmPrefix string
	backfillLLMCmd := &cobra.Command{
		Use:   "backfill-llm",
		Short: "Extract relations from narrative entries with the LLM",
		Long: `Read narrative memory entries (memoirs, conversations, notes, goals) with
the LLM and add the relations they state in prose. Deterministic extractors
cannot read prose; this is what covers it.

Bounded by --limit and resumable: each run continues where the last stopped, so
the cost of a full pass can be spread over several runs.`,
		Example: `  memory-cli graph backfill-llm --limit 10
  memory-cli graph backfill-llm --limit 50
  memory-cli graph backfill-llm --limit 10 --reset`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// One model call per entry: the session must outlive the batch.
			if llmTimeout > 0 {
				mcpCallTimeout = time.Duration(llmTimeout) * time.Second
			}
			client, err := newMemoryClient(dbPath, "", serverURL)
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}
			defer client.close()

			result, err := client.callTool("graph_backfill_llm", map[string]any{
				"limit":  float64(llmLimit),
				"reset":  llmReset,
				"prefix": llmPrefix,
			})
			if err != nil {
				return fmt.Errorf("graph_backfill_llm call: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	backfillLLMCmd.Flags().StringVar(&dbPath, "db", "", "Path to the memory-store-mcp database (stdio mode)")
	backfillLLMCmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL")
	backfillLLMCmd.Flags().IntVar(&llmLimit, "limit", 10, "Maximum number of model calls in this run")
	backfillLLMCmd.Flags().BoolVar(&llmReset, "reset", false, "Start from the beginning instead of resuming")
	backfillLLMCmd.Flags().IntVar(&llmTimeout, "timeout", 1800, "MCP session timeout in seconds")
	backfillLLMCmd.Flags().StringVar(&llmPrefix, "prefix", "", "Restrict the run to one key prefix (e.g. memory/user/)")

	return []*cobra.Command{repairCmd, mergeCmd, aliasCmd, relationsCmd, backfillCmd, backfillLLMCmd}
}
