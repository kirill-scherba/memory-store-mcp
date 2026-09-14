// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// MCP tools for the entity registry: repairing the graph, merging duplicates,
// registering aliases, and reporting the relation vocabulary.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// graphRepairTool runs the Phase 1 repair: it types what the names and the
// relations already imply, folds document duplicates, and canonicalises relation
// spellings. It never deletes a fact — edges that contradict the vocabulary are
// reported for a human to judge.
func graphRepairTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_repair",
			mcp.WithDescription(`Repair the knowledge graph: assign entity types, merge duplicate document nodes, canonicalise relation spellings.
Idempotent and non-destructive — no fact is deleted, unknown relations are reported rather than rewritten.
Use dry_run=true to see what would change without writing.`),
			mcp.WithBoolean("dry_run",
				mcp.Description("Report what would change without writing (default: false)"),
			),
			mcp.WithBoolean("retype",
				mcp.Description("Re-derive entity types from scratch. Use after the relation vocabulary changes: a type, once set, is never overwritten, so a type derived from a wrong rule stays wrong."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			dryRun, _ := args["dry_run"].(bool)
			retype, _ := args["retype"].(bool)
			db := s.goals

			before, err := graphOverviewOf(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}

			// Count the work that the write phases would do, so dry_run and a
			// real run report the same numbers.
			patternFixes, err := patternTypeFixes(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			patternTypes := len(patternFixes)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			candidates, err := typeCandidates(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			docPairs, err := duplicateDocPairs(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			docDups := len(docPairs)
			var relationFixes int
			for _, rc := range before.Relations {
				if rc.Canonical != rc.Relation {
					relationFixes += rc.Count
				}
			}

			report := map[string]any{
				"dry_run":               dryRun,
				"entities_before":       before.Entities,
				"edges_before":          before.Edges,
				"untyped_before":        before.UntypedEntities,
				"pattern_types":         patternTypes,
				"type_candidates":       len(candidates),
				"duplicate_docs":        docDups,
				"relation_rows_to_fold": relationFixes,
			}

			if dryRun {
				out, _ := json.MarshalIndent(report, "", "  ")
				return mcp.NewToolResultText(fmt.Sprintf("Dry run — nothing written.\n%s", string(out))), nil
			}

			cleared := 0
			if retype {
				if cleared, err = resetDerivedTypes(db); err != nil {
					return mcp.NewToolResultText(fmt.Sprintf("Error clearing derived types: %v", err)), nil
				}
			}
			typed, err := applyPatternTypes(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error applying pattern types: %v", err)), nil
			}
			propagated, ambiguous, err := propagateEntityTypes(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error propagating types: %v", err)), nil
			}
			removed, err := mergeDuplicateDocs(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error merging duplicates: %v", err)), nil
			}
			folded, err := normalizeStoredRelations(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error folding relations: %v", err)), nil
			}

			after, err := graphOverviewOf(db)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}

			report["pattern_types_applied"] = typed
			report["types_cleared_for_retype"] = cleared
			report["types_propagated"] = propagated
			report["types_ambiguous"] = ambiguous
			report["duplicate_docs_merged"] = removed
			report["relation_rows_folded"] = folded
			report["entities_after"] = after.Entities
			report["edges_after"] = after.Edges
			report["untyped_after"] = after.UntypedEntities

			var unknown []RelationCount
			for _, rc := range after.Relations {
				if !rc.InVocabulary {
					unknown = append(unknown, rc)
				}
			}
			report["relations_outside_vocabulary"] = unknown

			out, _ := json.MarshalIndent(report, "", "  ")
			return mcp.NewToolResultText(string(out)), nil
		},
	}
}

// graphMergeTool folds one entity into another and keeps the retired spelling
// as an alias.
func graphMergeTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_merge",
			mcp.WithDescription(`Merge one entity into another: edges are re-pointed, duplicates collapse, and the retired name becomes an alias so the old spelling keeps resolving.
Use when two nodes are the same thing ("Kirill" and "Кирилл", "chapter-08-x" and "chapter-08-x.md").`),
			mcp.WithString("keep",
				mcp.Description("Entity to keep (the canonical node)"),
				mcp.Required(),
			),
			mcp.WithString("drop",
				mcp.Description("Entity to fold into keep (its name becomes an alias)"),
				mcp.Required(),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			keepName, _ := args["keep"].(string)
			dropName, _ := args["drop"].(string)
			if keepName == "" || dropName == "" {
				return mcp.NewToolResultText("Error: keep and drop are required"), nil
			}

			keep, err := getEntityByName(s.goals, keepName)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: entity %q not found", keepName)), nil
			}
			drop, err := getEntityByName(s.goals, dropName)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: entity %q not found", dropName)), nil
			}
			if keep.ID == drop.ID {
				return mcp.NewToolResultText(fmt.Sprintf("%q and %q are already the same entity (id %d)", keepName, dropName, keep.ID)), nil
			}

			moved, merged, err := mergeEntities(s.goals, keep.ID, drop.ID)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf(
				"Merged %q into %q: %d edges moved, %d collapsed as duplicates. %q now resolves to %q.",
				drop.Name, keep.Name, moved, merged, drop.Name, keep.Name)), nil
		},
	}
}

