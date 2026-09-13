// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestGoalsTimestampMigration guards against the driver swap breaking goal
// reads: sqlh maps time.Time to a "timestamp" column, but the goals table was
// created long ago with created_at/updated_at declared TEXT. modernc.org/sqlite
// only returns time.Time for time-typed columns, so a TEXT column makes every
// goal read fail with "unsupported Scan, storing driver.Value type string into
// type *time.Time".
func TestGoalsTimestampMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	raw, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE goals (
		id          TEXT PRIMARY KEY NOT NULL,
		title       TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'active',
		labels      TEXT NOT NULL DEFAULT '[]',
		priority    INTEGER NOT NULL DEFAULT 5,
		progress    INTEGER NOT NULL DEFAULT 0,
		deadline    TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at  TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		t.Fatalf("create legacy goals: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO goals (id, title, created_at, updated_at) VALUES (?,?,?,?)`,
		"g1", "legacy goal", "2026-05-03 21:39:49", "2026-05-03 21:39:49",
	); err != nil {
		t.Fatalf("seed legacy goal: %v", err)
	}
	raw.Close()

	store, err := NewStorage(dbPath)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer store.Close()

	// The column must have been migrated to a time type.
	typ, err := columnDeclType(store.goals, "goals", "created_at")
	if err != nil {
		t.Fatal(err)
	}
	if typ == "TEXT" {
		t.Fatalf("created_at still declared %q", typ)
	}

	goals, err := store.ListGoals("", nil)
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 1 || goals[0].ID != "g1" {
		t.Fatalf("goals = %+v, want the migrated one", goals)
	}
	if goals[0].CreatedAt == 0 {
		t.Fatal("created_at was not parsed")
	}

	// The legacy table must be gone.
	var n int
	if err := store.goals.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE name='goals_legacy'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("goals_legacy was not dropped")
	}
}
