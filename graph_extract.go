// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Deterministic graph extractors.
//
// Until now the graph only grew when an LLM happened to emit a triple, or when
// someone called graph_add_edge by hand. That left the graph both sparse and
// noisy: it knew that an image mentions Сварня, but not that Kirill ate there
// eleven times, because the visits sit in structured memory entries that nobody
// ever read back into the graph.
//
// These extractors read those entries directly. They are pure functions over
// (key, value): no network, no model, no guessing. When a field is present the
// relation is a fact; when it is absent the extractor returns nothing rather
// than inventing something. They run in the save path, so the graph now grows
// by itself, and a backfill pass applies them to the history that already
// exists.
//
// What is deliberately NOT extracted: tags. A tag is a topic of a record, not a
// relation between two things in the world. Wiring tags in would have added
// hundreds of low-signal edges of exactly the kind the relation vocabulary was
// introduced to remove.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// deterministicTriple is one relation read out of a memory entry. Source is
// optional: when an extractor leaves it empty the entry key is used, which is
// what provenance should point at anyway.
type deterministicTriple struct {
	From     string
	To       string
	Relation string
	Date     string
	Source   string

	// Type hints for entities that do not exist yet. Used only on creation.
	FromType string
	ToType   string
}

// memoryExtractor turns one memory entry into relations. Extractors are pure:
// the same key and value always produce the same triples, and an entry they do
// not recognise produces none.
type memoryExtractor struct {
	Name string
	Run  func(key string, value *MemoryValue) []deterministicTriple
}

// memoryExtractors is the registry. Adding a source is adding an entry here.
var memoryExtractors = []memoryExtractor{
	{Name: "place-visit", Run: extractPlaceVisit},
	{Name: "food-registry", Run: extractFoodRegistry},
	{Name: "person-relation", Run: extractPersonRelation},
	{Name: "mail-sender", Run: extractMailSender},
}

// entryObject decodes the JSON document a structured memory entry carries.
// Those entries store a JSON document as a string inside "content", so it is
// decoded twice: the outer MemoryValue, then the document.
func entryObject(value *MemoryValue) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	raw := strings.TrimSpace(value.Content)
	if !strings.HasPrefix(raw, "{") {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, false
	}
	return obj, true
}

// looksLikeDate reports whether s starts with an ISO date (YYYY-MM-DD).
func looksLikeDate(s string) bool {
	if len(s) < 10 {
		return false
	}
	if s[4] != '-' || s[7] != '-' {
		return false
	}
	_, err := time.Parse("2006-01-02", s[:10])
	return err == nil
}

// entryDate returns the date an entry is about: an explicit date field when the
// document carries one, otherwise the day the entry was written. Entries record
// dates under different names depending on who wrote them.
func entryDate(obj map[string]any, value *MemoryValue) string {
	for _, field := range []string{"date", "last_hinkali", "last_visit", "visited", "when"} {
		if s, ok := obj[field].(string); ok && looksLikeDate(s) {
			return s[:10]
		}
	}
	if value != nil && looksLikeDate(value.Timestamp) {
		return value.Timestamp[:10]
	}
	return ""
}

