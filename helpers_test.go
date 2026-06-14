package main

import (
	"strings"
	"testing"
)

func TestAutoKey(t *testing.T) {
	key1 := autoKey("hello world")
	key2 := autoKey("hello world")

	if !strings.HasPrefix(key1, "memory/auto/") {
		t.Fatalf("autoKey() = %q, want prefix memory/auto/", key1)
	}
	if key1 != key2 {
		t.Fatalf("autoKey() not deterministic: %q vs %q", key1, key2)
	}

	parts := strings.Split(key1, "/")
	if len(parts) != 4 {
		t.Fatalf("autoKey() parts = %d, want 4", len(parts))
	}
	if len(parts[2]) != 10 {
		t.Fatalf("autoKey() date part = %q, want YYYY-MM-DD format", parts[2])
	}

	key3 := autoKey("different content")
	if key1 == key3 {
		t.Fatal("autoKey() returned same key for different content")
	}
}

func TestParseStoredLabels(t *testing.T) {
	got := parseStoredLabels(`["bug","mcp"]`)
	if len(got) != 2 || got[0] != "bug" || got[1] != "mcp" {
		t.Fatalf("parseStoredLabels() = %v, want [bug mcp]", got)
	}

	got = parseStoredLabels("")
	if got != nil {
		t.Fatalf("parseStoredLabels(\"\") = %v, want nil", got)
	}

	got = parseStoredLabels(`["bug",,"mcp"]`) // malformed
	// Should gracefully handle — returns nil after failed unmarshal
	if got != nil {
		t.Logf("parseStoredLabels(malformed) = %v (expected nil or best-effort)", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Fatalf("truncate(short) = %q, want hello", got)
	}
	if got := truncate("hello world", 8); got != "hello wo…" {
		t.Fatalf("truncate(long) = %q, want hello wo…", got)
	}
	if got := truncate("", 5); got != "" {
		t.Fatalf("truncate(empty) = %q, want empty", got)
	}
	if got := truncate("exactly", 7); got != "exactly" {
		t.Fatalf("truncate(exact) = %q, want exactly", got)
	}
}

func TestGoalRowConversion(t *testing.T) {
	row := &goalRow{
		ID:          "goal/2026-06-14/12345",
		Title:       "Test Goal",
		Description: "Test description",
		Status:      "active",
		Labels:      `["bug","mcp"]`,
		Priority:    8,
		Progress:    50,
		Deadline:    "2026-12-31",
	}

	goal := row.toGoal()
	if goal.Title != row.Title {
		t.Fatalf("toGoal().Title = %q, want %q", goal.Title, row.Title)
	}
	if len(goal.Labels) != 2 {
		t.Fatalf("toGoal().Labels = %v, want [bug mcp]", goal.Labels)
	}

	// Round-trip: goalRow → Goal → goalRow
	back := goalFrom(goal)
	if back.Title != row.Title {
		t.Fatalf("goalFrom().Title = %q, want %q", back.Title, row.Title)
	}
}
