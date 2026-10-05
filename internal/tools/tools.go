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
				"Line numbers can be wrong, so pass expect too: the lines the range must hold, joined with \\n. If they differ, the look is refused and the message says where those lines are. expect is whole lines: a part of a line is not found. If a place differs from expect only in spaces or tabs, or holds it only as part of a line, the error has it in nearMatches: copy expect from there. " +
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
					"expect":    map[string]any{"type": "string", "description": "The lines the range must hold, joined with \\n (whole lines, whitespace counts). Without startLine and endLine, the range is where these lines are"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"file", "why"},
			},
		},
		{
			Name: Edit,
			Description: "Replace a range with newText, in one call. Two ways to point at the range. " +
				"With a selection token returned by look (or by the previous edit or new): pass selection, newText and why, no file or line numbers. This is the usual way: look first, then edit. " +
				"Without a token: pass file and expect, the lines the range must hold joined with \\n (whitespace counts), and, if you know them, startLine and endLine. srwr then finds the range by itself: " +
				"the lines you gave, those lines moved to where they are after edits made since your last look at the file (so edits to one file can be sent together in any order), or the one place where expect is in the file. " +
				"If expect is in the file in more than one place, or not at all, nothing is changed and the error says so; give startLine and endLine, or more lines in expect. expect is whole lines: a part of a line is not found. If a place differs from expect only in spaces or tabs, or holds it only as part of a line, the error has it in nearMatches: copy expect from there. " +
				"To insert next to lines, point at them as usual (selection, or file and expect) and add insert: \"after\" or \"before\": those lines are kept and newText goes after (before) them. This works however the file has changed, because expect is checked. " +
				"To insert at a place that is not next to any lines you can name, give startLine and endLine = startLine - 1 and no expect: this works only if the file has not changed since your last look at it. " +
				"To delete, make newText an empty string. " +
				"The result has selection (the token of the range after the replacement; use it to go on fixing the same place), lines (the content of the range now) and before and after (up to 2 lines around it), so you can check the edit without reading the file again. " +
				"With a token, edits elsewhere shift the lines and srwr corrects the line numbers; only an edit that overlaps the range makes the result selection_stale, and then you call look again. " +
				"why is required: say why you change it this way, in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selection": map[string]any{"type": "string", "minLength": 1, "description": "The selection token (sel_…) returned by look, edit or new. Without it, give file and expect"},
					"file":      map[string]any{"type": "string", "minLength": 1, "description": "Path relative to the workspace. Only when there is no selection"},
					"startLine": map[string]any{"type": "integer", "minimum": 1, "description": "With file: first line (1-based) of the range, as you saw it. Give both startLine and endLine, or neither"},
					"endLine":   map[string]any{"type": "integer", "minimum": 0, "description": "With file: last line (inclusive); startLine - 1 inserts before startLine"},
					"expect":    map[string]any{"type": "string", "description": "With file: the lines the range holds now, joined with \\n (whole lines, whitespace counts). Required unless inserting"},
					"newText":   map[string]any{"type": "string", "description": "The text to put in. An empty string deletes. Line breaks are LF"},
					"insert":    map[string]any{"type": "string", "enum": []string{"after", "before"}, "description": "Keep the range (the selection, or the lines of expect) and put newText after (or before) it, instead of replacing it. Not with an empty range; newText must not be empty"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"newText", "why"},
			},
		},
		{
			Name: Replace,
			Description: "Replace a text with another in 2 or more places, in one file or several, like a simple sed, and record why. For one place, use edit (or look and edit): replace is refused for it, with where the place is and the edit call to make. " +
				"old is searched for as plain text (not a regular expression), left to right, places do not overlap. " +
				"count is how many places you expect in all the files together, 2 or more. If the number found is different, nothing is changed and the error says how many there are in each file (and, in nearMatches, the places that differ from old only in spaces or tabs); this is the check that you changed what you meant. " +
				"The result has, for each file that changed, count (how many places) and hits, one for each place (places on the same line are one): startLine and endLine (after the change), lines (what they hold now), and before and after (the line before and the line after). There is no selection token; use look and edit for a place you want to go on with. " +
				"Use look and edit instead when the places must be chosen one by one. " +
				"why is required: say why you change it, in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"files": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1}, "description": "Paths relative to the workspace. Existing files only"},
					"old":   map[string]any{"type": "string", "minLength": 1, "description": "The text to look for. It may have several lines (LF)"},
					"new":   map[string]any{"type": "string", "description": "The text to put in its place. An empty string deletes it"},
					"count": map[string]any{"type": "integer", "minimum": 1, "description": "How many places you expect in all the files together. 2 or more (1 is answered with use_edit)"},
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
