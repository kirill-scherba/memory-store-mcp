// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Entity registry and relation vocabulary for the knowledge graph.
//
// Before this file the graph was a bag of triples. Every name that ever appeared
// became an entity, every relation the extractor invented became an edge, and
// nothing recorded what a relation meant or which way it pointed. The result was
// visible in production:
//
//   - duplicates: chapter-08-baron-sees and chapter-08-baron-sees.md were two
//     separate nodes for one document;
//   - documents as entities: memoir chapter keys and image filenames competed
//     with real people and places for space in the answer;
//   - directionless relations: "Сварня -> рубиновая: заказал" claims the place
//     ordered the dish, because nothing said заказал is person -> dish;
//   - a long tail of one-off relations (путает_имя, проводил_в_аэропорт) that
//     can never be queried, only accumulated.
//
// This file adds the three missing pieces:
//
//	graph_aliases   alias_key -> entity_id, so "Kirill" and "Кирилл" are one node
//	entity types    a closed set, so a relation can declare what it accepts
//	relation specs  a closed vocabulary with direction and endpoint types
//
// Nothing here deletes facts. Edges that contradict the vocabulary are reported,
// not removed: the vocabulary describes what the graph should be, and the gap
// between the two is the work queue.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/kirill-scherba/sqlh"
)

// Entity types. The set is closed on purpose: relations validate against it, so
// a type outside this list cannot take part in reasoning.
const (
	TypePerson  = "person"
	TypePlace   = "place"
	TypeDish    = "dish"
	TypeProject = "project"
	TypeTool    = "tool"
	TypeImage   = "image"
	TypeDoc     = "doc"
	TypeOrg     = "org"
	TypeEvent   = "event"
	TypeIdea    = "idea"
	TypeThing   = "thing"
)

// entityTypes is the closed set, used to reject unknown stored values.
var entityTypes = map[string]bool{
	TypePerson: true, TypePlace: true, TypeDish: true, TypeProject: true,
	TypeTool: true, TypeImage: true, TypeDoc: true, TypeOrg: true,
	TypeEvent: true, TypeIdea: true, TypeThing: true,
}

// RelationSpec is one entry of the relation vocabulary.
//
// From and To list the entity types allowed at each end. An empty list means
// "any type is acceptable" and is used where the object is open-ended (a
// document mentions anything). Document relations describe the archive rather
// than the world: they are the ones worth filtering out when answering a
// question about Kirill's life.
type RelationSpec struct {
	Name     string
	Aliases  []string
	From     []string
	To       []string
	Document bool
}

