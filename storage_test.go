package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGetTimelineAppliesDateFilters(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.LogEvent("old", "old-key", "old summary", "")
	store.LogEvent("new", "new-key", "new summary", "")

	rows, err := store.goals.Query(`SELECT id FROM timeline_events ORDER BY id ASC`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d events, want 2", len(ids))
	}

	oldCreatedAt := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := store.goals.Exec(`UPDATE timeline_events SET created_at = ? WHERE id = ?`, oldCreatedAt, ids[0]); err != nil {
		t.Fatal(err)
	}

	from := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	events, err := store.GetTimeline(from, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %#v", len(events), events)
	}
	if events[0].Key != "new-key" {
		t.Fatalf("got event key %q, want new-key", events[0].Key)
	}
}