// splitList splits a comma- or "и"-separated list of names.
func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '/'
	})
	var out []string
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// stringList normalises a JSON value that may be a string or an array of
// strings into a slice. The registry and the visit entries disagree on which
// one a list field is.
func stringList(v any) []string {
	switch t := v.(type) {
	case string:
		return splitList(t)
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

// canonicalOwner maps the possessive forms that appear in relation phrases
// ("жена Кирилла") to the canonical entity name.
func canonicalOwner(s string) string {
	key := normalizeName(s)
	switch {
	case strings.HasPrefix(key, "кирилл"), strings.HasPrefix(key, "kirill"):
		return "Кирилл"
	case strings.HasPrefix(key, "барон"), strings.HasPrefix(key, "baron"):
		return "Барон"
	}
	return strings.TrimSpace(s)
}

// senderDisplayName extracts the display name from a mail From header such as
// `Т-Банк <inform@emails.tinkoff.ru>`.
func senderDisplayName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "<"); i > 0 {
		if name := strings.TrimSpace(s[:i]); name != "" {
			return name
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// Extractors
// ---------------------------------------------------------------------------

// extractPlaceVisit reads an entry that records a visit: a document with a
// "place" field, and optionally who was there and what was ordered. This is the
// one that fills in "Kirill ate at X on date Y", which the graph was missing
// even though the food registry had the dates all along.
func extractPlaceVisit(key string, value *MemoryValue) []deterministicTriple {
	obj, ok := entryObject(value)
	if !ok {
		return nil
	}
	place, _ := obj["place"].(string)
	place = strings.TrimSpace(place)
	if place == "" {
		return nil
	}
	date := entryDate(obj, value)

	out := []deterministicTriple{{
		From: "Кирилл", FromType: TypePerson,
		To: place, ToType: TypePlace,
		Relation: "был_в", Date: date,
	}}

	if with, _ := obj["with"].(string); with != "" {
		for _, person := range splitList(with) {
			out = append(out, deterministicTriple{
				From: "Кирилл", FromType: TypePerson,
				To: canonicalOwner(person), ToType: TypePerson,
				Relation: "был_с", Date: date,
			})
		}
	}
	for _, field := range []string{"order", "dishes", "dish"} {
		for _, dish := range stringList(obj[field]) {
			out = append(out, deterministicTriple{
				From: "Кирилл", FromType: TypePerson,
				To: dish, ToType: TypeDish,
				Relation: "заказал", Date: date,
			})
			out = append(out, deterministicTriple{
				From: dish, FromType: TypeDish,
				To: place, ToType: TypePlace,
				Relation: "подают_в", Date: date,
			})
		}
	}
	return out
}

// extractFoodRegistry reads the food registry: a document with a "places" array,
// where each place carries its dishes and the dates it was visited. One entry
// holds the whole dining history.
func extractFoodRegistry(key string, value *MemoryValue) []deterministicTriple {
	obj, ok := entryObject(value)
	if !ok {
		return nil
	}
	places, ok := obj["places"].([]any)
	if !ok {
		return nil
	}

	var out []deterministicTriple
	for _, raw := range places {
		place, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := place["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, dish := range stringList(place["dishes"]) {
			out = append(out, deterministicTriple{
				From: dish, FromType: TypeDish,
				To: name, ToType: TypePlace,
				Relation: "подают_в",
			})
		}
		for _, visit := range stringList(place["visits"]) {
			if !looksLikeDate(visit) {
				continue
			}
			out = append(out, deterministicTriple{
				From: "Кирилл", FromType: TypePerson,
				To: name, ToType: TypePlace,
				Relation: "был_в", Date: visit[:10],
			})
		}
	}
	return out
}

// extractPersonRelation reads a person entry that states how they relate to
// Kirill ("relation": "жена Кирилла"). The relation word is the first token; the
// rest names the other end.
func extractPersonRelation(key string, value *MemoryValue) []deterministicTriple {
	obj, ok := entryObject(value)
	if !ok {
		return nil
	}
	name, _ := obj["name"].(string)
	relation, _ := obj["relation"].(string)
	name, relation = strings.TrimSpace(name), strings.TrimSpace(relation)
	if name == "" || relation == "" {
		return nil
	}

	fields := strings.Fields(relation)
	if len(fields) < 2 {
		return nil
	}
	word := fields[0]
	target := canonicalOwner(strings.Join(fields[1:], " "))
	if _, known := canonicalRelation(word); !known {
		return nil
	}
	if target == "" || target == name {
		return nil
	}
	return []deterministicTriple{{
		From: name, FromType: TypePerson,
		To: target, ToType: TypePerson,
		Relation: word, Date: entryDate(obj, value),
	}}
}

// extractMailSender reads a mail entry and records who sent it. The sender
// header is `Display Name <address>`; only the display name becomes an entity,
// because an address is not a thing in the world.
func extractMailSender(key string, value *MemoryValue) []deterministicTriple {
	if !strings.HasPrefix(key, "memory/mail/") {
		return nil
	}
	obj, ok := entryObject(value)
	if !ok {
		return nil
	}
	sender, _ := obj["sender"].(string)
	sender = senderDisplayName(sender)
	if sender == "" {
		return nil
	}
	return []deterministicTriple{{
		From: sender, FromType: TypeOrg,
		To: "Кирилл", ToType: TypePerson,
		Relation: "направил", Date: entryDate(obj, value),
	}}
}

// ---------------------------------------------------------------------------
// Application
// ---------------------------------------------------------------------------

// triplesForEntry runs every extractor over one entry.
func triplesForEntry(key string, value *MemoryValue) []deterministicTriple {
	var out []deterministicTriple
	for _, ex := range memoryExtractors {
		out = append(out, ex.Run(key, value)...)
	}
	return out
}

// applyExtractors writes the relations an entry implies. It runs in the save
// path, so the graph grows without anyone calling graph_add_edge. Failures are
// logged, never returned: a graph edge is a side effect of saving, and a
// side effect must not fail the save.
func (s *Storage) applyExtractors(key string, value *MemoryValue) int {
	triples := triplesForEntry(key, value)
	written := 0
	for _, t := range triples {
		source := t.Source
		if source == "" {
			source = "auto:memory:" + key
		}
		if err := addGraphEdgeTyped(s.goals, t.From, t.FromType, t.To, t.ToType, t.Relation, t.Date, source); err != nil {
			log.Printf("⚠ graph: extract %q: %v", key, err)
			continue
		}
		written++
	}
	return written
}

// deterministicReport summarises a backfill pass.
type deterministicReport struct {
	EntriesScanned int            `json:"entries_scanned"`
	EntriesMatched int            `json:"entries_matched"`
	Triples        int            `json:"triples"`
	ByExtractor    map[string]int `json:"by_extractor"`
	ByRelation     map[string]int `json:"by_relation"`
}

// allKeys walks a prefix and returns every leaf key under it. keyvalembd.List
// has S3 folder semantics: it collapses sub-folders into single entries with a
// trailing slash, so one call sees only one level and a recursive walk is
// required to reach the leaves.
func (s *Storage) allKeys(prefix string, depth int) []string {
	if depth > 32 {
		return nil
	}
	var out []string
	for key := range s.kv.List(prefix) {
		if strings.HasSuffix(key, "/") {
			out = append(out, s.allKeys(key, depth+1)...)
			continue
		}
		out = append(out, key)
	}
	return out
}

// backfillDeterministicGraph walks the memory store once and applies every
// extractor to every entry. Idempotent: addGraphEdge collapses an edge that is
// already there, so re-running costs time but changes nothing.
func (s *Storage) backfillDeterministicGraph() (*deterministicReport, error) {
	report := &deterministicReport{ByExtractor: map[string]int{}, ByRelation: map[string]int{}}

	for _, key := range s.allKeys("memory/", 0) {
		value, err := s.Get(key)
		if err != nil {
			continue
		}
		report.EntriesScanned++

		matched := false
		for _, ex := range memoryExtractors {
			triples := ex.Run(key, value)
			if len(triples) == 0 {
				continue
			}
			matched = true
			report.ByExtractor[ex.Name] += len(triples)
			for _, t := range triples {
				source := t.Source
				if source == "" {
					source = "auto:memory:" + key
				}
				if err := addGraphEdgeTyped(s.goals, t.From, t.FromType, t.To, t.ToType, t.Relation, t.Date, source); err != nil {
					return report, fmt.Errorf("backfill %q (%s): %w", key, ex.Name, err)
				}
				report.Triples++
				canonical, _ := canonicalRelation(t.Relation)
				report.ByRelation[canonical]++
			}
		}
		if matched {
			report.EntriesMatched++
		}
	}
	return report, nil
}
