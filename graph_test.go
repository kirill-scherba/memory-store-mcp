// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func TestGraphSchemaAndEdges(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	// Both tables must exist.
	for _, table := range []string{"graph_entities", "graph_edges"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&n); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if n != 1 {
			t.Fatalf("table %s was not created", table)
		}
	}

	// Indexes on from_id and to_id must exist.
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='graph_edges'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var indexes []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		indexes = append(indexes, name)
	}
	t.Logf("graph_edges indexes: %v", indexes)
}

func TestAddEdgeIsIdempotentAndCaseInsensitive(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	if err := addGraphEdge(db, "Кирилл", "Сварня", "был_в", "2026-07-23", "test"); err != nil {
		t.Fatalf("addGraphEdge: %v", err)
	}
	// Same edge, different case, different whitespace: must not duplicate.
	if err := addGraphEdge(db, " кирилл ", "СВАРНЯ", "был_в", "2026-07-23", "test"); err != nil {
		t.Fatalf("addGraphEdge (variant): %v", err)
	}

	entities, edges, err := graphStats(db)
	if err != nil {
		t.Fatal(err)
	}
	if entities != 2 {
		t.Fatalf("entities = %d, want 2 (case-insensitive merge failed)", entities)
	}
	if edges != 1 {
		t.Fatalf("edges = %d, want 1 (duplicate inserted)", edges)
	}

	// A different date is a different occurrence and must be kept.
	if err := addGraphEdge(db, "Кирилл", "Сварня", "был_в", "2026-08-12", "test"); err != nil {
		t.Fatal(err)
	}
	if _, edges, _ = graphStats(db); edges != 2 {
		t.Fatalf("edges = %d, want 2", edges)
	}
}

func TestEdgesForEntity(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	for _, e := range []struct{ from, to, rel, date string }{
		{"Кирилл", "Сварня", "был_в", "2026-07-23"},
		{"Барон", "Сварня", "был_в", "2026-07-25"},
		{"Сварня", "плесковица", "заказал", "2026-07-23"},
	} {
		if err := addGraphEdge(db, e.from, e.to, e.rel, e.date, "test"); err != nil {
			t.Fatal(err)
		}
	}

	// Both directions, case-insensitive.
	edges, err := edgesForEntity(db, "сварня")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 3 {
		t.Fatalf("edges for сварня = %d, want 3: %+v", len(edges), edges)
	}

	edges, err = edgesForEntity(db, "КИРИЛЛ")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("edges for КИРИЛЛ = %d, want 1", len(edges))
	}
	if edges[0].FromName != "Кирилл" || edges[0].ToName != "Сварня" {
		t.Fatalf("unexpected edge: %+v", edges[0])
	}

	if edges, _ := edgesForEntity(db, "нет-такого"); len(edges) != 0 {
		t.Fatalf("expected no edges, got %+v", edges)
	}
}

// TestGraphEdgesJSONShape guards the field names the gallery reads. Without
// json tags the struct marshals as FromName/ToName/Relation and the gallery's
// viewer renders nothing.
func TestGraphEdgesJSONShape(t *testing.T) {
	store := newTestStorage(t)

	if err := addGraphEdge(store.goals, "img_1.png", "Сварня", "фото_в", "2026-07-25", "test"); err != nil {
		t.Fatal(err)
	}
	edges, err := edgesForEntity(store.goals, "img_1.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(edges)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"from"`, `"to"`, `"relation"`, `"date"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("edge JSON is missing %s: %s", field, data)
		}
	}
}

