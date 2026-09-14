// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Inference over the knowledge graph.
//
// The graph holds edges; this file derives what those edges imply and checks
// what they cannot mean. The rules live in graph_rules.pl — versioned in git,
// embedded in the binary, reviewable and testable on their own. What they
// replace was assembled by string concatenation in the middle of a request
// handler, so the rules behind an answer could not be read, changed or trusted
// apart from the code around them.
//
// Facts are rendered here, the query is asked one predicate at a time, and the
// solutions are deduplicated in Go: a symmetric rule returns both orders, and a
// per-date rule returns the same pair once per date.
package main

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

//go:embed graph_rules.pl
var embeddedGraphRules string

// graphRulesFile is an optional override. The embedded copy is what ships; the
// override exists so the rules can be edited without a rebuild while working on
// them. It is a startup flag, not a runtime setting: the rules behind an answer
// should be the rules in the repository at that commit.
var graphRulesFile string

// graphRules returns the inference rules in effect.
func graphRules() (string, error) {
	if graphRulesFile == "" {
		return embeddedGraphRules, nil
	}
	data, err := os.ReadFile(graphRulesFile)
	if err != nil {
		return "", fmt.Errorf("read graph rules %s: %w", graphRulesFile, err)
	}
	return string(data), nil
}

// derivedPredicate is one relation the rules can derive.
//
// The list is written out rather than parsed from the rules file: a Prolog
// parser in Go would be a liability, and a test asserts that every name here is
// defined in graph_rules.pl, so the two cannot drift apart silently.
type derivedPredicate struct {
	Name string
	Goal string
	// Vars is the argument order, as written in Goal. The engine answers with
	// variable names, and sorting those names would render together(A,B,P,D)
	// as "person, person, date, place" — the fact would read in the wrong
	// order and quietly mislead whoever reads it.
	Vars []string
	// Unordered lists groups of variables whose order carries no meaning: the
	// two people in together, the two values in a contradiction. The engine
	// answers such a rule in both orders, and without this the same fact would
	// be reported twice.
	Unordered [][]string
	Note      string
}

var derivedPredicates = []derivedPredicate{
	{Name: "inverse_of", Goal: "inverse_of(A,B,R,S)", Vars: []string{"A", "B", "R", "S"},
		Note: "a relation whose reverse direction is implied but not stored"},
	{Name: "together", Goal: "together(A,B,P,D)", Vars: []string{"A", "B", "P", "D"},
		Unordered: [][]string{{"A", "B"}},
		Note:      "two people at the same place on the same day"},
	{Name: "serves", Goal: "serves(P,Dish,D)", Vars: []string{"P", "Dish", "D"},
		Note: "a place serves a dish, derived from an order placed during a visit"},
	{Name: "contradiction", Goal: "contradiction(A,R,B,C)", Vars: []string{"A", "R", "B", "C"},
		Unordered: [][]string{{"B", "C"}},
		Note:      "a relation that admits one value per subject but holds two"},
}

// renderFact renders a solution in the order the goal declares.
func renderFact(pred derivedPredicate, s map[string]string) string {
	parts := make([]string, 0, len(pred.Vars))
	for _, v := range pred.Vars {
		if value, ok := s[v]; ok {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " | ")
}

// factKey renders a solution as a deduplication key. Variables in an unordered
// group are sorted, so the engine answering a symmetric rule in both orders
// counts once.
func factKey(pred derivedPredicate, s map[string]string) string {
	values := make([]string, 0, len(pred.Vars))
	for _, v := range pred.Vars {
		values = append(values, s[v])
	}
	for _, group := range pred.Unordered {
		var picked []string
		positions := make([]int, 0, len(group))
		for _, name := range group {
			for i, v := range pred.Vars {
				if v == name {
					picked = append(picked, s[name])
					positions = append(positions, i)
				}
			}
		}
		sort.Strings(picked)
		for i, pos := range positions {
			values[pos] = picked[i]
		}
	}
	return strings.Join(values, "\x00")
}

// prologQuery is the seam for tests. Inference needs a running prolog-mcp; a
// unit test of the deduplication and the rendering should not, so the query
// function is a variable the test can replace.
var prologQuery = prologSolutions

// prologSolutions runs one Prolog program and returns its solutions. The
// response is a JSON array of variable bindings; an unsatisfied query answers
// null. The previous implementation stripped the brackets and returned the
// fragments, which threw away the structure the caller needs to tell one
// solution from the next.
func prologSolutions(code string) ([]map[string]string, error) {
	raw := callPrologRaw(code)
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
		return nil, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, fmt.Errorf("parse prolog response: %w", err)
	}
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		bindings := make(map[string]string, len(row))
		for k, v := range row {
			bindings[k] = fmt.Sprintf("%v", v)
		}
		out = append(out, bindings)
	}
	return out, nil
}

