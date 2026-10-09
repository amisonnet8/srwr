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

// editsSchema is the input of edit with edits: the items have the inputs of a single edit, and the why is shared.
func editsSchema() map[string]any {
	return map[string]any{
		"type": "array", "minItems": 1, "maxItems": 50,
		"description": "Several edits in one call, sharing the why. Give them instead of selection, file, startLine, endLine, expect, newText, insert, old and new at the top. All or none",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"selection": map[string]any{"type": "string", "minLength": 1, "description": "A selection token. Without it, give file"},
				"file":      map[string]any{"type": "string", "minLength": 1, "description": "Path relative to the workspace. Only when there is no selection"},
				"startLine": map[string]any{"type": "integer", "minimum": 1, "description": "First line, as you saw it. Give both startLine and endLine, or neither"},
				"endLine":   map[string]any{"type": "integer", "minimum": 0, "description": "Last line (inclusive); startLine - 1 inserts before startLine"},
				"expect":    map[string]any{"type": "string", "description": "The lines the range holds now, joined with \\n (whole lines). Required unless inserting"},
				"newText":   map[string]any{"type": "string", "description": "The text to put in. An empty string deletes. Not with old and new"},
				"insert":    map[string]any{"type": "string", "enum": []string{"after", "before", "start", "end"}, "description": "Keep the range and put newText after (or before) it. \"start\" or \"end\": with file only, put newText at the top (bottom) of the file"},
				"old":       map[string]any{"type": "string", "minLength": 1, "description": "With file: the text to replace, in one place only"},
				"new":       map[string]any{"type": "string", "description": "With old: the text that takes its place"},
				"content":   map[string]any{"type": "string", "description": "Makes the new file named by file, with this content (what new does; the file must not exist). Give file and content only"},
			},
		},
	}
}

