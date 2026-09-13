// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Knowledge graph: entities and directed relations between them.
//
// The graph used to live as ordinary memory entries under memory/graph/, which
// meant every query listed all keys and fetched each one (O(N) reads) and the
// edges showed up in semantic search. It now lives in two indexed tables in the
// same database:
//
//	graph_entities  canonical name, lowercased lookup key, type, aliases
//	graph_edges     from_id, to_id, relation, date, source, confidence
//
// Entity lookup is case-insensitive through name_key, which is lowercased in Go
// (SQLite's lower()/COLLATE NOCASE only fold ASCII, so Cyrillic would not work).
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kirill-scherba/sqlh"
)

// GraphEntity is a node in the knowledge graph.
type GraphEntity struct {
	_       bool   `db_table_name:"graph_entities"`
	ID      int64  `db:"id" db_key:"primary key autoincrement"`
	Name    string `db:"name"`                 // display name, as first seen
	NameKey string `db:"name_key" db_key:"unique"` // lowercased lookup key
	Type    string `db:"type"`                 // person, place, image, doc, idea, ...
	Aliases string `db:"aliases"`              // JSON array of alternative spellings
}

// GraphEdge is a directed relation between two entities.
type GraphEdge struct {
	_          bool    `db_table_name:"graph_edges"`
	ID         int64   `db:"id" db_key:"primary key autoincrement"`
	FromID     int64   `db:"from_id"`
	ToID       int64   `db:"to_id"`
	Relation   string  `db:"relation"`
	Date       string  `db:"date"`
	Source     string  `db:"source"`
	Confidence float64 `db:"confidence"`

	_ string `db:"-" db_key:"UNIQUE (from_id, relation, to_id, date)"`
}

// GraphEdgeRow is a joined view of an edge with both entity names. The JSON
// tags keep the field names the consumers (the gallery) already expect.
type GraphEdgeRow struct {
	FromName   string  `db:"from_name" json:"from"`
	ToName     string  `db:"to_name" json:"to"`
	Relation   string  `db:"relation" json:"relation"`
	Date       string  `db:"date" json:"date"`
	Source     string  `db:"source" json:"source,omitempty"`
	Confidence float64 `db:"confidence" json:"-"`
}

// GraphNeighbor is an entity reachable from the queried one.
type GraphNeighbor struct {
	ID    int64  `db:"id"`
	Name  string `db:"name"`
	Type  string `db:"type"`
	Depth int    `db:"depth"`
}

// createGraphTables creates the graph tables if they do not exist.
func createGraphTables(db *sql.DB) error {
	if err := sqlh.Create[GraphEntity](db); err != nil {
		return fmt.Errorf("create graph_entities: %w", err)
	}
	if err := sqlh.Create[GraphEdge](db); err != nil {
		return fmt.Errorf("create graph_edges: %w", err)
	}
	// sqlh's db_key "KEY ..." syntax is MySQL-only, so the indexes are created
	// explicitly here. They turn edge lookups into index seeks.
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS graph_edges_from ON graph_edges (from_id, relation)`,
		`CREATE INDEX IF NOT EXISTS graph_edges_to ON graph_edges (to_id, relation)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create graph index: %w", err)
		}
	}
	return nil
}

// normalizeName returns the case-folded lookup key for an entity name.
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// resolveEntityID returns the id of the named entity, creating it when it does
// not exist yet. typ is only used on creation.
func resolveEntityID(db *sql.DB, name, typ string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("empty entity name")
	}
	key := normalizeName(name)

	existing, err := sqlh.Get[GraphEntity](db, sqlh.Where{Field: "name_key=", Value: key})
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("lookup entity %q: %w", name, err)
	}
	if existing != nil {
		return existing.ID, nil
	}

	id, err := sqlh.InsertId(db, GraphEntity{Name: name, NameKey: key, Type: typ})
	if err != nil {
		// A concurrent writer may have created it; re-read before giving up.
		if again, err2 := sqlh.Get[GraphEntity](db, sqlh.Where{Field: "name_key=", Value: key}); err2 == nil && again != nil {
			return again.ID, nil
		}
		return 0, fmt.Errorf("create entity %q: %w", name, err)
	}
	return id, nil
}