// graphFactsProgram renders the graph as Prolog facts. maxEdges bounds the
// program: an unbounded graph would turn one inference into an unbounded model
// call, and a bounded answer that says it was bounded is more useful than a
// slow one that does not.
func graphFactsProgram(db *sql.DB, maxEdges int) (string, int, error) {
	rows, err := db.Query(`
		SELECT f.name, t.name, e.relation, e.date
		  FROM graph_edges e
		  JOIN graph_entities f ON f.id = e.from_id
		  JOIN graph_entities t ON t.id = e.to_id
		 ORDER BY e.id`)
	if err != nil {
		return "", 0, fmt.Errorf("load edges: %w", err)
	}
	defer rows.Close()

	var b strings.Builder
	n := 0
	for rows.Next() {
		var from, to, relation, date string
		if err := rows.Scan(&from, &to, &relation, &date); err != nil {
			return "", 0, fmt.Errorf("scan edge: %w", err)
		}
		if n >= maxEdges {
			break
		}
		fmt.Fprintf(&b, "edge(%s,%s,%s).\n", prologAtom(from), prologAtom(to), prologAtom(relation))
		if date != "" {
			fmt.Fprintf(&b, "edge_on(%s,%s,%s,%s).\n",
				prologAtom(from), prologAtom(to), prologAtom(relation), prologAtom(date))
		}
		n++
	}
	return b.String(), n, rows.Err()
}

// inferenceFact is one derived relation.
type inferenceFact struct {
	Predicate string `json:"predicate"`
	Fact      string `json:"fact"`
	Note      string `json:"note,omitempty"`
}

// inferenceResult is what one inference pass found.
type inferenceResult struct {
	EdgesConsidered int             `json:"edges_considered"`
	Derived         []inferenceFact `json:"derived"`
	ByPredicate     map[string]int  `json:"by_predicate"`
	Failed          []string        `json:"failed,omitempty"`
}

// graphInfer derives facts from the graph using the rules. only restricts the
// pass to one predicate; an empty value runs them all.
func graphInfer(db *sql.DB, only string, maxEdges int) (*inferenceResult, error) {
	rules, err := graphRules()
	if err != nil {
		return nil, err
	}
	facts, edges, err := graphFactsProgram(db, maxEdges)
	if err != nil {
		return nil, err
	}
	if edges == 0 {
		return &inferenceResult{ByPredicate: map[string]int{}}, nil
	}

	res := &inferenceResult{EdgesConsidered: edges, ByPredicate: map[string]int{}}
	seen := map[string]bool{}

	for _, pred := range derivedPredicates {
		if only != "" && only != pred.Name {
			continue
		}
		solutions, err := prologQuery(rules + "\n" + facts + "\n?- " + pred.Goal + ".\n")
		if err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", pred.Name, err))
			continue
		}
		for _, s := range solutions {
			key := pred.Name + "|" + factKey(pred, s)
			if seen[key] {
				continue
			}
			seen[key] = true
			res.Derived = append(res.Derived, inferenceFact{
				Predicate: pred.Name,
				Fact:      renderFact(pred, s),
				Note:      pred.Note,
			})
			res.ByPredicate[pred.Name]++
		}
	}
	sort.Slice(res.Derived, func(i, j int) bool {
		if res.Derived[i].Predicate != res.Derived[j].Predicate {
			return res.Derived[i].Predicate < res.Derived[j].Predicate
		}
		return res.Derived[i].Fact < res.Derived[j].Fact
	})
	return res, nil
}

// verificationIssue is one thing the graph asserts that it should not.
type verificationIssue struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
}

