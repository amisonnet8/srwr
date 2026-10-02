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

const whyDescription = "理由を1文で。ユーザーとの会話と同じ言語で書く。人が読むためのもの。空や空白だけは不可"

// List returns the tools srwr provides.
func List() []Tool {
	return []Tool{
		{
			Name: Select,
			Description: "編集したい範囲を宣言する。ファイルの startLine から endLine 行（1始まり、両端を含む）を見て、" +
				"その範囲を編集するための範囲トークン（selection）と、範囲の現在の内容（lines）を返す。" +
				"replace には、このトークンをそのまま渡す（行番号や内容は渡さなくてよい）。" +
				"挿入したい位置は、endLine = startLine - 1 の空範囲で指す（startLine 行目の直前。ファイルの末尾への追記は startLine = 行数 + 1）。" +
				"既存のファイルだけが対象で、新しいファイルは作れない。" +
				"why は必須。なぜここを見るのかを書く（何をしているかの言い換えではなく、理由）。ユーザーとの会話と同じ言語で書く。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":      map[string]any{"type": "string", "minLength": 1, "description": "作業場からの相対パス"},
					"startLine": map[string]any{"type": "integer", "minimum": 1, "description": "開始行（1始まり）"},
					"endLine":   map[string]any{"type": "integer", "minimum": 0, "description": "終了行（両端を含む）。空範囲なら startLine - 1"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"file", "startLine", "endLine", "why"},
			},
		},
		{
			Name: Replace,
			Description: "select（または直前の replace）が返した範囲トークンの範囲を、newText に置き換える。" +
				"ファイルや行番号は渡さない。削除は newText を空文字列にする。挿入は、空範囲を select してから replace する。" +
				"戻り値の selection は置き換え後の範囲のトークンで、同じ箇所を続けて直すときは select し直さずに使える。" +
				"別の場所の編集で行がずれても、行番号は srwr が補正する。範囲と重なる編集があったときだけ selection_stale になるので、select し直す。" +
				"why は必須。なぜこう変えるのかを書く。ユーザーとの会話と同じ言語で書く。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selection": map[string]any{"type": "string", "minLength": 1, "description": "select または replace が返した範囲トークン（sel_…）"},
					"newText":   map[string]any{"type": "string", "description": "置き換え後のテキスト。空文字列は削除。改行は LF"},
					"why":       map[string]any{"type": "string", "minLength": 1, "pattern": `\S`, "description": whyDescription},
				},
				"required": []string{"selection", "newText", "why"},
			},
		},
	}
}
