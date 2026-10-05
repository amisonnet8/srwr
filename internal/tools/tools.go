// Package tools defines the MCP tools, look, edit, replace and new: their names, the text the AI reads
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
	Look    = "look"
	Edit    = "edit"
	Replace = "replace"
	New     = "new"
)

const whyDescription = "The reason, in one sentence, in the language of the conversation with the user. It is for people to read. Empty or blank is not allowed"

// List returns the tools srwr provides.
func List() []Tool {
	return []Tool{
		{
			Name: Look,
			Description: "Look at a range you want to read or edit. Looks at lines startLine to endLine of the file (1-based, both inclusive) and returns " +
				"a selection token for editing that range with edit (selection) and the current content of the range (lines). " +
				"The result also has startLine and endLine (the range that was selected). " +
				"Pass the token to edit as it is (no line numbers or content needed). " +
				"Line numbers can be wrong, so pass expect too: the lines the range must hold, joined with \\n. If they differ, the look is refused and the message says where those lines are. " +
				"Or leave out startLine and endLine and pass only expect: srwr finds those consecutive lines (exactly one place is needed; give line numbers if they appear in more than one). " +
				"To point at a place to insert, use an empty range with endLine = startLine - 1 (just before line startLine; to append to the end of the file, startLine = number of lines + 1). " +
				"Only existing files can be selected; create a new file with new. " +
				"why is required: say what you look for or why you look here (the reason, not a rephrasing of what you do), in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":      map[string]any{"type": "string", "minLength": 1, "description": "Path relative to the workspace"},
					"startLine": map[string]any{"type": "integer", "minimum": 1, "description": "First line (1-based)"},
					"endLine":   map[string]any{"type": "integer", "minimum": 0, "description": "Last line (inclusive); startLine - 1 for an empty range. Give both startLine and endLine, or neither"},
					"expect":    map[string]any{"type": "string", "description": "The lines the range must hold, joined with \\n (whitespace counts). Without startLine and endLine, the range is where these lines are"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"file", "why"},
			},
		},
		{
			Name: Edit,
			Description: "Replace the range of the selection token returned by look (or by the previous edit or new) with newText. " +
				"Do not pass a file or line numbers. To delete, make newText an empty string. To insert, look at an empty range and then edit. " +
				"The selection in the result is the token of the range after the replacement; use it to go on fixing the same place without calling look again. " +
				"The result also has lines (the content of the range now) and before and after (up to 2 lines around it), so you can check the edit without reading the file again. " +
				"If edits elsewhere shift the lines, srwr corrects the line numbers. Only an edit that overlaps the range makes the result selection_stale; then call look again. " +
				"why is required: say why you change it this way, in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selection": map[string]any{"type": "string", "minLength": 1, "description": "The selection token (sel_…) returned by look, edit or new"},
					"newText":   map[string]any{"type": "string", "description": "The text to put in. An empty string deletes. Line breaks are LF"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"selection", "newText", "why"},
			},
		},
		{
			Name: Replace,
			Description: "Replace a text with another in several files at once, like a simple sed, and record why. old is searched for as plain text (not a regular expression), left to right, places do not overlap. " +
				"count is how many places you expect in all the files together. If the number found is different, nothing is changed and the error says how many there are in each file; this is the check that you changed what you meant. " +
				"The result has, for each file that changed, hits (how many places), startLine and endLine (the lines from the first place to the last, after the change), selection (a token for those lines, usable by edit) and lines. " +
				"Use look and edit instead when the places must be chosen one by one. " +
				"why is required: say why you change it, in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"files": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1}, "description": "Paths relative to the workspace. Existing files only"},
					"old":   map[string]any{"type": "string", "minLength": 1, "description": "The text to look for. It may have several lines (LF)"},
					"new":   map[string]any{"type": "string", "description": "The text to put in its place. An empty string deletes it"},
					"count": map[string]any{"type": "integer", "minimum": 1, "description": "How many places you expect in all the files together"},
					"why":   map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"files", "old", "new", "count", "why"},
			},
		},
		{
			Name: New,
			Description: "Create a file that does not exist yet, with its content, and record why. If the file already exists, nothing is changed and the error is file_exists; use look and edit for it. " +
				"Directories above the file are created when they are missing. The file ends with a line break. " +
				"The result has selection (a token for the whole content, usable by edit), startLine and endLine. " +
				"why is required: say why you create the file, in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":    map[string]any{"type": "string", "minLength": 1, "description": "Path relative to the workspace. The file must not exist"},
					"content": map[string]any{"type": "string", "description": "The content of the file. Line breaks are LF. An empty string makes an empty file"},
					"why":     map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"file", "content", "why"},
			},
		},
	}
}