// relationVocabulary is the closed set of relations the graph is allowed to
// hold. It was derived from the relations actually present in production, so
// every canonical name here already has data behind it.
//
// Adding a relation is a deliberate act: a new relation must say what it means
// and which types it connects, otherwise the graph drifts back into a bag of
// triples. Unknown relations are still stored (dropping data silently would be
// worse) but they are reported by relationReport as work to do.
var relationVocabulary = []RelationSpec{
	// World relations.
	{Name: "был_в", Aliases: []string{"был", "побывал", "ходил_в", "пошёл_в", "поехал_в"},
		From: []string{TypePerson}, To: []string{TypePlace}},
	{Name: "живёт_в", Aliases: []string{"живет_в", "живёт"},
		From: []string{TypePerson}, To: []string{TypePlace}},
	{Name: "заказал", Aliases: []string{"заказала", "заказали", "заказывал"},
		From: []string{TypePerson}, To: []string{TypeDish}},
	{Name: "подают_в", Aliases: []string{"блюдо_в", "готовят_в"},
		From: []string{TypeDish}, To: []string{TypePlace}},
	{Name: "жена", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "муж", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "мать", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "отец", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "дочь", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "сын", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "был_с", Aliases: []string{"вместе_с", "гулял_с"},
		From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "направил", Aliases: []string{"отправил", "прислал"},
		From: []string{TypeOrg, TypePerson}, To: []string{TypePerson}},
	{Name: "друг", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "коллега", From: []string{TypePerson}, To: []string{TypePerson}},
	{Name: "работает_в", Aliases: []string{"работает_через", "провайдер"},
		From: []string{TypePerson, TypeProject}, To: []string{TypeOrg, TypeTool, TypeProject}},
	{Name: "владеет", Aliases: []string{"принадлежит"},
		From: []string{TypePerson, TypeOrg}, To: []string{TypeThing, TypeProject}},
	{Name: "породил_идею", Aliases: []string{"породила_идею"},
		From: []string{TypePlace, TypeEvent, TypePerson, TypeProject, TypeThing},
		To:   []string{TypeIdea, TypeProject, TypeTool, TypeThing}},
	{Name: "сгенерировал", Aliases: []string{"сгенерировала", "нарисовал"},
		From: []string{TypeTool, TypeProject, TypePerson}, To: []string{TypeImage}},
	{Name: "фото_в", Aliases: []string{"снято_в"},
		From: []string{TypeImage}, To: []string{TypePlace}},

	// Document relations: they describe the archive, not the world.
	// "Tells about" is said of a document and of a person: a chapter tells about
	// Сварня, Барон told about Сварня. The verifier flagged the person case
	// against the first version of this rule, which is how the gap was found.
	{Name: "рассказ_о", From: []string{TypeDoc, TypePerson}, To: nil, Document: true},
	// An image illustrates something: the edge runs image -> illustrated, and
	// the illustrated end is open (a document, a project, a place). The
	// direction was declared the other way at first, and because the object end
	// was a single type, type propagation painted every illustrated entity as
	// an image — Cooksy and MATRICA among them.
	{Name: "иллюстрация", From: []string{TypeImage}, To: nil, Document: true},
	{Name: "упоминает", From: []string{TypeImage, TypeDoc, TypePerson, TypeProject}, To: nil, Document: true},
}

// relationIndex maps every canonical name and alias to its spec.
var relationIndex = func() map[string]*RelationSpec {
	idx := make(map[string]*RelationSpec, len(relationVocabulary)*2)
	for i := range relationVocabulary {
		spec := &relationVocabulary[i]
		idx[spec.Name] = spec
		for _, a := range spec.Aliases {
			idx[a] = spec
		}
	}
	return idx
}()

// canonicalRelation maps a relation as written by an extractor to its canonical
// form. The second return value reports whether the relation belongs to the
// vocabulary at all; unknown relations are kept as they are so that no data is
// lost, and show up in relationReport.
func canonicalRelation(relation string) (string, bool) {
	r := strings.TrimSpace(relation)
	if r == "" {
		return r, false
	}
	if spec, ok := relationIndex[r]; ok {
		return spec.Name, true
	}
	if spec, ok := relationIndex[strings.ToLower(r)]; ok {
		return spec.Name, true
	}
	return r, false
}

// relationSpecByName returns the vocabulary entry for a canonical relation.
func relationSpecByName(name string) *RelationSpec {
	return relationIndex[name]
}

// typeAllowed reports whether typ is acceptable at one end of a relation.
// An empty allow list accepts anything, and an empty typ is always accepted
// because an untyped entity must not invalidate an edge on its own.
func typeAllowed(allow []string, typ string) bool {
	if len(allow) == 0 || typ == "" {
		return true
	}
	for _, a := range allow {
		if a == typ {
			return true
		}
	}
	return false
}

// looksLikeImageName reports whether a name is a generated image filename.
func looksLikeImageName(key string) bool {
	if !strings.HasPrefix(key, "img_") {
		return false
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif"} {
		if strings.HasSuffix(key, ext) {
			return true
		}
	}
	return false
}

