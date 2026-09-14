// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

// visitEntry is the shape of memory/user/kirill/food/svarnya-2026-07-15.
func visitEntry(content, ts string) *MemoryValue {
	return &MemoryValue{Content: content, Timestamp: ts}
}

func TestExtractPlaceVisit(t *testing.T) {
	value := visitEntry(
		`{"date":"2026-07-15","place":"Сварня","with":"Теона","order":"пиво, крылышки","context":"Кирилл и Теона пошли в Сварню вечером"}`,
		"2026-07-15T18:28:56Z")

	triples := extractPlaceVisit("memory/user/kirill/food/svarnya-2026-07-15", value)
	// был_в, был_с, plus заказал and подают_в for each of the two dishes.
	if len(triples) != 6 {
		t.Fatalf("got %d triples, want 6: %+v", len(triples), triples)
	}

	want := map[string]string{
		"Кирилл|Сварня|был_в":      "2026-07-15",
		"Кирилл|Теона|был_с":       "2026-07-15",
		"Кирилл|пиво|заказал":      "2026-07-15",
		"Кирилл|крылышки|заказал":  "2026-07-15",
		"пиво|Сварня|подают_в":     "2026-07-15",
		"крылышки|Сварня|подают_в": "2026-07-15",
	}
	got := map[string]string{}
	for _, tr := range triples {
		got[tr.From+"|"+tr.To+"|"+tr.Relation] = tr.Date
	}
	for k, wantDate := range want {
		if got[k] != wantDate {
			t.Errorf("triple %q has date %q, want %q (all: %v)", k, got[k], wantDate, got)
		}
	}
}

func TestExtractPlaceVisitFallsBackToEntryDate(t *testing.T) {
	// No date field: the entry's own timestamp is the day it is about.
	value := visitEntry(`{"place":"Сварня","type":"regular spot"}`, "2026-07-15T19:34:07Z")
	triples := extractPlaceVisit("memory/user/kirill/food/svarnya-regular", value)
	if len(triples) != 1 {
		t.Fatalf("got %d triples, want 1: %+v", len(triples), triples)
	}
	if triples[0].Relation != "был_в" || triples[0].Date != "2026-07-15" {
		t.Fatalf("got %+v, want был_в on 2026-07-15", triples[0])
	}
}

func TestExtractFoodRegistry(t *testing.T) {
	value := visitEntry(`{"rule":"после каждого разговора о еде сохранять визит","places":[
		{"name":"Сварня","address":"Усиевича 12","dishes":["жареха","рубиновое пиво"],"visits":["2026-06-29","2026-07-23"]},
		{"name":"Алкон","dishes":["суши"],"visits":["2026-07-13"]}
	]}`, "2026-08-12T16:49:01Z")

	triples := extractFoodRegistry("memory/user/kirill/food/places", value)

	var visits, served int
	for _, tr := range triples {
		switch tr.Relation {
		case "был_в":
			visits++
		case "подают_в":
			served++
		}
	}
	if visits != 3 {
		t.Errorf("got %d visit triples, want 3", visits)
	}
	if served != 3 {
		t.Errorf("got %d served-at triples, want 3", served)
	}
	// The dates must come from the registry, not from the entry timestamp.
	for _, tr := range triples {
		if tr.Relation == "был_в" && tr.Date == "2026-08-12" {
			t.Errorf("visit used the entry date instead of the registry date: %+v", tr)
		}
	}
}

func TestExtractPersonRelation(t *testing.T) {
	value := visitEntry(`{"name":"Эка","relation":"жена Кирилла","origin":"Москва"}`, "2026-07-31T18:28:01Z")
	triples := extractPersonRelation("memory/user/kirill/family/eka", value)
	if len(triples) != 1 {
		t.Fatalf("got %d triples, want 1: %+v", len(triples), triples)
	}
	if triples[0].From != "Эка" || triples[0].To != "Кирилл" || triples[0].Relation != "жена" {
		t.Fatalf("got %+v, want Эка -жена-> Кирилл", triples[0])
	}
}

func TestExtractMailSender(t *testing.T) {
	value := visitEntry(
		`{"type":"payment","sender":"Т-Банк <inform@emails.tinkoff.ru>","subject":"Уведомление о новом счете ЖКУ Москва"}`,
		"2026-09-14T06:29:49Z")
	triples := extractMailSender("memory/mail/important/2026-09-14/t-bank-zhku", value)
	if len(triples) != 1 {
		t.Fatalf("got %d triples, want 1: %+v", len(triples), triples)
	}
	if triples[0].From != "Т-Банк" || triples[0].Relation != "направил" {
		t.Fatalf("got %+v, want Т-Банк -направил-> Кирилл", triples[0])
	}
	// The address must not become an entity.
	if triples[0].From == "inform@emails.tinkoff.ru" {
		t.Fatal("the e-mail address leaked into the graph as an entity")
	}
}