// addGraphEdge inserts an edge unless an identical one already exists. The
// unique constraint on (from_id, relation, to_id, date) makes this idempotent.
func addGraphEdge(db *sql.DB, from, to, relation, date, source string) error {
	fromID, err := resolveEntityID(db, from, "")
	if err != nil {
		return err
	}
	toID, err := resolveEntityID(db, to, "")
	if err != nil {
		return err
	}
	relation = strings.TrimSpace(relation)
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	existing, err := sqlh.Get[GraphEdge](db,
		sqlh.Where{Field: "from_id=", Value: fromID},
		sqlh.Where{Field: "to_id=", Value: toID},
		sqlh.Where{Field: "relation=", Value: relation},
		sqlh.Where{Field: "date=", Value: date},
	)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lookup edge: %w", err)
	}
	if existing != nil {
		return nil
	}

	_, err = sqlh.InsertId(db, GraphEdge{
		FromID:     fromID,
		ToID:       toID,
		Relation:   relation,
		Date:       date,
		Source:     source,
		Confidence: 1.0,
	})
	if err != nil {
		return fmt.Errorf("insert edge: %w", err)
	}
	return nil
}

// edgesForEntity returns every edge that touches the named entity, matched
// case-insensitively. It uses the (from_id, relation) and (to_id, relation)
// indexes instead of listing and fetching all graph entries.
func edgesForEntity(db *sql.DB, name string) ([]GraphEdgeRow, error) {
	key := normalizeName(name)
	if key == "" {
		return nil, nil
	}

	// Custom SELECT with joins: raw SQL and rows.Scan, as sqlh.QueryRange maps
	// table-composite structs rather than flat result rows.
	const query = `
		SELECT f.name AS from_name, t.name AS to_name,
		       e.relation, e.date, e.source, e.confidence
		FROM graph_edges e
		JOIN graph_entities f ON f.id = e.from_id
		JOIN graph_entities t ON t.id = e.to_id
		JOIN graph_entities q ON q.id = e.from_id OR q.id = e.to_id
		WHERE q.name_key = ?
		ORDER BY e.date DESC, e.id DESC`

	rows, err := db.Query(query, key)
	if err != nil {
		return nil, fmt.Errorf("query edges for %q: %w", name, err)
	}
	defer rows.Close()

	var out []GraphEdgeRow
	for rows.Next() {
		var r GraphEdgeRow
		if err := rows.Scan(&r.FromName, &r.ToName, &r.Relation,
			&r.Date, &r.Source, &r.Confidence); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edges for %q: %w", name, err)
	}
	return out, nil
}