// verificationResult is what one verification pass found.
type verificationResult struct {
	EdgesConsidered int                 `json:"edges_considered"`
	Issues          []verificationIssue `json:"issues"`
	ByKind          map[string]int      `json:"by_kind"`
}

// graphVerify looks for facts the graph cannot hold: contradictions the rules
// derive, plus edges whose endpoints contradict the relation vocabulary.
func graphVerify(db *sql.DB, maxEdges int) (*verificationResult, error) {
	rules, err := graphRules()
	if err != nil {
		return nil, err
	}
	facts, edges, err := graphFactsProgram(db, maxEdges)
	if err != nil {
		return nil, err
	}

	res := &verificationResult{EdgesConsidered: edges, ByKind: map[string]int{}}

	// Contradictions, straight from the rules.
	if edges > 0 {
		solutions, err := prologQuery(rules + "\n" + facts + "\n?- contradiction(A,R,B,C).\n")
		if err != nil {
			return nil, err
		}
		for _, s := range solutions {
			res.Issues = append(res.Issues, verificationIssue{
				Kind:    "contradiction",
				Subject: s["A"],
				Detail: fmt.Sprintf("%s holds both %q and %q for the relation %q",
					s["A"], s["B"], s["C"], s["R"]),
			})
			res.ByKind["contradiction"]++
		}
	}

	// Vocabulary violations: an edge whose endpoints contradict the types the
	// relation declares. The rules cannot see this because it is about the
	// relation vocabulary, which lives in Go.
	violations, err := vocabularyViolations(db)
	if err != nil {
		return nil, err
	}
	for _, v := range violations {
		res.Issues = append(res.Issues, v)
		res.ByKind[v.Kind]++
	}

	// One issue per distinct violation. The same document illustrated by four
	// images is one thing to look at, not four.
	seen := map[string]bool{}
	unique := res.Issues[:0]
	for _, issue := range res.Issues {
		key := issue.Kind + "|" + issue.Subject + "|" + issue.Detail
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, issue)
	}
	res.Issues = unique

	res.ByKind = map[string]int{}
	for _, issue := range res.Issues {
		res.ByKind[issue.Kind]++
	}

	sort.Slice(res.Issues, func(i, j int) bool {
		if res.Issues[i].Kind != res.Issues[j].Kind {
			return res.Issues[i].Kind < res.Issues[j].Kind
		}
		return res.Issues[i].Subject < res.Issues[j].Subject
	})
	return res, nil
}

// vocabularyViolations reports edges whose endpoints contradict the types their
// relation declares: a place cannot order a dish, an image cannot be a person.
func vocabularyViolations(db *sql.DB) ([]verificationIssue, error) {
	rows, err := db.Query(`
		SELECT f.name, f.type, t.name, t.type, e.relation
		  FROM graph_edges e
		  JOIN graph_entities f ON f.id = e.from_id
		  JOIN graph_entities t ON t.id = e.to_id
		 ORDER BY e.id`)
	if err != nil {
		return nil, fmt.Errorf("load typed edges: %w", err)
	}
	defer rows.Close()

	var issues []verificationIssue
	for rows.Next() {
		var from, fromType, to, toType, relation string
		if err := rows.Scan(&from, &fromType, &to, &toType, &relation); err != nil {
			return nil, fmt.Errorf("scan typed edge: %w", err)
		}
		spec := relationSpecByName(relation)
		if spec == nil {
			continue
		}
		if !typeAllowed(spec.From, fromType) {
			issues = append(issues, verificationIssue{
				Kind:    "type_violation",
				Subject: from,
				Detail: fmt.Sprintf("%q (%s) cannot be the subject of %q, which expects %s",
					from, typeName(fromType), relation, strings.Join(spec.From, "|")),
			})
		}
		if !typeAllowed(spec.To, toType) {
			issues = append(issues, verificationIssue{
				Kind:    "type_violation",
				Subject: to,
				Detail: fmt.Sprintf("%q (%s) cannot be the object of %q, which expects %s",
					to, typeName(toType), relation, strings.Join(spec.To, "|")),
			})
		}
	}
	return issues, rows.Err()
}

// typeName renders an entity type for a message.
func typeName(typ string) string {
	if typ == "" {
		return "untyped"
	}
	return typ
}
