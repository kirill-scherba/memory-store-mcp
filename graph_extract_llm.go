// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// LLM graph backfill.
//
// The deterministic extractors cover entries that already state a relation in a
// field. Everything else — memoirs, conversations, notes, goal descriptions —
// states relations in prose, and only a model can read them out. This file runs
// the extraction over that history.
//
// Two properties matter more than throughput here:
//
//   - It is bounded. A run costs a predictable number of model calls, so it can
//     be started, measured and stopped instead of becoming an overnight job.
//   - It resumes. The last processed key is kept in memory, so the work can be
//     spread over several runs without reprocessing what is already done.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

// llmBackfillSections are the memory sections whose entries are narrative
// enough to be worth a model call.
//
// The list is deliberately short, and it was measured rather than guessed. A
// sample of twenty entries drawn from memory/baron/ and memory/chat/ — agent
// configuration and chat transcripts — produced one usable relation and nine
// invented ones; the sections that hold prose about the world are the ones
// worth paying for. Everything else is machine state (auto, agent, scheduler,
// mail-butler) or already covered by a deterministic extractor (gallery, mail).
var llmBackfillSections = []string{
	"memory/memoirs/",
	"memory/user/",
	"memory/goals/",
	"memory/conversations/",
	"memory/story/",
	"memory/notes/",
	"memory/observations/",
	"memory/philosophy/",
}

const (
	// A text shorter than this cannot hold two named entities and a relation
	// between them; calling the model would only waste a round trip.
	llmBackfillMinChars = 80
	// Bound the prompt: the extractor's job is to find relations, not to
	// summarise a book chapter.
	llmBackfillMaxChars = 4000
	// Where the resume cursor lives. The key is deliberately outside
	// llmBackfillSections so the cursor never feeds itself back to the model.
	llmBackfillCursorKey = "memory/graph/llm-backfill/cursor"
)

// llmBackfillReport summarises one backfill run.
type llmBackfillReport struct {
	Scanned          int            `json:"scanned"`
	Extracted        int            `json:"extracted"`
	Triples          int            `json:"triples"`
	Facts            int            `json:"facts"`
	Outside          int            `json:"outside_vocabulary"`
	Failed           int            `json:"failed"`
	Relations        map[string]int `json:"relations"`
	OutsideRelations []string       `json:"outside_relations,omitempty"`
	LastKey          string         `json:"last_key"`
	Remaining        int            `json:"remaining"`
}

// llmBackfillKeys returns the candidate keys in a stable order: every leaf under
// the narrative sections, sorted so a cursor means something. A single prefix
// overrides the section list, which is how the yield of one section can be
// measured without running the others.
func (s *Storage) llmBackfillKeys(prefix string) []string {
	sections := llmBackfillSections
	if prefix != "" {
		sections = []string{prefix}
	}
	seen := map[string]bool{}
	var keys []string
	for _, prefix := range sections {
		for _, key := range s.allKeys(prefix, 0) {
			if seen[key] {
				continue
			}
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// llmBackfillCursor returns the key the next run should start after.
func (s *Storage) llmBackfillCursor() string {
	value, err := s.Get(llmBackfillCursorKey)
	if err != nil || value == nil {
		return ""
	}
	return strings.TrimSpace(value.Content)
}

// setLLMBackfillCursor records how far the backfill has got.
func (s *Storage) setLLMBackfillCursor(key string) {
	value := &MemoryValue{
		Content: key,
		Summary: "llm backfill cursor",
		Source:  "graph-backfill",
		Tags:    []string{"graph", "backfill", "cursor"},
	}
	if _, err := s.saveWithKey(llmBackfillCursorKey, value, key); err != nil {
		log.Printf("⚠ graph: could not store the backfill cursor: %v", err)
	}
}

// llmBackfillText returns the text worth showing the model for an entry, or ""
// when the entry is too short or empty to be worth a call.
func llmBackfillText(value *MemoryValue) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(value.Content)
	if text == "" {
		text = strings.TrimSpace(value.Summary)
	}
	if len([]rune(text)) < llmBackfillMinChars {
		return ""
	}
	runes := []rune(text)
	if len(runes) > llmBackfillMaxChars {
		text = string(runes[:llmBackfillMaxChars])
	}
	return text
}

// backfillLLMGraph runs the LLM extractor over narrative entries, resuming from
// the stored cursor and stopping after limit model calls. It returns what it
// did so a run can be judged before the next one is started.
func (s *Storage) backfillLLMGraph(limit int, reset bool, prefix string) (*llmBackfillReport, error) {
	if s.extractFn == nil {
		return nil, fmt.Errorf("no extractor configured")
	}
	if limit <= 0 {
		limit = 10
	}

	keys := s.llmBackfillKeys(prefix)
	// A one-off prefix run is a measurement, not a pass: it must not move the
	// cursor of the full backfill.
	cursor := ""
	if prefix == "" {
		cursor = s.llmBackfillCursor()
	}
	if reset {
		cursor = ""
	}

	report := &llmBackfillReport{Relations: map[string]int{}}
	start := 0
	if cursor != "" {
		start = sort.SearchStrings(keys, cursor)
		if start < len(keys) && keys[start] == cursor {
			start++ // resume after the key that was already processed
		}
	}
	report.Remaining = len(keys) - start

	for _, key := range keys[start:] {
		if report.Scanned >= limit {
			break
		}
		value, err := s.Get(key)
		if err != nil {
			continue
		}
		text := llmBackfillText(value)
		if text == "" {
			report.LastKey = key
			continue
		}

		report.Scanned++
		res, err := s.extractFn(text)
		if err != nil {
			report.Failed++
			log.Printf("⚠ graph: llm backfill %q: %v", key, err)
			report.LastKey = key
			continue
		}
		report.Facts += len(res.Facts)

		if len(res.Triples) > 0 {
			report.Extracted++
			saved, outside := s.saveExtractedTriples(res.Triples)
			report.Triples += saved
			report.Outside += outside
			for _, t := range res.Triples {
				relation, known := canonicalRelation(t.Relation)
				report.Relations[relation]++
				if !known {
					report.OutsideRelations = append(report.OutsideRelations, t.Relation)
				}
			}
		}
		report.LastKey = key
	}

	if report.LastKey != "" && prefix == "" {
		s.setLLMBackfillCursor(report.LastKey)
	}
	report.Remaining = len(keys) - (start + report.Scanned)
	if report.Remaining < 0 {
		report.Remaining = 0
	}
	return report, nil
}

// marshalLLMBackfillReport renders a report for a tool result.
func marshalLLMBackfillReport(r *llmBackfillReport) string {
	out, _ := json.MarshalIndent(r, "", "  ")
	return string(out)
}