// TestExtractorsIgnoreProse guards the rule that an extractor never guesses:
// prose entries and unrelated documents must produce nothing at all.
func TestExtractorsIgnoreProse(t *testing.T) {
	cases := map[string]*MemoryValue{
		"prose":      visitEntry("Kirill's people: Eka wife, Teona daughter", "2026-09-14T10:00:00Z"),
		"empty":      visitEntry("", "2026-09-14T10:00:00Z"),
		"other-json": visitEntry(`{"tags":["memory","graph"],"status":"done"}`, "2026-09-14T10:00:00Z"),
	}
	for name, value := range cases {
		if got := triplesForEntry("memory/project/cooksy/architecture", value); len(got) != 0 {
			t.Errorf("%s: extractors produced %+v, want nothing", name, got)
		}
	}
}

// TestExtractorsRunOnSave is the "graph grows by itself" contract: saving a
// structured entry must add edges without anyone calling graph_add_edge.
func TestExtractorsRunOnSave(t *testing.T) {
	store := newTestStorage(t)

	value := &MemoryValue{
		Content:   `{"date":"2026-08-12","place":"Сварня","with":"Теона","order":"пиво"}`,
		Timestamp: "2026-08-12T20:00:00Z",
	}
	if _, err := store.Save("memory/user/kirill/food/svarnya-2026-08-12", value, "visit", false); err != nil {
		t.Fatalf("Save: %v", err)
	}

	edges, err := edgesForEntity(store.goals, "Сварня")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) == 0 {
		t.Fatal("saving a visit entry produced no edges; the save hook is not wired")
	}
	var sawVisit bool
	for _, e := range edges {
		if e.Relation == "был_в" && e.FromName == "Кирилл" {
			sawVisit = true
		}
	}
	if !sawVisit {
		t.Fatalf("edges for Сварня = %+v, want a был_в edge", edges)
	}

	// The companion edge connects two people, so it hangs off Теона, not off
	// the place.
	companionEdges, err := edgesForEntity(store.goals, "Теона")
	if err != nil {
		t.Fatal(err)
	}
	var sawCompanion bool
	for _, e := range companionEdges {
		if e.Relation == "был_с" && e.FromName == "Кирилл" {
			sawCompanion = true
		}
	}
	if !sawCompanion {
		t.Fatalf("edges for Теона = %+v, want a был_с edge", companionEdges)
	}
}

func TestBackfillDeterministicGraph(t *testing.T) {
	store := newTestStorage(t)

	entries := map[string]string{
		"memory/user/kirill/food/svarnya-2026-07-15": `{"date":"2026-07-15","place":"Сварня","with":"Теона"}`,
		"memory/user/kirill/food/places":             `{"places":[{"name":"Алкон","dishes":["суши"],"visits":["2026-07-13"]}]}`,
		"memory/user/kirill/family/eka":              `{"name":"Эка","relation":"жена Кирилла"}`,
	}
	for key, content := range entries {
		if _, err := store.saveWithKey(key, &MemoryValue{Content: content, Timestamp: "2026-07-15T18:00:00Z"}, ""); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}

	// A second pass must not duplicate anything.
	first, err := store.backfillDeterministicGraph()
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if first.EntriesMatched != 3 {
		t.Fatalf("matched %d entries, want 3 (%+v)", first.EntriesMatched, first)
	}
	_, edgesAfterFirst, err := graphStats(store.goals)
	if err != nil {
		t.Fatal(err)
	}

	second, err := store.backfillDeterministicGraph()
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if second.Triples != first.Triples {
		t.Errorf("second pass reported %d triples, first reported %d", second.Triples, first.Triples)
	}
	_, edgesAfterSecond, err := graphStats(store.goals)
	if err != nil {
		t.Fatal(err)
	}
	if edgesAfterSecond != edgesAfterFirst {
		t.Fatalf("backfill is not idempotent: %d edges then %d", edgesAfterFirst, edgesAfterSecond)
	}
}

