// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// errFakeProlog stands in for a prolog-mcp that is down or refusing a program.
var errFakeProlog = errors.New("prolog engine unavailable")

// TestGraphRulesDefineEveryPredicate keeps the Go list and the rules file in
// step. The list is written out rather than parsed — a Prolog parser in Go
// would be a liability — so a test is what stops the two from drifting apart.
func TestGraphRulesDefineEveryPredicate(t *testing.T) {
	rules, err := graphRules()
	if err != nil {
		t.Fatalf("graphRules: %v", err)
	}
	for _, pred := range derivedPredicates {
		// A predicate is defined by a clause: name( ... ) :- ...
		defined := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(pred.Name) + `\s*\(`).MatchString(rules)
		if !defined {
			t.Errorf("predicate %q is listed in Go but not defined in graph_rules.pl", pred.Name)
		}
		if pred.Goal == "" {
			t.Errorf("predicate %q has no goal", pred.Name)
		}
	}
}

// TestGraphRulesAreEmbedded guards the build: the rules must ship inside the
// binary, because a missing file at runtime would silently turn inference into
// a no-op.
func TestGraphRulesAreEmbedded(t *testing.T) {
	if strings.TrimSpace(embeddedGraphRules) == "" {
		t.Fatal("graph_rules.pl is empty; the embed did not pick it up")
	}
	for _, want := range []string{"edge_on", "inverse(", "functional("} {
		if !strings.Contains(embeddedGraphRules, want) {
			t.Errorf("graph_rules.pl does not mention %q", want)
		}
	}
}

// TestIllustrationDirection pins the direction of иллюстрация. The legacy data
// carries it both ways, and the first version of the vocabulary declared it
// backwards — which mattered, because the object end was a single type and
// propagation then painted every illustrated entity as an image.
func TestIllustrationDirection(t *testing.T) {
	spec := relationSpecByName("иллюстрация")
	if spec == nil {
		t.Fatal("иллюстрация is not in the vocabulary")
	}
	if !typeAllowed(spec.From, TypeImage) {
		t.Error("an image must be allowed as the subject of иллюстрация")
	}
	if len(spec.To) > 0 && !typeAllowed(spec.To, TypeDoc) {
		t.Errorf("a document must be allowed as the object of иллюстрация, got %v", spec.To)
	}
	// The object end must stay open: an image can illustrate a project or a
	// place, not only a document. A single-type object end is what mistyped
	// Cooksy and MATRICA as images.
	if len(spec.To) == 1 {
		t.Errorf("the object end of иллюстрация is a single type (%v); "+
			"propagation would type everything illustrated as that type", spec.To)
	}
}

// TestGraphInferDeduplicates covers the reason deduplication is needed at all:
// a symmetric rule returns both orders, and a per-date rule returns the same
// pair once per date. Without it a place "serves" the same dish a dozen times.
func TestGraphInferDeduplicates(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	if err := addGraphEdge(db, "Кирилл", "Сварня", "был_в", "2026-07-15", "test"); err != nil {
		t.Fatal(err)
	}

	old := prologQuery
	defer func() { prologQuery = old }()
	prologQuery = func(code string) ([]map[string]string, error) {
		// The engine answers a symmetric rule with both orders, and a dated
		// rule with the same pair on each date.
		return []map[string]string{
			{"A": "Кирилл", "B": "Теона", "P": "Сварня", "D": "2026-07-15"},
			{"B": "Кирилл", "A": "Теона", "P": "Сварня", "D": "2026-07-15"},
			{"A": "Кирилл", "B": "Теона", "P": "Сварня", "D": "2026-07-16"},
		}, nil
	}

	res, err := graphInfer(db, "together", maxInferenceEdges)
	if err != nil {
		t.Fatalf("graphInfer: %v", err)
	}
	// Two facts, not three: the engine answered the symmetric rule in both
	// orders for 2026-07-15, and those collapse. The 16th is a different day
	// and must survive — a per-date rule reports one fact per date, not one
	// fact overall.
	if len(res.Derived) != 2 {
		t.Fatalf("got %d derived facts, want 2: %+v", len(res.Derived), res.Derived)
	}
	if res.ByPredicate["together"] != 2 {
		t.Fatalf("by_predicate = %v, want together=2", res.ByPredicate)
	}
	// The fact must read in the order the rule declares — person, person,
	// place, date — not in the alphabetical order of the variable names.
	if got, want := res.Derived[0].Fact, "Кирилл | Теона | Сварня | 2026-07-15"; got != want {
		t.Fatalf("fact rendered as %q, want %q", got, want)
	}
}