// List returns the tools srwr provides.
func List() []Tool {
	return []Tool{
		{
			Name: Look,
			Description: "Look at a range you want to read or edit. To read several files at once, use the looks argument of this tool (it is an argument of look, not a tool of its own). Looks at lines startLine to endLine of the file (1-based, both inclusive) and returns " +
				"a selection token for editing that range with edit (selection) and the current content of the range (lines). " +
				"The result also has startLine and endLine (the range that was selected). " +
				"Pass the token to edit as it is (no line numbers or content needed). " +
				"Line numbers can be wrong, so pass expect too: the lines the range must hold, joined with \\n. If they differ, the look is refused and the message says where those lines are. expect is whole lines: a part of a line is not found. If a place differs from expect only in spaces or tabs, or holds it only as part of a line, the error has it in nearMatches: copy expect from there. When that is one place, the error also has retry: the arguments of the call to make again (add why). " +
				"With no startLine, endLine or expect, the whole file is read (the first 2000 lines of a longer one; the result has a note). Or pass only expect, without startLine and endLine: srwr finds those consecutive lines (exactly one place is needed; give line numbers if they appear in more than one). " +
				"To point at a place to insert, use an empty range with endLine = startLine - 1 (just before line startLine; to append to the end of the file, startLine = number of lines + 1). " +
				"An endLine past the end of the file is cut to the last line (the result has lineCount and a note); a startLine past the end is an error. " +
				"To read several files at once, pass looks instead of file: a list of 1 to 10 items, each with file (and startLine and endLine, or neither for the whole file), up to 2000 lines in all; the result has looks, one result with a token for each item. " +
				"Only existing files can be selected; create a new file with new. " +
				"why is required: say what you look for or why you look here (the reason, not a rephrasing of what you do), in the language of the conversation with the user.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":      map[string]any{"type": "string", "minLength": 1, "description": "Path relative to the workspace"},
					"startLine": map[string]any{"type": "integer", "minimum": 1, "description": "First line (1-based). Give both startLine and endLine, or neither (then the whole file is read)"},
					"endLine":   map[string]any{"type": "integer", "minimum": 0, "description": "Last line (inclusive); startLine - 1 for an empty range. Give both startLine and endLine, or neither"},
					"expect":    map[string]any{"type": "string", "description": "The lines the range must hold, joined with \\n (whole lines, whitespace counts). Without startLine and endLine, the range is where these lines are"},
					"search":    map[string]any{"type": "string", "description": "Instead of a range: a text to find in the file (plain text, one line, case counts). Returns every line that holds it, up to 20, each with a token (in a directory, 20 in all, each with its file). A line longer than 200 characters is cut to the part around the text (cut: true; the token is of the whole line) Not with startLine, endLine or expect"},
					"include":   map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1}, "description": "With search: search only the files that match one of these patterns (no ! here: to leave files out use exclude) (.gitignore syntax: \"*.go\", \"internal/\", \"docs/**/*.md\"; a pattern with no / matches the name at any depth)"},
					"exclude":   map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1}, "description": "With search: leave out the files that match one of these patterns (same syntax as include; a pattern starting with ! takes back an earlier pattern of exclude)"},
					"offset":    map[string]any{"type": "integer", "minimum": 0, "description": "With search: skip this many matches, then return the next 20 (use it when more is more than 0)"},
					"looks": map[string]any{"type": "array", "minItems": 1, "maxItems": 10, "description": "Instead of file: several files in one call, sharing the why. Each item has file, and startLine and endLine (both, or neither for the whole file). Up to 10 items and 2000 lines in all. All or none; each item gets a token. Not with file, startLine, endLine, expect or search",
						"items": map[string]any{"type": "object", "properties": map[string]any{
							"file":      map[string]any{"type": "string", "minLength": 1, "description": "Path relative to the workspace"},
							"startLine": map[string]any{"type": "integer", "minimum": 1, "description": "First line (1-based). Give both startLine and endLine, or neither"},
							"endLine":   map[string]any{"type": "integer", "minimum": 0, "description": "Last line (inclusive)"},
						}, "required": []string{"file"}}},
					"why": map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"why"},
			},
		},
		{
			Name: Edit,
			Description: "Replace a range with newText, in one call. Two ways to point at the range. " +
				"With a selection token returned by look (or by the previous edit or new): pass selection, newText and why, no file or line numbers. This is the usual way: look first, then edit. " +
				"Without a token: pass file and expect, the lines the range must hold joined with \\n (whitespace counts), and, if you know them, startLine and endLine. srwr then finds the range by itself: " +
				"the lines you gave, those lines moved to where they are after edits made since your last look at the file (so edits to one file can be sent together in any order), or the one place where expect is in the file. " +
				"If expect is in the file in more than one place, or not at all, nothing is changed and the error says so; give startLine and endLine, or more lines in expect. expect is whole lines: a part of a line is not found (to change a part of a line, use old and new, below). If a place differs from expect only in spaces or tabs, or holds it only as part of a line, the error has it in nearMatches: copy expect from there. When that is one place, the error also has retry: the arguments of the call to make again (add why). " +
				"To insert next to lines, point at them as usual (selection, or file and expect) and add insert: \"after\" or \"before\": those lines are kept and newText goes after (before) them; an empty newText with insert puts one empty line (without insert it deletes). This works however the file has changed, because expect is checked. " +
				"To put text at the top or the bottom of a file, pass file, newText and insert: \"start\" or \"end\" (no expect or line numbers needed; it works whatever the file has become). To insert at another place that is not next to any lines you can name, give startLine and endLine = startLine - 1 and no expect: this works only if the file has not changed since your last look at it. " +
				"To change a part of a line (not the whole line), pass file, old and new instead of expect and newText: old is a text that is in the file in one place only (it may have several lines; give startLine and endLine to look in those lines only), and the lines it touches become what they would be with new in its place. " +
				"To delete, make newText an empty string. " +
				"To make the same kind of change in several places, or in several files, with one why, pass edits instead of the single-edit inputs: a list of 1 to 50 items, each with selection, or file with expect (and startLine and endLine), or file with old and new, and newText and insert; or file with content, which makes a new file (so a new file and the code that uses it go in one call). " +
				"All the ranges are found as the files are now, before any item is made, so the items do not depend on each other and may come in any order; they must not overlap. If any item fails, nothing is changed and the error says which one (edits[i]): fix it and send all the items again. The result has edits, one result for each item, in the order given. " +
				"With brief: true the result is only selection, startLine and endLine for each edit (no lines, above, below): use it for many edits when you do not need to read them back. The result has selection (the token of the range after the replacement; use it to go on fixing the same place), lines (the content of the range now) and above and below (up to 2 lines of the file now, just above and just below the new range; they are not the old content), so you can check the edit without reading the file again. " +
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
					"newText":   map[string]any{"type": "string", "description": "The text to put in. An empty string deletes. Line breaks are LF. Not with old and new"},
					"old":       map[string]any{"type": "string", "minLength": 1, "description": "With file, to change a part of the text: the text to replace, in the file in one place only (several lines are fine). Not with selection, expect, newText or insert"},
					"new":       map[string]any{"type": "string", "description": "With old: the text that takes its place. An empty string deletes old"},
					"edits":     editsSchema(),
					"brief":     map[string]any{"type": "boolean", "description": "true: the result has only selection, startLine and endLine for each edit (no lines, above, below). With edits of 6 or more items this is the default; write false to get the lines. Write true or false without quotes"},
					"insert":    map[string]any{"type": "string", "enum": []string{"after", "before", "start", "end"}, "description": "Keep the range (the selection, or the lines of expect) and put newText after (or before) it, instead of replacing it. Not with an empty range. \"start\" or \"end\": with file and newText only (no selection, expect or line numbers), put newText at the top (bottom) of the file. An empty newText puts one empty line"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"why"},
			},
		},
		{
			Name: Replace,
			Description: "Replace a text with another in 2 or more places, in one file or several, like a simple sed, and record why. For one place, use edit (or look and edit): replace is refused for it, with where the place is and the edit call to make. " +
				"old is searched for as plain text (not a regular expression), left to right, places do not overlap. " +
				"count is how many places you expect in all the files together, 2 or more. If the number found is different, nothing is changed and the error says how many there are in each file (and, in nearMatches, the places that differ from old only in spaces or tabs); this is the check that you changed what you meant. " +
				"The result has, for each file that changed, count (how many places) and hits, one for each place (places on the same line are one): startLine and endLine (after the change), lines (what they hold now), and above and below (the line above and the line below, as the file is now). There is no selection token; use look and edit for a place you want to go on with. " +
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
