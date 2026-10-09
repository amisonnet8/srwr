package main

import (
	"encoding/json"
	"html"
	"os"
	"strings"
)

// WriteJSON writes the counts.
func WriteJSON(path string, reports []Report) error {
	b, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600) //nolint:gosec // a result file of this tool
}

// WritePage writes one table: a row for each form, a column for each file and one for the sum.
func WritePage(path string, reports []Report) error {
	total := map[string]int{}
	for _, r := range reports {
		for k, v := range r.Counts {
			total[k] += v
		}
	}
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>srwr usage</title>
<style>:root{--bg:#fff;--fg:#1a1a1a;--line:#ccc;--dim:#666}@media(prefers-color-scheme:dark){:root{--bg:#1a1a1a;--fg:#eee;--line:#444;--dim:#999}}
body{background:var(--bg);color:var(--fg);font:14px/1.5 system-ui,sans-serif;margin:16px}table{border-collapse:collapse}th,td{border:1px solid var(--line);padding:2px 8px;text-align:left}td.n{text-align:right}tr.g td{background:rgba(128,128,128,.15);font-weight:bold}.w{overflow-x:auto}</style></head><body>
<h1>How the AI used srwr</h1><p>Counts from the conversation files. A form used 0 times in one run is not proof that it is not needed.</p><div class="w"><table><tr><th>form</th>`)
	for _, r := range reports {
		sb.WriteString("<th>" + html.EscapeString(r.File) + "</th>")
	}
	sb.WriteString("<th>total</th></tr>\n")
	last := ""
	for _, k := range Sorted(total) {
		if g := group(k); g != last {
			last = g
			sb.WriteString("<tr class=\"g\"><td colspan=\"" + itoa(len(reports)+2) + "\">" + html.EscapeString(g) + "</td></tr>\n")
		}
		sb.WriteString("<tr><td>" + html.EscapeString(k) + "</td>")
		for _, r := range reports {
			sb.WriteString("<td class=\"n\">" + itoa(r.Counts[k]) + "</td>")
		}
		sb.WriteString("<td class=\"n\">" + itoa(total[k]) + "</td></tr>\n")
	}
	sb.WriteString("</table></div></body></html>\n")
	return os.WriteFile(path, []byte(sb.String()), 0o600)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