// TestGraphInferReportsEngineFailure checks that a Prolog error is reported
// rather than silently swallowed into an empty result.
func TestGraphInferReportsEngineFailure(t *testing.T) {
	store := newTestStorage(t)
	if err := addGraphEdge(store.goals, "Кирилл", "Сварня", "был_в", "2026-07-15", "test"); err != nil {
		t.Fatal(err)
	}

	old := prologQuery
	defer func() { prologQuery = old }()
	prologQuery = func(code string) ([]map[string]string, error) {
		return nil, errFakeProlog
	}

	res, err := graphInfer(store.goals, "together", maxInferenceEdges)
	if err != nil {
		t.Fatalf("graphInfer: %v", err)
	}
	if len(res.Failed) == 0 {
		t.Fatal("engine failure was not reported")
	}
	if len(res.Derived) != 0 {
		t.Fatalf("got %d derived facts from a failing engine, want 0", len(res.Derived))
	}
}

// TestVocabularyViolations is the half of verification that needs no engine: an
// edge whose endpoints contradict what the relation declares.
func TestVocabularyViolations(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	// A place cannot order a dish; a person can. This is the exact shape of the
	// junk found in production: "Сварня -> рубиновая: заказал".
	if err := addGraphEdgeTyped(db, "Сварня", TypePlace, "рубиновая", TypeDish, "заказал", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}
	// A well-formed edge must not be reported.
	if err := addGraphEdgeTyped(db, "Кирилл", TypePerson, "Сварня", TypePlace, "был_в", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}

	issues, err := vocabularyViolations(db)
	if err != nil {
		t.Fatalf("vocabularyViolations: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
	}
	if issues[0].Kind != "type_violation" || issues[0].Subject != "Сварня" {
		t.Fatalf("issue = %+v, want a type_violation on Сварня", issues[0])
	}
}

// TestVocabularyViolationsIgnoresUntyped keeps the check quiet on a graph that
// has not been typed yet: an empty type is not a contradiction.
func TestVocabularyViolationsIgnoresUntyped(t *testing.T) {
	store := newTestStorage(t)
	if err := addGraphEdge(store.goals, "Сварня", "рубиновая", "заказал", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}
	issues, err := vocabularyViolations(store.goals)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("got %d issues for untyped entities, want 0: %+v", len(issues), issues)
	}
}

// TestGraphVerifyCombinesBothChecks checks that verification reports the rules'
// contradictions and the vocabulary violations together.
func TestGraphVerifyCombinesBothChecks(t *testing.T) {
	store := newTestStorage(t)
	db := store.goals

	if err := addGraphEdgeTyped(db, "Сварня", TypePlace, "рубиновая", TypeDish, "заказал", "2026-07-23", "test"); err != nil {
		t.Fatal(err)
	}

	old := prologQuery
	defer func() { prologQuery = old }()
	prologQuery = func(code string) ([]map[string]string, error) {
		return []map[string]string{
			{"A": "Кирилл", "R": "жена", "B": "Эка", "C": "Лена"},
		}, nil
	}

	res, err := graphVerify(db, maxInferenceEdges)
	if err != nil {
		t.Fatalf("graphVerify: %v", err)
	}
	if res.ByKind["contradiction"] != 1 || res.ByKind["type_violation"] != 1 {
		t.Fatalf("by_kind = %v, want one of each", res.ByKind)
	}
}