func TestNeighborsDepth(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	// Кирилл -> Сварня -> плесковица ; Кирилл -> Тбилиси
	for _, e := range []struct{ from, to, rel string }{
		{"Кирилл", "Сварня", "был_в"},
		{"Сварня", "плесковица", "заказал"},
		{"Кирилл", "Тбилиси", "был_в"},
	} {
		if err := addGraphEdge(db, e.from, e.to, e.rel, "2026-07-23", "test"); err != nil {
			t.Fatal(err)
		}
	}

	// Depth 1: direct neighbours only.
	n1, err := neighborsForEntity(db, "Кирилл", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(n1) != 2 {
		t.Fatalf("depth 1 = %d neighbours, want 2: %+v", len(n1), n1)
	}

	// Depth 2: плесковица becomes reachable.
	n2, err := neighborsForEntity(db, "Кирилл", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(n2) != 3 {
		t.Fatalf("depth 2 = %d neighbours, want 3: %+v", len(n2), n2)
	}
	found := false
	for _, nb := range n2 {
		if nb.Name == "плесковица" {
			found = true
			if nb.Depth != 2 {
				t.Fatalf("плесковица depth = %d, want 2", nb.Depth)
			}
		}
	}
	if !found {
		t.Fatal("плесковица not reachable at depth 2")
	}
}

func TestMigrateLegacyGraph(t *testing.T) {
	store := newTestStorage(t)

	// Simulate the old storage format: memory/graph/ entries in the KV store.
	legacy := []struct{ key, content string }{
		{"memory/graph/2026-07-23/Кирилл-Сварня-был_в", `{"from":"Кирилл","to":"Сварня","relation":"был_в","date":"2026-07-23"}`},
		{"memory/graph/edge/Сварня-плесковица-заказал", `{"from":"Сварня","to":"плесковица","relation":"заказал","date":"2026-07-23"}`},
		{"memory/graph/2026-07-25-Кирилл-Сварня-был_в", `{"from":"Кирилл","to":"Сварня","relation":"был_в","date":"2026-07-25"}`},
	}
	for _, l := range legacy {
		// Legacy entries were MemoryValue-wrapped; write straight to the KV
		// store so the test does not depend on Ollama.
		val, err := json.Marshal(MemoryValue{Content: l.content})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.kv.Set(l.key, val); err != nil {
			t.Fatalf("seed legacy %s: %v", l.key, err)
		}
	}

	n, err := migrateLegacyGraph(store)
	if err != nil {
		t.Fatalf("migrateLegacyGraph: %v", err)
	}
	if n != 3 {
		t.Fatalf("migrated %d, want 3", n)
	}

	_, edges, err := graphStats(store.goals)
	if err != nil {
		t.Fatal(err)
	}
	if edges != 3 {
		t.Fatalf("graph has %d edges, want 3", edges)
	}

	// The legacy keys must be gone.
	var left int
	if err := store.goals.QueryRow(
		`SELECT COUNT(*) FROM kv_data WHERE key LIKE 'memory/graph/%'`,
	).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d legacy graph keys left in kv_data", left)
	}

	// Re-running is a no-op.
	if n, err := migrateLegacyGraph(store); err != nil || n != 0 {
		t.Fatalf("second migration: n=%d err=%v", n, err)
	}
}

var _ = sql.ErrNoRows

// TestGraphContextForText covers entity detection in free text and the
// compact rendering used for context injection.
func TestGraphContextForText(t *testing.T) {
	store := newTestStorage(t)

	for _, e := range []struct{ from, to, rel string }{
		{"Сварня", "плесковица", "заказал"},
		{"Кирилл", "Сварня", "был_в"},
		{"Барон", "Сварня", "был_в"},
	} {
		if err := addGraphEdge(store.goals, e.from, e.to, e.rel, "2026-07-23", "test"); err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.graphContextForText("сегодня были в Сварне и заказали плесковицу", 5)
	if err != nil {
		t.Fatalf("graphContextForText: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no entities detected in text")
	}
	if items[0].Entity != "Сварня" {
		t.Fatalf("first entity = %q, want Сварня (longest match first)", items[0].Entity)
	}

	section := formatGraphContext(items)
	for _, want := range []string{"Graph context", "Сварня", "заказал", "плесковица", "Кирилл"} {
		if !strings.Contains(section, want) {
			t.Fatalf("graph section missing %q:\n%s", want, section)
		}
	}
	t.Logf("graph section:\n%s", section)

	// No entities in the text -> no section.
	if items, _ := store.graphContextForText("совершенно посторонний текст", 5); len(items) != 0 {
		t.Fatalf("unexpected entities: %+v", items)
	}
}

// TestGraphContextInjection verifies the graph reaches memory_get_context
// without the agent calling any graph tool.
func TestGraphContextInjection(t *testing.T) {
	store := newTestStorage(t)

	if err := addGraphEdge(store.goals, "Сварня", "плесковица", "заказал", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save("memory/test/svarnya",
		&MemoryValue{Content: "Были в Сварне, заказали плесковицу"},
		"Сварня вечер, заказали плесковицу", false); err != nil {
		t.Fatal(err)
	}

	injection, err := store.GetContextForInjection("Сварня", 5)
	if err != nil {
		t.Fatalf("GetContextForInjection: %v", err)
	}
	if !strings.Contains(injection, "Graph context") {
		t.Fatalf("injection has no graph section:\n%s", injection)
	}
	if !strings.Contains(injection, "плесковица") {
		t.Fatalf("injection has no graph edge:\n%s", injection)
	}
}

// TestExtractImageEdges covers the automatic image -> entity linking.
func TestExtractImageEdges(t *testing.T) {
	store := newTestStorage(t)

	// An existing entity that the image metadata mentions.
	if err := addGraphEdge(store.goals, "Сварня", "плесковица", "заказал", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}

	const text = "Верблюд в поварской шапке на ярмарке, Сварня, хинкали"
	n, err := store.extractImageEdges("img_1.png", text, 10)
	if err != nil {
		t.Fatalf("extractImageEdges: %v", err)
	}
	if n == 0 {
		t.Fatal("no edges added")
	}

	// Idempotent: a second pass adds nothing.
	if n, err := store.extractImageEdges("img_1.png", text, 10); err != nil || n != 0 {
		t.Fatalf("second pass added %d edges (err=%v), want 0", n, err)
	}

	edges, err := edgesForEntity(store.goals, "img_1.png")
	if err != nil {
		t.Fatal(err)
	}
	linked := false
	for _, e := range edges {
		if e.FromName == "Сварня" || e.ToName == "Сварня" {
			linked = true
		}
	}
	if !linked {
		t.Fatalf("image not linked to Сварня: %+v", edges)
	}
}

// TestBackfillImageGraph walks stored gallery metadata and derives edges.
func TestBackfillImageGraph(t *testing.T) {
	store := newTestStorage(t)

	if err := addGraphEdge(store.goals, "Сварня", "плесковица", "заказал", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}

	val, err := json.Marshal(MemoryValue{
		Content: `{"description":"Верблюд на ярмарке, Сварня","tags":"сварня,верблюд"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.kv.Set("memory/gallery/meta/img_9.png", val); err != nil {
		t.Fatal(err)
	}

	n, err := store.backfillImageGraph()
	if err != nil {
		t.Fatalf("backfillImageGraph: %v", err)
	}
	if n == 0 {
		t.Fatal("backfill added no edges")
	}
}

// TestSaveExtractedTriples covers the graph edges produced by memory_extract.
func TestSaveExtractedTriples(t *testing.T) {
	store := newTestStorage(t)

	n := store.saveExtractedTriples([]GraphTriple{
		{From: "Кирилл", Relation: "был_в", To: "Сварня", Date: "2026-09-13"},
		{From: "", Relation: "мусор", To: "мусор"},                              // incomplete: skipped
		{From: "Кирилл", Relation: "был_в", To: "Сварня", Date: "2026-09-13"},   // duplicate
	})
	if n != 2 {
		t.Fatalf("processed %d triples, want 2 (one incomplete skipped)", n)
	}

	_, edges, err := graphStats(store.goals)
	if err != nil {
		t.Fatal(err)
	}
	if edges != 1 {
		t.Fatalf("graph has %d edges, want 1 (duplicate deduplicated)", edges)
	}

	found, err := edgesForEntity(store.goals, "Сварня")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Relation != "был_в" {
		t.Fatalf("unexpected edges: %+v", found)
	}
}
