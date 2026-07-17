package main

import (
	"testing"
)

func TestRootCommand(t *testing.T) {
	// Verify root command exists and has expected usage
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("newRootCmd() returned nil")
	}
	if cmd.Use != "memory-cli" {
		t.Fatalf("root command Use = %q, want memory-cli", cmd.Use)
	}
	if len(cmd.Commands()) != 13 {
		t.Fatalf("root command has %d subcommands, want 13", len(cmd.Commands()))
	}
}

func TestSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expected := []string{
		"save", "get", "delete", "search", "list",
		"context", "extract", "goals", "timeline", "suggest",
		"find", "dig", "session",
	}
	names := make(map[string]bool)
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}

	for _, name := range expected {
		if !names[name] {
			t.Fatalf("missing subcommand: %q", name)
		}
	}
}
