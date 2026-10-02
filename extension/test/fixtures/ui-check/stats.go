package accesslog

import (
	"math"
	"sort"
	"time"
)

// Summary は複数のエントリをまとめた集計。
type Summary struct {
	Count  int
	Errors int // ステータスが 400 以上の件数
	Bytes  int
	AvgMS  float64
	MaxMS  float64
}

// Summarize はエントリを集計する。
func Summarize(entries []Entry) Summary {
	s := Summary{Count: len(entries)}
	if s.Count == 0 {
		return s
	}
	totalMS := 0.0
	for _, e := range entries {
		if e.Status >= 400 {
			s.Errors++
		}
		s.Bytes += e.Bytes
		totalMS += e.LatencyMS
		s.MaxMS = math.Max(s.MaxMS, e.LatencyMS)
	}
	s.AvgMS = totalMS / float64(s.Count)
	return s
}

// Merge は別々に集計した2つの Summary を1つにまとめる（ログを分割して処理するとき用）。
func Merge(a, b Summary) Summary {
	out := Summary{
		Count:  a.Count + b.Count,
		Errors: a.Errors + b.Errors,
		Bytes:  a.Bytes + b.Bytes,
		MaxMS:  math.Max(a.MaxMS, b.MaxMS),
	}
	if out.Count > 0 {
		out.AvgMS = (a.AvgMS*float64(a.Count) + b.AvgMS*float64(b.Count)) / float64(out.Count)
	}
	return out
}

// ErrorRate はエラーの割合（パーセント）を返す。件数が 0 なら 0。
func (s Summary) ErrorRate() float64 {
	if s.Count == 0 {
		return 0
	}
	return float64(s.Errors) * 100 / float64(s.Count)
}

// Percentile は応答時間の p パーセンタイル（nearest-rank 法）を返す。
// 空のときは 0。エントリの並びは変えない。
func Percentile(entries []Entry, p float64) float64 {
	if len(entries) == 0 {
		return 0
	}
	ms := make([]float64, len(entries))
	for i, e := range entries {
		ms[i] = e.LatencyMS
	}
	sort.Float64s(ms)
	rank := int(math.Ceil(p / 100 * float64(len(ms))))
	rank = min(max(rank, 1), len(ms))
	return ms[rank-1]
}

// Bucket は1分ぶんのリクエスト数。
type Bucket struct {
	Start time.Time // 分の先頭
	Count int
}

// PerMinute は分ごとのリクエスト数を、時刻の早い順に返す。
func PerMinute(entries []Entry) []Bucket {
	counts := make(map[time.Time]int)
	for _, e := range entries {
		counts[e.Time.Truncate(time.Minute)]++
	}
	buckets := make([]Bucket, 0, len(counts))
	for k, n := range counts {
		buckets = append(buckets, Bucket{Start: k, Count: n})
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Start.Before(buckets[j].Start) })
	return buckets
}

// PathCount はパスごとのリクエスト数。
type PathCount struct {
	Path  string
	Count int
}

// TopPaths はリクエストの多いパスを上位 n 件返す。同数ならパスの辞書順。
func TopPaths(entries []Entry, n int) []PathCount {
	counts := make(map[string]int)
	for _, e := range entries {
		counts[e.Path]++
	}
	out := make([]PathCount, 0, len(counts))
	for p, c := range counts {
		out = append(out, PathCount{p, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
