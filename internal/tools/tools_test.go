package tools

import (
	"strings"
	"testing"
)

// What the AI sees must name every input the server takes: an input the schema leaves out is one the AI never uses.
func TestEditToolNamesItsInputs(t *testing.T) {
	var edit Tool
	for _, tl := range List() {
		if tl.Name == Edit {
			edit = tl
		}
	}
	props, _ := edit.InputSchema["properties"].(map[string]any)
	for _, name := range []string{"selection", "file", "startLine", "endLine", "expect", "newText", "insert", "old", "new", "edits", "brief", "why"} {
		if _, ok := props[name]; !ok {
			t.Errorf("edit has no input %q", name)
		}
	}
	if b, _ := props["brief"].(map[string]any); b["type"] != "boolean" {
		t.Errorf("brief = %v: it must be a boolean", b)
	}
	items, _ := props["edits"].(map[string]any)["items"].(map[string]any)
	iprops, _ := items["properties"].(map[string]any)
	for _, name := range []string{"selection", "file", "startLine", "endLine", "expect", "newText", "insert", "old", "new", "content"} {
		if _, ok := iprops[name]; !ok {
			t.Errorf("an item of edits has no input %q", name)
		}
	}
	if req, _ := edit.InputSchema["required"].([]string); len(req) != 1 || req[0] != "why" {
		t.Errorf("required = %v: newText is not needed with old and new, or edits", req)
	}
	for _, word := range []string{"edits", "old and new", "above and below"} {
		if !strings.Contains(edit.Description, word) {
			t.Errorf("the description of edit does not mention %q", word)
		}
	}
}

// The inputs of a search of a directory are in the schema of look.
func TestLookToolNamesTheSearchInputs(t *testing.T) {
	for _, tl := range List() {
		if tl.Name != Look {
			continue
		}
		props, _ := tl.InputSchema["properties"].(map[string]any)
		for _, name := range []string{"search", "include", "exclude", "offset"} {
			if _, ok := props[name]; !ok {
				t.Errorf("look has no input %q", name)
			}
		}
		search, _ := props["search"].(map[string]any)["description"].(string)
		if !strings.Contains(search, "that one line only") {
			t.Errorf("search does not say what its selection stands for: %q", search)
		}
	}
}
