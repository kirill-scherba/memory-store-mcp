// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newGoalsCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var all bool
	var output string

	cmd := &cobra.Command{
		Use:   "goals [status]",
		Short: "Manage goals (list, create, update, delete, show)",
		Long: `Manage tracked goals.

Subcommands:
  goals [status]     List goals (default: active). Use --all or specify status.
  goals create       Create a new goal
  goals show <id>    Show a single goal with subtask breakdown
  goals update <id>  Update a goal
  goals delete <id>  Delete a goal

Examples:
  memory-cli goals
  memory-cli goals --all -o json
  memory-cli goals create --title "Refactor CLI" --priority 8 --labels "mcp,cli"
  memory-cli goals show goal/2026-05-05/abc123
  memory-cli goals update goal/2026-05-05/abc123 --status completed
  memory-cli goals delete goal/2026-05-05/abc123`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status := ""
			if len(args) > 0 {
				status = args[0]
			}
			if all {
				status = ""
			} else if status == "" {
				status = "active"
			}

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_goal_list", map[string]any{
				"status": status,
			})
			if err != nil {
				return err
			}

			format := ParseOutputFormat(output)
			fmt.Println(formatGoalsList(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Show all goals (active, completed, archived)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	cmd.AddCommand(newGoalsListCmd())
	cmd.AddCommand(newGoalsCreateCmd())
	cmd.AddCommand(newGoalsShowCmd())
	cmd.AddCommand(newGoalsUpdateCmd())
	cmd.AddCommand(newGoalsDeleteCmd())

	return cmd
}

// ---------------------------------------------------------------------------
// goals list (with backward compatibility: `goals [status]` also works via Run)
// ---------------------------------------------------------------------------

func newGoalsListCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var all bool
	var output string

	cmd := &cobra.Command{
		Use:   "list [status]",
		Short: "List goals",
		Long: `List user's goals. By default shows active goals.

Use --all to show all goals (active, completed, archived).
Or specify status explicitly: active, completed, archived.
Use -o to control output format: table (default), json, summary.

Examples:
  memory-cli goals list
  memory-cli goals list --all
  memory-cli goals list completed
  memory-cli goals list -o json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status := ""
			if len(args) > 0 {
				status = args[0]
			}
			if all {
				status = ""
			} else if status == "" {
				status = "active"
			}

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_goal_list", map[string]any{
				"status": status,
			})
			if err != nil {
				return err
			}

			format := ParseOutputFormat(output)
			fmt.Println(formatGoalsList(result, format))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Show all goals (active, completed, archived)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}

// ---------------------------------------------------------------------------
// goals create
// ---------------------------------------------------------------------------

func newGoalsCreateCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var title, description, deadline, labels string
	var priority int
	var output string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new goal",
		Long: `Create a new tracked goal with title, description, priority, labels, and deadline.

Examples:
  memory-cli goals create --title "Refactor CLI" --description "Improve output formatting"
  memory-cli goals create --title "Fix bug" --priority 9 --labels "bug,urgent" --deadline "2026-06-01"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return fmt.Errorf("title is required")
			}

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			params := map[string]any{
				"title": title,
			}
			if description != "" {
				params["description"] = description
			}
			if cmd.Flags().Changed("priority") {
				params["priority"] = float64(priority)
			}
			if deadline != "" {
				params["deadline"] = deadline
			}
			if labels != "" {
				params["labels"] = labels
			}

			result, err := rc.callTool("memory_goal_create", params)
			if err != nil {
				return err
			}

			format := ParseOutputFormat(output)
			if format == OutputTable || format == OutputSummary {
				if g, err := parseGoalJSON(result); err == nil && g != nil {
					fmt.Println(renderGoalDetail(g))
					return nil
				}
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().StringVar(&title, "title", "", "Goal title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Goal description")
	cmd.Flags().IntVar(&priority, "priority", 5, "Priority (0-10)")
	cmd.Flags().StringVar(&deadline, "deadline", "", "Deadline (ISO 8601)")
	cmd.Flags().StringVar(&labels, "labels", "", `Labels (comma-separated: "bug,mcp" or JSON: '["bug","mcp"]')`)
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}

// ---------------------------------------------------------------------------
// goals show <id>
// ---------------------------------------------------------------------------

func newGoalsShowCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var output string

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a single goal with subtask breakdown",
		Long: `Show detailed information about a single goal, including TODO subtasks
parsed from the description.

Examples:
  memory-cli goals show goal/2026-05-05/abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			// List goals with status="" to get all, then filter by id
			result, err := rc.callTool("memory_goal_list", map[string]any{
				"status": "",
			})
			if err != nil {
				return err
			}

			goals, err := parseGoalsJSON(result)
			if err != nil {
				return fmt.Errorf("failed to parse goals: %w", err)
			}

			// Find goal by id (full match or prefix match)
			var found *goalRow
			for _, g := range goals {
				if g.ID == id || strings.HasPrefix(g.ID, id) {
					found = &g
					break
				}
			}
			if found == nil {
				return fmt.Errorf("goal not found: %s", id)
			}

			format := ParseOutputFormat(output)
			switch format {
			case OutputJSON:
				fmt.Println(result)
			default:
				fmt.Println(renderGoalDetail(found))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}

// ---------------------------------------------------------------------------
// goals update <id>
// ---------------------------------------------------------------------------

func newGoalsUpdateCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string
	var title, description, status, deadline, labels string
	var priority, progress int
	var output string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a goal",
		Long: `Update an existing goal's fields. Only provided fields are changed.

Examples:
  memory-cli goals update goal/2026-05-05/abc123 --status completed
  memory-cli goals update goal/2026-05-05/abc123 --progress 80 --priority 7
  memory-cli goals update goal/2026-05-05/abc123 --title "New title" --labels "mcp,cli"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			params := map[string]any{
				"id": id,
			}
			if cmd.Flags().Changed("title") {
				params["title"] = title
			}
			if cmd.Flags().Changed("description") {
				params["description"] = description
			}
			if cmd.Flags().Changed("status") {
				params["status"] = status
			}
			if cmd.Flags().Changed("deadline") {
				params["deadline"] = deadline
			}
			if cmd.Flags().Changed("priority") {
				params["priority"] = float64(priority)
			}
			if cmd.Flags().Changed("progress") {
				params["progress"] = float64(progress)
			}
			if cmd.Flags().Changed("labels") {
				params["labels"] = labels
			}

			result, err := rc.callTool("memory_goal_update", params)
			if err != nil {
				return err
			}

			format := ParseOutputFormat(output)
			if format == OutputTable || format == OutputSummary {
				if g, err := parseGoalJSON(result); err == nil && g != nil {
					fmt.Println(renderGoalDetail(g))
					return nil
				}
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")
	cmd.Flags().StringVar(&title, "title", "", "New title")
	cmd.Flags().StringVar(&description, "description", "", "New description")
	cmd.Flags().StringVar(&status, "status", "", "New status: active, completed, archived")
	cmd.Flags().StringVar(&deadline, "deadline", "", "New deadline (ISO 8601)")
	cmd.Flags().IntVar(&priority, "priority", -1, "New priority (0-10)")
	cmd.Flags().IntVar(&progress, "progress", -1, "New progress (0-100)")
	cmd.Flags().StringVar(&labels, "labels", "", `New labels (comma-separated or JSON array). Use "[]" to clear.`)
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: json, table, summary")

	return cmd
}

// ---------------------------------------------------------------------------
// goals delete <id>
// ---------------------------------------------------------------------------

func newGoalsDeleteCmd() *cobra.Command {
	var dbPath, chatModel, serverURL string

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a goal",
		Long: `Delete a tracked goal by ID.

Examples:
  memory-cli goals delete goal/2026-05-05/abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			rc, err := newMemoryClient(dbPath, chatModel, serverURL)
			if err != nil {
				return err
			}
			defer rc.close()

			result, err := rc.callTool("memory_goal_delete", map[string]any{
				"id": id,
			})
			if err != nil {
				return err
			}

			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Path to memory-store-mcp database")
	cmd.Flags().StringVar(&chatModel, "chat-model", "", "Chat model")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MCP server URL (e.g. http://localhost:8080/mcp) for remote connection")

	return cmd
}
