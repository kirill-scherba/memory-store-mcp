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
	"path"
	"sort"
	"strings"
	"time"

	"github.com/kirill-scherba/sqlh"
)

// GraphEntity is a node in the knowledge graph.
type GraphEntity struct {
	_       bool   `db_table_name:"graph_entities"`
	ID      int64  `db:"id" db_key:"primary key autoincrement"`
	Name    string `db:"name"`                     // display name, as first seen
	NameKey string `db:"name_key" db_key:"unique"` // lowercased lookup key
	Type    string `db:"type"`                     // person, place, image, doc, idea, ...
	Aliases string `db:"aliases"`                  // JSON array of alternative spellings
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
	FromName string `db:"from_name" json:"from"`
	ToName   string `db:"to_name" json:"to"`
	// Entity types are used internally (to filter low-signal relations) and are
	// deliberately not serialised: the gallery contract is from/to/relation/date.
	FromType   string  `db:"from_type" json:"-"`
	ToType     string  `db:"to_type" json:"-"`
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
	if err := createAliasTable(db); err != nil {
		return err
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

	// A registered alias resolves to the canonical entity, so "Kirill" and
	// "Кирилл" end up as one node instead of two.
	if id, ok, err := resolveAliasKey(db, key); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	// Infer the type from the name when the caller did not supply one. The
	// relations an entity takes part in refine this later.
	if typ == "" {
		typ = inferEntityTypeFromName(name)
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
	return addGraphEdgeTyped(db, from, "", to, "", relation, date, source)
}

// addGraphEdgeTyped is addGraphEdge with type hints for the two endpoints. A
// hint is used only when the entity is created, so it can never overwrite a
// type the registry already established. Deterministic extractors know what
// they extracted — a mail sender is an organisation, a dish is a dish — and
// saying so here is what keeps the graph typed as it grows.
func addGraphEdgeTyped(db *sql.DB, from, fromType, to, toType, relation, date, source string) error {
	fromID, err := resolveEntityID(db, from, fromType)
	if err != nil {
		return err
	}
	toID, err := resolveEntityID(db, to, toType)
	if err != nil {
		return err
	}
	relation = strings.TrimSpace(relation)
	// Fold extractor spellings into the vocabulary (заказала -> заказал). An
	// unknown relation is kept as written so no fact is lost; relationReport
	// lists it as vocabulary work.
	if canonical, known := canonicalRelation(relation); known {
		relation = canonical
	}
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
// case-insensitively and through the alias registry. The name is resolved to an
// entity id first, so an alias finds the canonical node's edges: a merged
// document's ".md" spelling still returns the surviving node's edges.
func edgesForEntity(db *sql.DB, name string) ([]GraphEdgeRow, error) {
	ent, err := getEntityByName(db, name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return edgesForEntityID(db, ent.ID)
}

// GraphContextItem is one entity with its immediate connections, ready to be
// injected into the agent's context.
type GraphContextItem struct {
	Entity string         `json:"entity"`
	Type   string         `json:"type,omitempty"`
	Edges  []GraphEdgeRow `json:"edges"`
}

// edgesForEntityID returns every edge touching the entity with the given id.
func edgesForEntityID(db *sql.DB, id int64) ([]GraphEdgeRow, error) {
	// Raw SQL with rows.Scan: custom SELECT with joins (see edgesForEntity).
	const query = `
		SELECT f.name AS from_name, t.name AS to_name,
		       f.type AS from_type, t.type AS to_type,
		       e.relation, e.date, e.source, e.confidence
		FROM graph_edges e
		JOIN graph_entities f ON f.id = e.from_id
		JOIN graph_entities t ON t.id = e.to_id
		WHERE e.from_id = ? OR e.to_id = ?
		ORDER BY e.date DESC, e.id DESC`

	rows, err := db.Query(query, id, id)
	if err != nil {
		return nil, fmt.Errorf("query edges for entity %d: %w", id, err)
	}
	defer rows.Close()

	var out []GraphEdgeRow
	for rows.Next() {
		var r GraphEdgeRow
		if err := rows.Scan(&r.FromName, &r.ToName, &r.FromType, &r.ToType,
			&r.Relation, &r.Date, &r.Source, &r.Confidence); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edges for entity %d: %w", id, err)
	}
	return out, nil
}

// entityStems returns the forms of an entity name to look for in free text.
// Russian inflects the ending ("Сварня" -> "в Сварне"), so for names long
// enough to stay specific the last one or two characters are also tried.
func entityStems(nameKey string) []string {
	r := []rune(nameKey)
	out := []string{nameKey}
	if len(r) >= 6 {
		out = append(out, string(r[:len(r)-1]))
	}
	if len(r) >= 8 {
		out = append(out, string(r[:len(r)-2]))
	}
	return out
}

// entitiesInText returns the entities whose name appears in text, in any
// common inflection. The comparison is case-folded in Go because SQLite's
// lower() only folds ASCII.
//
// The whole entity table is read and matched in Go: it is small (hundreds of
// rows), and matching stems cannot be expressed as an index seek anyway. If it
// ever grows into the tens of thousands this should become an FTS index.
func entitiesInText(db *sql.DB, text string, limit int) ([]GraphEntity, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	all := limit <= 0
	if limit <= 0 {
		limit = 5
	}
	haystack := normalizeName(text)

	rows, err := db.Query(`SELECT id, name, name_key, type, aliases FROM graph_entities`)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}
	defer rows.Close()

	type match struct {
		entity GraphEntity
		at     int // first occurrence in the text
		stem   int // matched stem length, for tie-breaking
	}
	var matches []match
	for rows.Next() {
		var e GraphEntity
		if err := rows.Scan(&e.ID, &e.Name, &e.NameKey, &e.Type, &e.Aliases); err != nil {
			return nil, fmt.Errorf("scan entity: %w", err)
		}
		for _, stem := range entityStems(e.NameKey) {
			if len([]rune(stem)) < 3 {
				continue
			}
			if at := strings.Index(haystack, stem); at >= 0 {
				matches = append(matches, match{entity: e, at: at, stem: len([]rune(stem))})
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Order by where the entity first appears; at the same position prefer the
	// longer match, so "кошелёк Барона" beats "Барон".
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].at != matches[j].at {
			return matches[i].at < matches[j].at
		}
		return matches[i].stem > matches[j].stem
	})
	if !all && len(matches) > limit {
		matches = matches[:limit]
	}

	out := make([]GraphEntity, len(matches))
	for i, m := range matches {
		out[i] = m.entity
	}
	return out, nil
}

// graphContextForText finds the entities mentioned in text and returns them
// with their immediate connections. This is the consumption half of the graph:
// the agent gets the connections without having to remember to ask for them.
func (s *Storage) graphContextForText(text string, maxEntities int) ([]GraphContextItem, error) {
	entities, err := entitiesInText(s.goals, text, maxEntities)
	if err != nil {
		return nil, err
	}

	items := make([]GraphContextItem, 0, len(entities))
	for _, e := range entities {
		edges, err := edgesForEntityID(s.goals, e.ID)
		if err != nil {
			return nil, err
		}
		if len(edges) == 0 {
			continue
		}
		items = append(items, GraphContextItem{Entity: e.Name, Type: e.Type, Edges: edges})
	}
	return items, nil
}

// autoMentionRelation is the relation used for automatically derived links from
// image metadata; imageEntityPrefix marks image entity names.
const (
	autoMentionRelation = "упоминает"
	imageEntityType     = "image"
)

// looksLikeMedia reports whether an entity name looks like a media file. Some
// image entities predate the "image" type (legacy migration stored them with an
// empty type), so the name is a useful second signal.
func looksLikeMedia(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".wav", ".mp3", ".mp4":
		return true
	}
	return false
}

// graphContextLimit caps how much graph detail is injected.
const (
	graphContextEntities     = 5
	graphContextGroups       = 6
	graphContextNamesPerLine = 8
)

// formatGraphContext renders graph items compactly for context injection.
func formatGraphContext(items []GraphContextItem) string {
	if len(items) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, "=== Graph context ===")

	for _, item := range items {
		header := item.Entity
		if item.Type != "" {
			header += " (" + item.Type + ")"
		}
		parts = append(parts, header)

		// Group by relation and direction, preserving first-seen order.
		type key struct {
			relation string
			outgoing bool
		}
		order := make([]key, 0, 8)
		seen := make(map[key][]string)
		for _, e := range item.Edges {
			outgoing := e.FromName == item.Entity
			var k key
			var other string
			if outgoing {
				k = key{e.Relation, true}
				other = e.ToName
			} else {
				k = key{e.Relation, false}
				other = e.FromName
			}

			// "Image X mentions entity Y" is useful in the gallery but is noise
			// in injected context: a well-known entity is mentioned by dozens of
			// images and that would drown its real relations.
			otherType := e.ToType
			if !outgoing {
				otherType = e.FromType
			}
			if k.relation == autoMentionRelation &&
				(otherType == imageEntityType || looksLikeMedia(other)) {
				continue
			}

			if _, ok := seen[k]; !ok {
				order = append(order, k)
			}
			seen[k] = append(seen[k], other)
		}

		shown := 0
		for _, k := range order {
			if shown >= graphContextGroups {
				parts = append(parts, fmt.Sprintf("  ... and %d more relations", len(order)-shown))
				break
			}
			names := dedupeStrings(seen[k])
			arrow := "→"
			if !k.outgoing {
				arrow = "←"
			}
			line := fmt.Sprintf("  %s %s %s", k.relation, arrow,
				strings.Join(truncateNames(names, graphContextNamesPerLine), ", "))
			parts = append(parts, line)
			shown++
		}
	}

	return strings.Join(parts, "\n")
}

// dedupeStrings removes duplicates, preserving order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// truncateNames keeps at most n names, adding a summary of the rest.
func truncateNames(names []string, n int) []string {
	if len(names) <= n {
		return names
	}
	out := append([]string{}, names[:n]...)
	out = append(out, fmt.Sprintf("+%d more", len(names)-n))
	return out
}

// extractImageEdges links a gallery image to the entities mentioned in its
// metadata. The relation is a neutral "упоминает": the manual edges keep their
// precise relations (фото_в, иллюстрация, сгенерировал), and this only adds the
// ones nobody entered by hand.
func (s *Storage) extractImageEdges(imageName, text string, maxEdges int) (int, error) {
	imageName = strings.TrimSpace(imageName)
	if imageName == "" || strings.TrimSpace(text) == "" {
		return 0, nil
	}

	subjectID, err := resolveEntityID(s.goals, imageName, imageEntityType)
	if err != nil {
		return 0, err
	}
	entities, err := entitiesInText(s.goals, text, 0) // all matches
	if err != nil {
		return 0, err
	}

	// Skip entities the image is already connected to, by any relation.
	existing, err := edgesForEntityID(s.goals, subjectID)
	if err != nil {
		return 0, err
	}
	have := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		other := e.ToName
		if e.FromName != imageName {
			other = e.FromName
		}
		have[normalizeName(other)] = struct{}{}
	}

	added := 0
	for _, e := range entities {
		if e.ID == subjectID {
			continue
		}
		if _, ok := have[e.NameKey]; ok {
			continue
		}
		if err := addGraphEdge(s.goals, imageName, e.Name, autoMentionRelation, "", "auto:gallery"); err != nil {
			return added, err
		}
		added++
		if maxEdges > 0 && added >= maxEdges {
			break
		}
	}
	return added, nil
}