func TestSenderDisplayName(t *testing.T) {
	cases := map[string]string{
		"Т-Банк <inform@emails.tinkoff.ru>": "Т-Банк",
		"GitHub <noreply@github.com>":       "GitHub",
		"plain@example.com":                 "plain@example.com",
	}
	for in, want := range cases {
		if got := senderDisplayName(in); got != want {
			t.Errorf("senderDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExtractionPromptCoversVocabulary guards against drift: the prompt and the
// vocabulary are the same contract, so a relation added to one must appear in
// the other. The previous prompt listed examples with a trailing "...", which
// is how the graph collected one-off relations nobody could query.
func TestExtractionPromptCoversVocabulary(t *testing.T) {
	prompt := extractSystemPrompt()
	for _, spec := range relationVocabulary {
		if spec.Document {
			// The extractor records facts about the world; document relations
			// describe the archive and must not be offered to the model.
			if strings.Contains(prompt, "     "+spec.Name+"  (") {
				t.Errorf("document relation %q leaked into the extraction prompt", spec.Name)
			}
			continue
		}
		if !strings.Contains(prompt, "     "+spec.Name+"  (") {
			t.Errorf("relation %q is in the vocabulary but missing from the extraction prompt", spec.Name)
		}
	}
}

// TestSaveExtractedTriplesReportsOutsiders checks that a relation the model
// invented is stored but counted, so the vocabulary can grow deliberately.
func TestSaveExtractedTriplesReportsOutsiders(t *testing.T) {
	store := newTestStorage(t)

	triples := []GraphTriple{
		{From: "Кирилл", Relation: "был_в", To: "Сварня", Date: "2026-07-23"},
		{From: "Кирилл", Relation: "заказала", To: "хинкали", Date: "2026-07-23"}, // alias, folds
		{From: "Баг", Relation: "путает_имя", To: "Кирилл", Date: "2026-07-23"},   // invented
		{From: "", Relation: "был_в", To: "Сварня"},                               // incomplete
	}
	saved, outside := store.saveExtractedTriples(triples)
	if saved != 2 {
		t.Fatalf("saved %d triples, want 2 (the invented relation is refused)", saved)
	}
	if outside != 1 {
		t.Fatalf("reported %d outsiders, want 1", outside)
	}
	var invented int
	if err := store.goals.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE relation='путает_имя'`).Scan(&invented); err != nil {
		t.Fatal(err)
	}
	if invented != 0 {
		t.Fatal("a relation outside the vocabulary was stored")
	}
	// The alias must have been folded into the canonical relation.
	var n int
	if err := store.goals.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE relation='заказал'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d заказал edges, want 1", n)
	}
}

// TestBackfillLLMGraphResumes covers the two properties that make a bounded
// backfill usable: it stops after the limit, and the next run continues instead
// of starting over.
func TestBackfillLLMGraphResumes(t *testing.T) {
	store := newTestStorage(t)

	// Five narrative entries, long enough to pass the length gate.
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		content := strings.Repeat("Кирилл и Теона пошли в Сварню вечером. ", 4)
		if _, err := store.saveWithKey("memory/memoirs/"+name,
			&MemoryValue{Content: content, Timestamp: "2026-07-23T18:00:00Z"}, content); err != nil {
			t.Fatal(err)
		}
	}

	calls := 0
	store.extractFn = func(text string) (*ExtractResult, error) {
		calls++
		return &ExtractResult{Triples: []GraphTriple{
			{From: "Кирилл", Relation: "был_в", To: "Сварня", Date: "2026-07-23"},
		}}, nil
	}

	first, err := store.backfillLLMGraph(2, true, "")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Scanned != 2 || calls != 2 {
		t.Fatalf("first run scanned %d entries with %d calls, want 2 and 2", first.Scanned, calls)
	}
	if first.LastKey == "" {
		t.Fatal("first run did not report a cursor")
	}

	second, err := store.backfillLLMGraph(2, false, "")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Scanned != 2 {
		t.Fatalf("second run scanned %d entries, want 2", second.Scanned)
	}
	if second.LastKey <= first.LastKey {
		t.Fatalf("second run did not advance: %q then %q", first.LastKey, second.LastKey)
	}
	if calls != 4 {
		t.Fatalf("extractor called %d times, want 4", calls)
	}

	// Both runs extracted the same relation; the graph must hold it once.
	var n int
	if err := store.goals.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE relation='был_в'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d был_в edges, want 1 (backfill is not idempotent)", n)
	}
}

// TestBackfillLLMSkipsShortEntries checks the length gate: a model call on a
// stub of text would only waste a round trip.
func TestBackfillLLMSkipsShortEntries(t *testing.T) {
	store := newTestStorage(t)
	if _, err := store.saveWithKey("memory/memoirs/tiny",
		&MemoryValue{Content: "ok", Timestamp: "2026-07-23T18:00:00Z"}, "ok"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	store.extractFn = func(text string) (*ExtractResult, error) {
		calls++
		return &ExtractResult{}, nil
	}
	if _, err := store.backfillLLMGraph(10, true, ""); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("extractor called %d times for a two-character entry, want 0", calls)
	}
}
