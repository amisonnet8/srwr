package textkit

import (
	"strings"
)

const ellipsis = "…"

// RuneWidth は端末での表示幅を返す。全角文字は 2、制御文字は 0、それ以外は 1。
func RuneWidth(r rune) int {
	switch {
	case r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6:
		return 2
	}
	return 1
}

// DisplayWidth は文字列の表示幅の合計。
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// Truncate は表示幅が w に収まるように文字列を切り詰める。
// 収まらないときは末尾を "…"（幅1）に置き換える。
func Truncate(s string, w int) string {
	if DisplayWidth(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := RuneWidth(r)
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString(ellipsis)
	return b.String()
}

// Align は列の寄せ方。
type Align int

const (
	Left Align = iota
	Right
	Center
)

// Pad は表示幅が w になるまで空白で埋める。すでに w 以上ならそのまま返す。
// Center で余りが奇数のときは右側を1つ多くする。
func Pad(s string, w int, a Align) string {
	gap := w - DisplayWidth(s)
	if gap <= 0 {
		return s
	}
	switch a {
	case Right:
		return strings.Repeat(" ", gap) + s
	case Center:
		left := gap / 2
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", gap-left)
	}
	return s + strings.Repeat(" ", gap)
}

// Wrap は text を表示幅 width 以内の行に折り返す。
// 単語は空白で区切られているものとし、幅より長い単語は折り返さずそのまま1行にする。
// 元の改行は保たれ、空行は空行のまま残る。
func Wrap(text string, width int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		lines = append(lines, wrapParagraph(para, width)...)
	}
	return lines
}

func wrapParagraph(p string, width int) []string {
	words := strings.Fields(p)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	curW := 0
	for _, w := range words {
		ww := DisplayWidth(w)
		switch {
		case cur == "":
			cur, curW = w, ww
		case curW+1+ww <= width:
			cur += " " + w
			curW += 1 + ww
		default:
			lines = append(lines, cur)
			cur, curW = w, ww
		}
	}
	return append(lines, cur)
}