// galleryMetaEntry is the value stored under memory/gallery/meta/<file>.
//
// tags is inconsistent in the stored data: older entries hold a
// comma-separated string, newer ones a JSON array. It is kept raw and
// normalised by tagsText.
type galleryMetaEntry struct {
	Description string          `json:"description"`
	Tags        json.RawMessage `json:"tags"`
	Prompt      string          `json:"prompt"`
}

// tagsText renders tags as plain text, accepting both stored shapes.
func (m galleryMetaEntry) tagsText() string {
	if len(m.Tags) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Tags, &s); err == nil {
		return s
	}
	var list []string
	if err := json.Unmarshal(m.Tags, &list); err == nil {
		return strings.Join(list, " ")
	}
	return ""
}

// backfillImageGraph links every gallery image to the entities mentioned in its
// metadata. Idempotent: extractImageEdges skips entities already connected.
func (s *Storage) backfillImageGraph() (int, error) {
	const prefix = "memory/gallery/meta/"

	rows, err := s.goals.Query(
		`SELECT key FROM kv_data WHERE key LIKE ? ORDER BY key`, prefix+"%")
	if err != nil {
		return 0, fmt.Errorf("list gallery meta: %w", err)
	}
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return 0, err
		}
		if !strings.HasSuffix(k, "/") {
			keys = append(keys, k)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var total int
	for _, key := range keys {
		mv, err := s.Get(key)
		if err != nil || mv == nil {
			continue
		}
		// The value is a MemoryValue whose Content holds the metadata JSON.
		var meta galleryMetaEntry
		if err := json.Unmarshal([]byte(mv.Content), &meta); err != nil {
			continue
		}
		imageName := strings.TrimPrefix(key, prefix)
		text := meta.Description + " " + meta.tagsText() + " " + meta.Prompt
		n, err := s.extractImageEdges(imageName, text, 10)
		if err != nil {
			return total, fmt.Errorf("image %s: %w", imageName, err)
		}
		total += n
	}
	return total, nil
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
		       f.type AS from_type, t.type AS to_type,
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
		if err := rows.Scan(&r.FromName, &r.ToName, &r.FromType, &r.ToType,
			&r.Relation, &r.Date, &r.Source, &r.Confidence); err != nil {
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
	// Resolve through the alias registry so traversal starts at the canonical
	// node even when the caller used a retired spelling.
	ent, err := getEntityByName(db, name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if maxDepth < 1 {
		maxDepth = 1
	}

	// UNION (not UNION ALL) deduplicates, so cycles terminate.
	const query = `
		WITH RECURSIVE start(id) AS (
			SELECT ?
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
	rows, err := db.Query(query, ent.ID, maxDepth, maxDepth)
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