// edgesAmong returns every edge whose two endpoints are both in ids.
func edgesAmong(db *sql.DB, ids []int64) ([]GraphEdgeRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)*2)
	for _, id := range ids {
		args = append(args, id)
	}
	for _, id := range ids {
		args = append(args, id)
	}

	// Raw SQL with rows.Scan: custom SELECT with joins (see edgesForEntity).
	query := `
		SELECT f.name AS from_name, t.name AS to_name,
		       e.relation, e.date, e.source, e.confidence
		FROM graph_edges e
		JOIN graph_entities f ON f.id = e.from_id
		JOIN graph_entities t ON t.id = e.to_id
		WHERE e.from_id IN (` + placeholders + `)
		  AND e.to_id IN (` + placeholders + `)
		ORDER BY e.date DESC, e.id DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges among %d entities: %w", len(ids), err)
	}
	defer rows.Close()

	var out []GraphEdgeRow
	for rows.Next() {
		var r GraphEdgeRow
		if err := rows.Scan(&r.FromName, &r.ToName, &r.Relation,
			&r.Date, &r.Source, &r.Confidence); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edges: %w", err)
	}
	return out, nil
}

// neighborsForEntity returns the entities reachable from the named one within
// maxDepth hops, following edges in both directions. This is real traversal,
// done with a recursive CTE: the previous implementation ignored depth.
func neighborsForEntity(db *sql.DB, name string, maxDepth int) ([]GraphNeighbor, error) {
	key := normalizeName(name)
	if key == "" {
		return nil, nil
	}
	if maxDepth < 1 {
		maxDepth = 1
	}

	// UNION (not UNION ALL) deduplicates, so cycles terminate.
	const query = `
		WITH RECURSIVE start(id) AS (
			SELECT id FROM graph_entities WHERE name_key = ?
		),
		reach(id, depth) AS (
			SELECT id, 0 FROM start
			UNION
			SELECT e.to_id, r.depth + 1
			  FROM graph_edges e JOIN reach r ON e.from_id = r.id
			 WHERE r.depth < ?
			UNION
			SELECT e.from_id, r.depth + 1
			  FROM graph_edges e JOIN reach r ON e.to_id = r.id
			 WHERE r.depth < ?
		)
		SELECT en.id AS id, en.name AS name, en.type AS type,
		       MIN(reach.depth) AS depth
		  FROM reach JOIN graph_entities en ON en.id = reach.id
		 WHERE reach.depth > 0 AND en.id <> (SELECT id FROM start)
		 GROUP BY en.id, en.name, en.type
		 ORDER BY depth, en.name`

	// Recursive CTE: raw SQL and rows.Scan (see edgesForEntity).
	rows, err := db.Query(query, key, maxDepth, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("traverse from %q: %w", name, err)
	}
	defer rows.Close()

	var out []GraphNeighbor
	for rows.Next() {
		var n GraphNeighbor
		if err := rows.Scan(&n.ID, &n.Name, &n.Type, &n.Depth); err != nil {
			return nil, fmt.Errorf("scan neighbour: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate neighbours from %q: %w", name, err)
	}
	return out, nil
}

// legacyGraphEdge is the value format used by the old memory/graph/ entries.
type legacyGraphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
	Date     string `json:"date"`
}

// migrateLegacyGraph moves memory/graph/ entries into the graph tables and
// removes them from the key-value store, where they polluted semantic search.
// It is idempotent: once the keys are gone it does nothing.
func migrateLegacyGraph(s *Storage) (int, error) {
	const prefix = "memory/graph/"

	// Raw SQL, not s.kv.List: the S3-style listing collapses sub-folders, which
	// would hide keys like memory/graph/<date>/<from>-<to>-<rel>.
	rows, err := s.goals.Query(
		`SELECT key FROM kv_data WHERE key LIKE ? ORDER BY key`, prefix+"%")
	if err != nil {
		return 0, fmt.Errorf("list legacy graph keys: %w", err)
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan legacy graph key: %w", err)
		}
		if !strings.HasSuffix(key, "/") {
			keys = append(keys, key)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate legacy graph keys: %w", err)
	}
	rows.Close()
	if len(keys) == 0 {
		return 0, nil
	}

	var migrated int
	for _, key := range keys {
		mv, err := s.Get(key)
		if err != nil || mv == nil {
			continue
		}
		var e legacyGraphEdge
		if err := json.Unmarshal([]byte(mv.Content), &e); err != nil {
			continue
		}
		if e.From == "" || e.To == "" || e.Relation == "" {
			continue
		}
		if err := addGraphEdge(s.goals, e.From, e.To, e.Relation, e.Date, key); err != nil {
			return migrated, fmt.Errorf("migrate %s: %w", key, err)
		}
		migrated++
	}

	// Drop the legacy entries only after they are safely in the tables.
	for _, key := range keys {
		if err := s.kv.Del(key); err != nil {
			return migrated, fmt.Errorf("delete legacy key %s: %w", key, err)
		}
	}

	log.Printf("🧹 migrated %d graph edges from %s into graph_edges", migrated, prefix)
	return migrated, nil
}

// graphStats returns the number of entities and edges.
func graphStats(db *sql.DB) (entities, edges int, err error) {
	et, err := sqlh.CreateTable[GraphEntity](db)
	if err != nil {
		return 0, 0, err
	}
	entities, err = et.Count()
	if err != nil {
		return 0, 0, err
	}
	gt, err := sqlh.CreateTable[GraphEdge](db)
	if err != nil {
		return 0, 0, err
	}
	edges, err = gt.Count()
	return entities, edges, err
}