// looksLikeDocName reports whether a name is a document key rather than a thing
// in the world. Memoir chapters, date-prefixed session notes and essay keys all
// used to become entities and crowded out real ones.
func looksLikeDocName(key string) bool {
	if strings.HasSuffix(key, ".md") {
		return true
	}
	for _, p := range []string{"chapter-", "chapter_", "visual-memory-", "essay-"} {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	// Date-prefixed keys: 2026-07-12-progulka-po-gorodu.
	if len(key) > 11 && key[4] == '-' && key[7] == '-' {
		return true
	}
	return false
}

// inferEntityTypeFromName derives a type from the name alone. It is
// deliberately conservative: only patterns that are unambiguous in this data
// are matched. A wrong type is worse than an empty one, because relations
// validate against types.
func inferEntityTypeFromName(name string) string {
	key := normalizeName(name)
	switch {
	case key == "":
		return ""
	case looksLikeImageName(key):
		return TypeImage
	case looksLikeDocName(key):
		return TypeDoc
	case strings.Contains(key, "@") && strings.Contains(key, "."):
		return TypeOrg
	}
	return ""
}

// GraphAlias maps an alternative spelling to its canonical entity. It is a
// table rather than a JSON column on graph_entities because alias lookup must
// be a single index seek, and because a JSON LIKE scan cannot be indexed.
type GraphAlias struct {
	_        bool   `db_table_name:"graph_aliases"`
	ID       int64  `db:"id" db_key:"primary key autoincrement"`
	AliasKey string `db:"alias_key" db_key:"unique"`
	EntityID int64  `db:"entity_id"`
}

// createAliasTable creates the alias table if it does not exist.
func createAliasTable(db *sql.DB) error {
	if err := sqlh.Create[GraphAlias](db); err != nil {
		return fmt.Errorf("create graph_aliases: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS graph_aliases_entity ON graph_aliases (entity_id)`); err != nil {
		return fmt.Errorf("create graph alias index: %w", err)
	}
	return nil
}

// resolveAliasKey returns the entity id an alias points at.
func resolveAliasKey(db *sql.DB, key string) (int64, bool, error) {
	row, err := sqlh.Get[GraphAlias](db, sqlh.Where{Field: "alias_key=", Value: key})
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("lookup alias %q: %w", key, err)
	}
	return row.EntityID, true, nil
}

// getEntityByName returns an entity by any of its spellings: the canonical name
// or a registered alias. It returns sql.ErrNoRows when nothing matches.
func getEntityByName(db *sql.DB, name string) (*GraphEntity, error) {
	key := normalizeName(name)
	ent, err := sqlh.Get[GraphEntity](db, sqlh.Where{Field: "name_key=", Value: key})
	if err == nil {
		return ent, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("lookup entity %q: %w", name, err)
	}
	id, ok, err := resolveAliasKey(db, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, sql.ErrNoRows
	}
	return sqlh.Get[GraphEntity](db, sqlh.Where{Field: "id=", Value: id})
}

// addEntityAlias registers an alternative spelling for an entity. The entity's
// own name key is never stored as an alias, and an alias already pointing at
// another entity is left alone: merging is an explicit decision, not a
// side effect of seeing a name twice.
func addEntityAlias(db *sql.DB, entityID int64, alias string) error {
	key := normalizeName(alias)
	if key == "" || entityID == 0 {
		return nil
	}
	ent, err := sqlh.Get[GraphEntity](db, sqlh.Where{Field: "id=", Value: entityID})
	if err != nil {
		return fmt.Errorf("lookup entity %d: %w", entityID, err)
	}
	if ent.NameKey == key {
		return nil
	}
	if existing, ok, err := resolveAliasKey(db, key); err != nil {
		return err
	} else if ok {
		if existing == entityID {
			return nil
		}
		return fmt.Errorf("alias %q already points at entity %d", alias, existing)
	}
	if _, err := sqlh.InsertId(db, GraphAlias{AliasKey: key, EntityID: entityID}); err != nil {
		return fmt.Errorf("insert alias %q: %w", alias, err)
	}
	return nil
}

// aliasesForEntity returns the alternative spellings registered for an entity.
func aliasesForEntity(db *sql.DB, entityID int64) ([]string, error) {
	rows, err := db.Query(`SELECT alias_key FROM graph_aliases WHERE entity_id=? ORDER BY alias_key`, entityID)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// mergeEntities folds the drop entity into the keep entity: every edge is
// re-pointed, duplicates created by the merge are removed, and the dropped name
// becomes an alias of the keeper so the old spelling keeps resolving.
//
// The return value is the number of edges that survived; edges that collapsed
// into an existing one are reported in merged.
func mergeEntities(db *sql.DB, keepID, dropID int64) (moved, merged int, err error) {
	if keepID == dropID || keepID == 0 || dropID == 0 {
		return 0, 0, fmt.Errorf("mergeEntities needs two distinct entity ids")
	}
	keep, err := sqlh.Get[GraphEntity](db, sqlh.Where{Field: "id=", Value: keepID})
	if err != nil {
		return 0, 0, fmt.Errorf("lookup keep entity %d: %w", keepID, err)
	}
	drop, err := sqlh.Get[GraphEntity](db, sqlh.Where{Field: "id=", Value: dropID})
	if err != nil {
		return 0, 0, fmt.Errorf("lookup drop entity %d: %w", dropID, err)
	}

	// Re-point subject edges, then object edges. An edge that already exists on
	// the keeper is deleted instead of updated, which the unique constraint on
	// (from_id, relation, to_id, date) would reject anyway.
	for _, side := range []struct {
		column string
		other  string
	}{{"from_id", "to_id"}, {"to_id", "from_id"}} {
		rows, err := db.Query(
			`SELECT id, `+side.other+`, relation, date FROM graph_edges WHERE `+side.column+`=?`, dropID)
		if err != nil {
			return 0, 0, fmt.Errorf("list edges to re-point: %w", err)
		}
		type edgeRef struct {
			id       int64
			other    int64
			relation string
			date     string
		}
		var refs []edgeRef
		for rows.Next() {
			var r edgeRef
			if err := rows.Scan(&r.id, &r.other, &r.relation, &r.date); err != nil {
				rows.Close()
				return 0, 0, fmt.Errorf("scan edge: %w", err)
			}
			refs = append(refs, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, 0, err
		}

		for _, r := range refs {
			var clash int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM graph_edges WHERE `+side.column+`=? AND `+side.other+`=? AND relation=? AND date=?`,
				keepID, r.other, r.relation, r.date).Scan(&clash); err != nil {
				return 0, 0, fmt.Errorf("check duplicate edge: %w", err)
			}
			if clash > 0 {
				if _, err := db.Exec(`DELETE FROM graph_edges WHERE id=?`, r.id); err != nil {
					return 0, 0, fmt.Errorf("delete duplicate edge: %w", err)
				}
				merged++
				continue
			}
			if _, err := db.Exec(
				`UPDATE graph_edges SET `+side.column+`=? WHERE id=?`, keepID, r.id); err != nil {
				return 0, 0, fmt.Errorf("re-point edge: %w", err)
			}
			moved++
		}
	}

	// Re-point aliases, then retire the dropped name into the keeper's aliases.
	if _, err := db.Exec(`UPDATE OR REPLACE graph_aliases SET entity_id=? WHERE entity_id=?`, keepID, dropID); err != nil {
		return 0, 0, fmt.Errorf("re-point aliases: %w", err)
	}
	if err := addEntityAlias(db, keepID, drop.Name); err != nil {
		// The dropped name may already alias a third entity; that is a conflict
		// for a human to resolve, not a reason to abort a completed merge.
		log.Printf("mergeEntities: could not alias %q to %d: %v", drop.Name, keepID, err)
	}
	if err := sqlh.Delete[GraphEntity](db, sqlh.Where{Field: "id=", Value: dropID}); err != nil {
		return moved, merged, fmt.Errorf("delete dropped entity: %w", err)
	}
	log.Printf("graph: merged %q into %q (%d edges moved, %d collapsed)", drop.Name, keep.Name, moved, merged)
	return moved, merged, nil
}

// typeCandidates walks every edge of the vocabulary and collects the types each
// entity is implied to have. An entity that appears as the subject of был_в is
// a person; one that appears as its object is a place. Candidates come from the
// vocabulary, so this needs no seed list of Kirill's family and favourite
// places — the relations already say it.
func typeCandidates(db *sql.DB) (map[int64]map[string]int, error) {
	candidates := map[int64]map[string]int{}
	bump := func(id int64, typ string) {
		if id == 0 || typ == "" {
			return
		}
		if candidates[id] == nil {
			candidates[id] = map[string]int{}
		}
		candidates[id][typ]++
	}

	for _, spec := range relationVocabulary {
		rows, err := db.Query(`SELECT from_id, to_id FROM graph_edges WHERE relation=?`, spec.Name)
		if err != nil {
			return nil, fmt.Errorf("scan relation %q: %w", spec.Name, err)
		}
		for rows.Next() {
			var from, to int64
			if err := rows.Scan(&from, &to); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan edge of %q: %w", spec.Name, err)
			}
			// A single-type allow list is a definite statement; a multi-type
			// list is a guess and is left to a human.
			if len(spec.From) == 1 {
				bump(from, spec.From[0])
			}
			if len(spec.To) == 1 {
				bump(to, spec.To[0])
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

// propagateEntityTypes fills in entity types implied by the relations the
// entities take part in. Types already present are never overwritten, and an
// entity whose evidence is contradictory is reported instead of guessed at.
func propagateEntityTypes(db *sql.DB) (assigned, ambiguous int, err error) {
	candidates, err := typeCandidates(db)
	if err != nil {
		return 0, 0, err
	}
	ids := make([]int64, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		ent, err := sqlh.Get[GraphEntity](db, sqlh.Where{Field: "id=", Value: id})
		if err != nil {
			return assigned, ambiguous, fmt.Errorf("lookup entity %d: %w", id, err)
		}
		if ent.Type != "" {
			continue
		}
		best, bestCount, tie := "", 0, false
		for typ, count := range candidates[id] {
			switch {
			case count > bestCount:
				best, bestCount, tie = typ, count, false
			case count == bestCount:
				tie = true
			}
		}
		if tie || best == "" {
			ambiguous++
			continue
		}
		ent.Type = best
		if err := sqlh.Update(db, sqlh.UpdateAttr[GraphEntity]{
			Row:    *ent,
			Wheres: []sqlh.Where{{Field: "id=", Value: id}},
		}); err != nil {
			return assigned, ambiguous, fmt.Errorf("set type of entity %d: %w", id, err)
		}
		assigned++
	}
	return assigned, ambiguous, nil
}

// relationReport counts the edges per relation and marks the ones outside the
// vocabulary. It is the work queue for extending the vocabulary: a relation with
// one edge is usually a one-off phrasing that should have been canonical.
func relationReport(db *sql.DB) ([]RelationCount, error) {
	rows, err := db.Query(`SELECT relation, COUNT(*) FROM graph_edges GROUP BY relation ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("count relations: %w", err)
	}
	defer rows.Close()

	var out []RelationCount
	for rows.Next() {
		var rc RelationCount
		if err := rows.Scan(&rc.Relation, &rc.Count); err != nil {
			return nil, fmt.Errorf("scan relation count: %w", err)
		}
		canonical, known := canonicalRelation(rc.Relation)
		rc.Canonical = canonical
		rc.InVocabulary = known
		out = append(out, rc)
	}
	return out, rows.Err()
}

// RelationCount is one row of the relation report.
type RelationCount struct {
	Relation     string `json:"relation"`
	Canonical    string `json:"canonical"`
	InVocabulary bool   `json:"in_vocabulary"`
	Count        int    `json:"count"`
}

// normalizeStoredRelations rewrites relation spellings that the vocabulary
// recognises as aliases of a canonical name. Unknown relations are left as they
// are and reported by relationReport. It returns the number of rows changed.
func normalizeStoredRelations(db *sql.DB) (int, error) {
	report, err := relationReport(db)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, rc := range report {
		if rc.Canonical == rc.Relation {
			continue
		}
		res, err := db.Exec(`UPDATE OR REPLACE graph_edges SET relation=? WHERE relation=?`,
			rc.Canonical, rc.Relation)
		if err != nil {
			return changed, fmt.Errorf("normalize relation %q: %w", rc.Relation, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed += int(n)
		}
	}
	return changed, nil
}

// mergeDuplicateDocs folds entities whose names differ only by a document
// suffix (chapter-08-baron-sees vs chapter-08-baron-sees.md) into one node.
// It returns the number of entities removed.
func mergeDuplicateDocs(db *sql.DB) (int, error) {
	pairs, err := duplicateDocPairs(db)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, p := range pairs {
		keep, err := getEntityByName(db, p[0])
		if err != nil {
			continue
		}
		drop, err := getEntityByName(db, p[1])
		if err != nil {
			continue
		}
		if keep.ID == drop.ID {
			continue
		}
		if _, _, err := mergeEntities(db, keep.ID, drop.ID); err != nil {
			return removed, fmt.Errorf("merge %q into %q: %w", drop.Name, keep.Name, err)
		}
		removed++
	}
	return removed, nil
}

// entityTypeFix is a pending type assignment.
type entityTypeFix struct {
	ID   int64
	Name string
	Type string
}

// patternTypeFixes returns the entities whose name matches an unambiguous
// pattern (image filename, document key, e-mail address) and that have no type
// yet. It is split out from applyPatternTypes so that a dry run can report the
// same number a real run would write.
func patternTypeFixes(db *sql.DB) ([]entityTypeFix, error) {
	rows, err := db.Query(`SELECT id, name, type FROM graph_entities`)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}
	defer rows.Close()

	var todo []entityTypeFix
	for rows.Next() {
		var id int64
		var name, typ string
		if err := rows.Scan(&id, &name, &typ); err != nil {
			return nil, fmt.Errorf("scan entity: %w", err)
		}
		if typ != "" {
			continue
		}
		if inferred := inferEntityTypeFromName(name); inferred != "" {
			todo = append(todo, entityTypeFix{ID: id, Name: name, Type: inferred})
		}
	}
	return todo, rows.Err()
}

// resetDerivedTypes clears the types that no relation justifies, so
// propagateEntityTypes can derive them again.
//
// A type, once set, is never overwritten, which is what keeps inference from
// flip-flopping. The cost is that a type derived from a wrong rule stays wrong
// forever: when иллюстрация pointed the other way, propagation painted the
// illustrated entity as an image and nothing would have corrected it.
//
// The test is "no edge justifies this type", and justification means a
// relation that states the type positively — a non-empty allow list that
// contains it. An open end (упоминает connects to anything) is silence, not
// justification: an entity mentioned by an image is not thereby an image, and
// treating that as support would have left Cooksy mistyped.
//
// Clearing everything would be simpler and wrong. Not every type comes from a
// relation: the deterministic extractors state one when they create an entity
// (a mail sender is an organisation), and a type with no edge against it must
// survive.
func resetDerivedTypes(db *sql.DB) (int, error) {
	rows, err := db.Query(`
		SELECT f.id, f.name, f.type, t.type, e.relation, 1 AS from_side
		  FROM graph_edges e
		  JOIN graph_entities f ON f.id = e.from_id
		  JOIN graph_entities t ON t.id = e.to_id
		 WHERE f.type <> ''
		UNION ALL
		SELECT t.id, t.name, t.type, f.type, e.relation, 0
		  FROM graph_edges e
		  JOIN graph_entities f ON f.id = e.from_id
		  JOIN graph_entities t ON t.id = e.to_id
		 WHERE t.type <> ''`)
	if err != nil {
		return 0, fmt.Errorf("load typed edges: %w", err)
	}
	defer rows.Close()

	// justified[id] is set when some edge states the stored type positively.
	justified := map[int64]bool{}
	// seen[id] marks entities that take part in at least one edge: an entity
	// with no edges cannot be contradicted by one.
	seen := map[int64]bool{}
	names := map[int64]string{}
	stored := map[int64]string{}

	for rows.Next() {
		var id int64
		var name, typ, otherType, relation string
		var fromSide int
		if err := rows.Scan(&id, &name, &typ, &otherType, &relation, &fromSide); err != nil {
			return 0, fmt.Errorf("scan typed edge: %w", err)
		}
		names[id], stored[id], seen[id] = name, typ, true

		spec := relationSpecByName(relation)
		if spec == nil {
			continue
		}
		allow := spec.To
		if fromSide == 1 {
			allow = spec.From
		}
		if len(allow) > 0 && typeAllowed(allow, typ) {
			justified[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	cleared := 0
	for id := range seen {
		if justified[id] || inferEntityTypeFromName(names[id]) != "" {
			continue
		}
		if _, err := db.Exec(`UPDATE graph_entities SET type='' WHERE id=?`, id); err != nil {
			return cleared, fmt.Errorf("clear type of %q: %w", names[id], err)
		}
		cleared++
	}
	return cleared, nil
}

// applyPatternTypes sets the type of every entity whose name matches an
// unambiguous pattern. Existing types are never overwritten, so this is safe to
// re-run and safe to combine with propagateEntityTypes.
func applyPatternTypes(db *sql.DB) (int, error) {
	todo, err := patternTypeFixes(db)
	if err != nil {
		return 0, err
	}
	for _, p := range todo {
		if _, err := db.Exec(`UPDATE graph_entities SET type=? WHERE id=?`, p.Type, p.ID); err != nil {
			return 0, fmt.Errorf("set type of %q: %w", p.Name, err)
		}
	}
	return len(todo), nil
}

// duplicateDocPairs returns the entity pairs that differ only by a document
// suffix, as (keep, drop) name pairs. It is split out from mergeDuplicateDocs so
// that a dry run reports the same number a real run would merge.
func duplicateDocPairs(db *sql.DB) ([][2]string, error) {
	rows, err := db.Query(`SELECT name_key FROM graph_entities`)
	if err != nil {
		return nil, fmt.Errorf("list entity keys: %w", err)
	}
	defer rows.Close()

	keys := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan entity key: %w", err)
		}
		keys[k] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var pairs [][2]string
	for k := range keys {
		if !strings.HasSuffix(k, ".md") {
			continue
		}
		if bare := strings.TrimSuffix(k, ".md"); keys[bare] {
			pairs = append(pairs, [2]string{bare, k})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][1] < pairs[j][1] })
	return pairs, nil
}

// graphOverview summarises the graph for a report: entity and edge counts, the
// distribution of types, and the relations outside the vocabulary.
type graphOverview struct {
	Entities        int             `json:"entities"`
	Edges           int             `json:"edges"`
	Types           map[string]int  `json:"types"`
	Relations       []RelationCount `json:"relations"`
	UntypedEntities int             `json:"untyped_entities"`
}

// overview collects the summary used by graph_repair and graph_relations.
func graphOverviewOf(db *sql.DB) (*graphOverview, error) {
	entities, edges, err := graphStats(db)
	if err != nil {
		return nil, err
	}
	ov := &graphOverview{Entities: entities, Edges: edges, Types: map[string]int{}}

	rows, err := db.Query(`SELECT COALESCE(NULLIF(type,''),'(none)') AS t, COUNT(*) FROM graph_entities GROUP BY t ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("count types: %w", err)
	}
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			rows.Close()
			return nil, err
		}
		ov.Types[t] = n
		if t == "(none)" {
			ov.UntypedEntities = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if ov.Relations, err = relationReport(db); err != nil {
		return nil, err
	}
	return ov, nil
}
