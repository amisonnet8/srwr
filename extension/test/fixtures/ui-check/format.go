package accesslog

import (
	"fmt"
	"strings"
)

// HumanBytes はバイト数を 1024 進法の読みやすい形にする。
func HumanBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, unit := range []string{"KiB", "MiB", "GiB"} {
		v /= 1024
		if v < 1024 || unit == "GiB" {
			return fmt.Sprintf("%.1f %s", v, unit)
		}
	}
	return ""
}

// busiest は最もリクエストの多い分を返す。同数なら早いほう。
func busiest(bs []Bucket) (Bucket, bool) {
	var best Bucket
	found := false
	for _, b := range bs {
		if !found || b.Count > best.Count {
			best, found = b, true
		}
	}
	return best, found
}

// Report はログ全体の要約を文章にする。top は表示するパスの数。
func Report(text string, top int) string {
	entries, bad := ParseAll(text)
	s := Summarize(entries)
	var sb strings.Builder
	fmt.Fprintf(&sb, "リクエスト %d件（不正な行 %d件）\n", s.Count, bad)
	fmt.Fprintf(&sb, "エラー率 %.1f%%\n", s.ErrorRate())
	fmt.Fprintf(&sb, "転送量 %s\n", HumanBytes(s.Bytes))
	fmt.Fprintf(&sb, "応答時間 平均 %.1fms / p50 %.1fms / p95 %.1fms / 最大 %.1fms\n",
		s.AvgMS, Percentile(entries, 50), Percentile(entries, 95), s.MaxMS)
	if b, ok := busiest(PerMinute(entries)); ok {
		fmt.Fprintf(&sb, "最多の分 %s（%d件）\n", b.Start.Format("2006-01-02 15:04"), b.Count)
	}
	sb.WriteString("上位パス:\n")
	for i, pc := range TopPaths(entries, top) {
		fmt.Fprintf(&sb, "%d. %s %d\n", i+1, pc.Path, pc.Count)
	}
	return sb.String()
}
