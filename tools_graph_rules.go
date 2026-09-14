// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// MCP tools for inference over the knowledge graph.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// maxInferenceEdges bounds the program sent to Prolog. The graph is small
// enough today that the limit never bites; it exists so that growth turns into
// a stated bound rather than a request that never returns.
const maxInferenceEdges = 5000

// graphInferTool derives facts the stored edges imply but do not state.
func graphInferTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_infer",
			mcp.WithDescription(`Derive facts the graph implies but does not store, using the versioned rules in graph_rules.pl.
Predicates: inverse_of (a relation whose reverse is missing), together (people at the same place on the same day), serves (a place serves a dish, derived from an order during a visit), contradiction (a relation holding two values where only one is possible).
Derived facts are answers, not rows: nothing is written to graph_edges.`),
			mcp.WithString("predicate",
				mcp.Description("Run one predicate only (inverse_of, together, serves, contradiction). Default: all of them."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			only, _ := args["predicate"].(string)

			res, err := graphInfer(s.goals, only, maxInferenceEdges)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			out, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf(
				"Inference over %d edges: %d derived facts.\n%s",
				res.EdgesConsidered, len(res.Derived), string(out))), nil
		},
	}
}

// graphVerifyTool reports what the graph asserts but cannot hold.
func graphVerifyTool(s *Storage) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("graph_verify",
			mcp.WithDescription(`Report what the graph asserts but cannot hold: contradictions derived by graph_rules.pl (a relation that admits one value per subject holding two), and type violations where an edge contradicts the relation vocabulary.
Nothing is changed: the result is a list of things to look at.`),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			res, err := graphVerify(s.goals, maxInferenceEdges)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			if len(res.Issues) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Graph is consistent: no contradiction and no type violation across %d edges.",
					res.EdgesConsidered)), nil
			}
			out, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf(
				"Checked %d edges: %d issue(s).\n%s",
				res.EdgesConsidered, len(res.Issues), string(out))), nil
		},
	}
}
