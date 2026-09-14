// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
)

func TestInferEntityTypeFromName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"img_1784820835.png", TypeImage},
		{"img_1784820835.JPG", TypeImage},
		{"chapter-08-baron-sees", TypeDoc},
		{"chapter-08-baron-sees.md", TypeDoc},
		{"2026-07-12-progulka-po-gorodu", TypeDoc},
		{"visual-memory-essay-01", TypeDoc},
		{"baron@matrica.work", TypeOrg},
		// Real entities must stay untyped: a wrong type is worse than none,
		// because relations validate against types.
		{"Кирилл", ""},
		{"Сварня", ""},
		{"хинкали", ""},
		{"MCP", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := inferEntityTypeFromName(c.name); got != c.want {
			t.Errorf("inferEntityTypeFromName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCanonicalRelation(t *testing.T) {
	cases := []struct {
		in       string
		want     string
		inVocab  bool
	}{
		{"заказала", "заказал", true},
		{"заказал", "заказал", true},
		{" Заказали ", "заказал", true},
		{"был", "был_в", true},
		{"поехал_в", "был_в", true},
		{"жена", "жена", true},
		// A one-off relation is kept as written and reported as vocabulary work.
		{"путает_имя", "путает_имя", false},
		{"проводил_в_аэропорт", "проводил_в_аэропорт", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := canonicalRelation(c.in)
		if got != c.want || ok != c.inVocab {
			t.Errorf("canonicalRelation(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.inVocab)
		}
	}
}

func TestAddEdgeFoldsRelationSpelling(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	if err := addGraphEdge(db, "Кирилл", "хинкали", "заказала", "2026-07-23", "test"); err != nil {
		t.Fatalf("addGraphEdge: %v", err)
	}
	if err := addGraphEdge(db, "Кирилл", "хинкали", "заказал", "2026-07-23", "test"); err != nil {
		t.Fatalf("addGraphEdge (canonical): %v", err)
	}

	// Both spellings must collapse into one canonical edge, not two.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE relation='заказал'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d заказал edges, want 1", n)
	}
	var other int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE relation='заказала'`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if other != 0 {
		t.Fatalf("got %d заказала edges, want 0", other)
	}
}

func TestAliasResolvesToCanonicalEntity(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	id, err := resolveEntityID(db, "Кирилл", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := addEntityAlias(db, id, "Kirill"); err != nil {
		t.Fatalf("addEntityAlias: %v", err)
	}

	// The Latin spelling must resolve to the same node instead of creating one.
	got, err := resolveEntityID(db, "kirill", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("resolveEntityID(kirill) = %d, want %d", got, id)
	}

	entities, _, err := graphStats(db)
	if err != nil {
		t.Fatal(err)
	}
	if entities != 1 {
		t.Fatalf("got %d entities, want 1", entities)
	}

	aliases, err := aliasesForEntity(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 || aliases[0] != "kirill" {
		t.Fatalf("aliases = %v, want [kirill]", aliases)
	}
}

func TestMergeEntitiesMovesEdgesAndAliases(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	keepID, err := resolveEntityID(db, "Кирилл", TypePerson)
	if err != nil {
		t.Fatal(err)
	}
	dropID, err := resolveEntityID(db, "Kirill", TypePerson)
	if err != nil {
		t.Fatal(err)
	}
	if keepID == dropID {
		t.Fatal("test needs two distinct entities")
	}

	// The dropped entity carries an edge of its own, plus one that duplicates
	// an edge the keeper already has.
	if err := addGraphEdge(db, "Kirill", "Сварня", "был_в", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}
	if err := addGraphEdge(db, "Кирилл", "Сварня", "был_в", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}
	if err := addGraphEdge(db, "Kirill", "Алкон", "был_в", "2026-07-24", "test"); err != nil {
		t.Fatal(err)
	}

	moved, merged, err := mergeEntities(db, keepID, dropID)
	if err != nil {
		t.Fatalf("mergeEntities: %v", err)
	}
	if moved != 1 || merged != 1 {
		t.Fatalf("moved=%d merged=%d, want 1 and 1", moved, merged)
	}

	// The duplicate must be gone, the unique edge must now hang off the keeper.
	var edges int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE from_id=?`, keepID).Scan(&edges); err != nil {
		t.Fatal(err)
	}
	if edges != 2 {
		t.Fatalf("keeper has %d edges, want 2", edges)
	}
	var orphans int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE from_id=? OR to_id=?`, dropID, dropID).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Fatalf("%d edges still reference the dropped entity", orphans)
	}

	// The dropped spelling must keep resolving, now as an alias.
	got, err := resolveEntityID(db, "Kirill", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != keepID {
		t.Fatalf("Kirill resolves to %d, want %d", got, keepID)
	}
}

func TestMergeDuplicateDocs(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	if err := addGraphEdge(db, "chapter-08-baron-sees.md", "Сварня", "рассказ_о", "2026-07-25", "test"); err != nil {
		t.Fatal(err)
	}
	if err := addGraphEdge(db, "chapter-08-baron-sees", "Сварня", "рассказ_о", "2026-07-25", "test"); err != nil {
		t.Fatal(err)
	}

	removed, err := mergeDuplicateDocs(db)
	if err != nil {
		t.Fatalf("mergeDuplicateDocs: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d entities, want 1", removed)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_entities WHERE name_key='chapter-08-baron-sees.md'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("the .md entity survived the merge")
	}
	// The duplicate edge collapsed instead of doubling.
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE relation='рассказ_о'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d рассказ_о edges, want 1", n)
	}
}

func TestPropagateEntityTypes(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	// был_в is person -> place, so the relation itself tells us the types.
	if err := addGraphEdge(db, "Кирилл", "Сварня", "был_в", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}

	assigned, _, err := propagateEntityTypes(db)
	if err != nil {
		t.Fatalf("propagateEntityTypes: %v", err)
	}
	if assigned != 2 {
		t.Fatalf("assigned %d types, want 2", assigned)
	}

	for name, want := range map[string]string{"Кирилл": TypePerson, "Сварня": TypePlace} {
		ent, err := getEntityByName(db, name)
		if err != nil {
			t.Fatal(err)
		}
		if ent.Type != want {
			t.Errorf("%s has type %q, want %q", name, ent.Type, want)
		}
	}
}

// TestPropagateEntityTypesKeepsPatternTypes guards the rule that inference never
// overwrites a type that a name pattern already established.
func TestPropagateEntityTypesKeepsPatternTypes(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	// An image that is the subject of рассказ_о would otherwise be inferred as
	// a document, because рассказ_о is doc -> any.
	if err := addGraphEdge(db, "img_1784820835.png", "Сварня", "рассказ_о", "2026-07-25", "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := propagateEntityTypes(db); err != nil {
		t.Fatal(err)
	}
	ent, err := getEntityByName(db, "img_1784820835.png")
	if err != nil {
		t.Fatal(err)
	}
	if ent.Type != TypeImage {
		t.Fatalf("image type = %q, want %q", ent.Type, TypeImage)
	}
}

// TestEdgesForEntityFollowsAlias guards a gap found on production data: after a
// merge, querying by the retired spelling returned nothing, because the edge
// lookup matched name_key directly and never consulted the alias registry.
func TestEdgesForEntityFollowsAlias(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	if err := addGraphEdge(db, "chapter-08-baron-sees", "Сварня", "рассказ_о", "2026-07-25", "test"); err != nil {
		t.Fatal(err)
	}
	if err := addGraphEdge(db, "chapter-08-baron-sees.md", "Сварня", "рассказ_о", "2026-07-25", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeDuplicateDocs(db); err != nil {
		t.Fatal(err)
	}

	edges, err := edgesForEntity(db, "chapter-08-baron-sees.md")
	if err != nil {
		t.Fatalf("edgesForEntity by retired spelling: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d edges by alias, want 1", len(edges))
	}

	neighbours, err := neighborsForEntity(db, "chapter-08-baron-sees.md", 1)
	if err != nil {
		t.Fatalf("neighborsForEntity by retired spelling: %v", err)
	}
	if len(neighbours) != 1 || neighbours[0].Name != "Сварня" {
		t.Fatalf("neighbours by alias = %v, want [Сварня]", neighbours)
	}
}
