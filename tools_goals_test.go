package main

import (
	"reflect"
	"testing"
)

func TestNormalizeLabels(t *testing.T) {
	got := normalizeLabels([]string{" bug ", "", "mcp", "bug", " deploy "})
	want := []string{"bug", "mcp", "deploy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLabels() = %#v, want %#v", got, want)
	}
}

func TestParseGoalLabelsArg(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "comma separated", input: "bug, mcp,bug,, deploy", want: []string{"bug", "mcp", "deploy"}},
		{name: "json array", input: `["bug","mcp","bug"," deploy "]`, want: []string{"bug", "mcp", "deploy"}},
		{name: "invalid json", input: `["bug",`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGoalLabelsArg(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseGoalLabelsArg() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGoalLabelsArg() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseGoalLabelsArg() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCountSubtasksFromDescription(t *testing.T) {
	description := `
- [x] done
- [ ] todo
* [X] also done
+ [ ] another todo
- plain bullet
  - [x] nested done
`

	done, total := countSubtasksFromDescription(description)
	if done != 3 || total != 5 {
		t.Fatalf("countSubtasksFromDescription() = (%d, %d), want (3, 5)", done, total)
	}
}