// graphAliasTool registers an alternative spelling for an entity.
func graphAliasTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_alias",
			mcp.WithDescription("Register an alternative spelling for an entity, so the alias resolves to the same node instead of creating a duplicate."),
			mcp.WithString("entity",
				mcp.Description("Canonical entity name"),
				mcp.Required(),
			),
			mcp.WithString("alias",
				mcp.Description("Alternative spelling to register"),
				mcp.Required(),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			entity, _ := args["entity"].(string)
			alias, _ := args["alias"].(string)
			if entity == "" || alias == "" {
				return mcp.NewToolResultText("Error: entity and alias are required"), nil
			}

			ent, err := getEntityByName(s.goals, entity)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: entity %q not found", entity)), nil
			}
			if err := addEntityAlias(s.goals, ent.ID, alias); err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			aliases, _ := aliasesForEntity(s.goals, ent.ID)
			return mcp.NewToolResultText(fmt.Sprintf("%q now also resolves to %q (id %d). Aliases: %v",
				alias, ent.Name, ent.ID, aliases)), nil
		},
	}
}

// graphRelationsTool reports the relation vocabulary and the relations present
// in the data that fall outside it.
func graphRelationsTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_relations",
			mcp.WithDescription("Report the knowledge graph vocabulary: every relation in the data with its canonical form, whether it belongs to the vocabulary, and the entity type distribution."),
			mcp.WithBoolean("vocabulary_only",
				mcp.Description("Return only the closed vocabulary, not the data report (default: false)"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			vocabOnly, _ := args["vocabulary_only"].(bool)

			if vocabOnly {
				type spec struct {
					Name     string   `json:"name"`
					Aliases  []string `json:"aliases,omitempty"`
					From     []string `json:"from,omitempty"`
					To       []string `json:"to,omitempty"`
					Document bool     `json:"document,omitempty"`
				}
				specs := make([]spec, 0, len(relationVocabulary))
				for _, r := range relationVocabulary {
					specs = append(specs, spec{r.Name, r.Aliases, r.From, r.To, r.Document})
				}
				out, _ := json.MarshalIndent(specs, "", "  ")
				return mcp.NewToolResultText(string(out)), nil
			}

			ov, err := graphOverviewOf(s.goals)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			var unknown int
			for _, rc := range ov.Relations {
				if !rc.InVocabulary {
					unknown++
				}
			}
			out, _ := json.MarshalIndent(ov, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf(
				"Graph: %d entities, %d edges, %d untyped. %d relations in the data, %d outside the vocabulary.\n%s",
				ov.Entities, ov.Edges, ov.UntypedEntities, len(ov.Relations), unknown, string(out))), nil
		},
	}
}

// graphBackfillTool applies the deterministic extractors to the whole memory
// store. New saves are extracted as they happen; this is for the history that
// was written before the extractors existed.
func graphBackfillTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_backfill",
			mcp.WithDescription(`Read the whole memory store and add the relations its structured entries already state (visits, dishes, family relations, mail senders).
No LLM is involved: an extractor only fires on fields it recognises. Idempotent — re-running changes nothing.`),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			before, err := graphOverviewOf(s.goals)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			report, err := s.backfillDeterministicGraph()
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			after, err := graphOverviewOf(s.goals)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}

			// Types implied by the new edges are worth filling in immediately:
			// the extractors introduce entities the graph had never seen.
			propagated, _, _ := propagateEntityTypes(s.goals)

			out, _ := json.MarshalIndent(map[string]any{
				"entries_scanned":  report.EntriesScanned,
				"entries_matched":  report.EntriesMatched,
				"triples":          report.Triples,
				"by_extractor":     report.ByExtractor,
				"by_relation":      report.ByRelation,
				"entities_before":  before.Entities,
				"entities_after":   after.Entities,
				"edges_before":     before.Edges,
				"edges_after":      after.Edges,
				"types_propagated": propagated,
				"untyped_after":    after.UntypedEntities,
			}, "", "  ")
			return mcp.NewToolResultText(string(out)), nil
		},
	}
}

// graphBackfillLLMTool runs the LLM extractor over narrative memory entries
// (memoirs, conversations, notes, goal descriptions) and adds the relations it
// finds. Bounded by limit and resumable, so the work can be spread over runs.
func graphBackfillLLMTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_backfill_llm",
			mcp.WithDescription(`Read narrative memory entries (memoirs, conversations, notes, goals) with the LLM and add the relations they state in prose.
Bounded by limit and resumable: each run continues where the last stopped, so the cost of a full pass can be spread out. Use reset=true to start over.`),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of model calls in this run (default 10)"),
			),
			mcp.WithBoolean("reset",
				mcp.Description("Ignore the stored cursor and start from the beginning (default: false)"),
			),
			mcp.WithString("prefix",
				mcp.Description("Restrict this run to one key prefix (e.g. memory/user/). Does not move the full-backfill cursor."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			limit := 10
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = int(v)
			}
			reset, _ := args["reset"].(bool)
			prefix, _ := args["prefix"].(string)

			report, err := s.backfillLLMGraph(limit, reset, prefix)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			propagated, _, _ := propagateEntityTypes(s.goals)
			out, _ := json.MarshalIndent(map[string]any{
				"scanned":            report.Scanned,
				"extracted":          report.Extracted,
				"triples":            report.Triples,
				"facts":              report.Facts,
				"outside_vocabulary": report.Outside,
				"failed":             report.Failed,
				"relations":          report.Relations,
				"outside_relations":  report.OutsideRelations,
				"last_key":           report.LastKey,
				"remaining":          report.Remaining,
				"types_propagated":   propagated,
			}, "", "  ")
			return mcp.NewToolResultText(string(out)), nil
		},
	}
}
