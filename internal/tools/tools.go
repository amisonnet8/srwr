// Package tools defines the two MCP tools, select and replace: their names, the text the AI reads
// and the schema of their input. What they do is in internal/core.
package tools

// Tool is an entry of tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Names of the tools.
const (
	Select  = "select"
	Replace = "replace"
)

const whyDescription = "The reason, in one sentence, in the language of the conversation with the user. It is for people to read. Empty or blank is not allowed"

// List returns the tools srwr provides.
func List() []Tool {
	return []Tool{
		{
			Name: Select,
			Description: "Declare the range you want to edit. Looks at lines startLine to endLine of the file (1-based, both inclusive) and returns " +
				"a selection token for editing that range (selection) and the current content of the range (lines). " +
				"Pass the token to replace as it is (no line numbers or content needed). " +
				"To point at a place to insert, use an empty range with endLine = startLine - 1 (just before line startLine; to append to the end of the file, startLine = number of lines + 1). " +
				"Only existing files can be selected; a new file cannot be created. " +
				"why is required: say why you look here (the reason, not a rephrasing of what you do), in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":      map[string]any{"type": "string", "minLength": 1, "description": "Path relative to the workspace"},
					"startLine": map[string]any{"type": "integer", "minimum": 1, "description": "First line (1-based)"},
					"endLine":   map[string]any{"type": "integer", "minimum": 0, "description": "Last line (inclusive); startLine - 1 for an empty range"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"file", "startLine", "endLine", "why"},
			},
		},
		{
			Name: Replace,
			Description: "Replace the range of the selection token returned by select (or by the previous replace) with newText. " +
				"Do not pass a file or line numbers. To delete, make newText an empty string. To insert, select an empty range and then replace. " +
				"The selection in the result is the token of the range after the replacement; use it to go on fixing the same place without calling select again. " +
				"If edits elsewhere shift the lines, srwr corrects the line numbers. Only an edit that overlaps the range makes the result selection_stale; then call select again. " +
				"why is required: say why you change it this way, in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selection": map[string]any{"type": "string", "minLength": 1, "description": "The selection token (sel_…) returned by select or replace"},
					"newText":   map[string]any{"type": "string", "description": "The text to put in. An empty string deletes. Line breaks are LF"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"selection", "newText", "why"},
			},
		},
	}
}
