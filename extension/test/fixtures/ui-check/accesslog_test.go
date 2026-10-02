package accesslog

import (
	"math"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func at(h, m, s int) time.Time {
	return time.Date(2026, 3, 1, h, m, s, 0, time.UTC)
}

func TestParseLine(t *testing.T) {
	e, err := ParseLine("2026-03-01T19:15:30+09:00 10.0.0.1 GET /api/users?id=3 200 512 35ms")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Time.Equal(at(10, 15, 30)) || e.Path != "/api/users" || e.Query != "id=3" || e.Status != 200 || e.Bytes != 512 {
		t.Errorf("entry = %+v", e)
	}
	e, err = ParseLine("2026-03-01T10:15:30Z 10.0.0.1 POST /login 302 - 12ms")
	if err != nil || e.Bytes != 0 || e.Query != "" {
		t.Errorf("entry = %+v, err = %v", e, err)
	}
	for _, bad := range []string{"", "a b c", "2026-03-01T10:15:30Z 10.0.0.1 GET / 999 1 1ms", "2026-03-01T10:15:30Z 10.0.0.1 GET / 200 1 1min"} {
		if _, err := ParseLine(bad); err == nil {
			t.Errorf("ParseLine(%q): want error", bad)
		}
	}
}

func TestParseLatencyUnits(t *testing.T) {
	cases := map[string]float64{"35ms": 35, "1.5s": 1500, "0.25s": 250, "500us": 0.5}
	for in, want := range cases {
		got, err := parseLatency(in)
		if err != nil || !near(got, want) {
			t.Errorf("parseLatency(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}

func TestFilters(t *testing.T) {
	entries := []Entry{
		{Time: at(10, 0, 0), Method: "GET", Path: "/api/a", Status: 200, LatencyMS: 10},
		{Time: at(10, 30, 0), Method: "POST", Path: "/api/b", Status: 500, LatencyMS: 300},
		{Time: at(11, 0, 0), Method: "GET", Path: "/index", Status: 404, LatencyMS: 5},
	}
	if got := Apply(entries, ByStatusClass(4)); len(got) != 1 || got[0].Path != "/index" {
		t.Errorf("4xx = %+v", got)
	}
	if got := Apply(entries, And(ByMethod("get"), ByPathPrefix("/api"))); len(got) != 1 || got[0].Path != "/api/a" {
		t.Errorf("get+api = %+v", got)
	}
	if got := Apply(entries, Between(at(10, 0, 0), at(11, 0, 0))); len(got) != 2 {
		t.Errorf("between = %+v", got)
	}
}

func TestSummarize(t *testing.T) {
	entries := []Entry{
		{Status: 200, Bytes: 100, LatencyMS: 10},
		{Status: 404, Bytes: 50, LatencyMS: 30},
		{Status: 500, Bytes: 0, LatencyMS: 20},
		{Status: 200, Bytes: 10, LatencyMS: 40},
	}
	s := Summarize(entries)
	if s.Count != 4 || s.Errors != 2 || s.Bytes != 160 || !near(s.AvgMS, 25) || !near(s.MaxMS, 40) {
		t.Errorf("summary = %+v", s)
	}
	if !near(s.ErrorRate(), 50) || Summarize(nil).ErrorRate() != 0 {
		t.Errorf("error rate = %v", s.ErrorRate())
	}
}

func TestMerge(t *testing.T) {
	a := Summarize([]Entry{{Status: 200, Bytes: 10, LatencyMS: 100}})
	b := Summarize([]Entry{{Status: 200, LatencyMS: 100}, {Status: 500, LatencyMS: 200}, {Status: 200, LatencyMS: 300}})
	m := Merge(a, b)
	if m.Count != 4 || m.Errors != 1 || m.Bytes != 10 || !near(m.MaxMS, 300) || !near(m.AvgMS, 175) {
		t.Errorf("merged = %+v", m)
	}
	if got := Merge(a, Summary{}); !near(got.AvgMS, 100) {
		t.Errorf("merge with empty: AvgMS = %v", got.AvgMS)
	}
}

func TestPercentile(t *testing.T) {
	var entries []Entry
	for _, ms := range []float64{50, 10, 40, 20, 30} {
		entries = append(entries, Entry{LatencyMS: ms})
	}
	cases := map[float64]float64{0: 10, 20: 10, 50: 30, 80: 40, 95: 50, 100: 50}
	for p, want := range cases {
		if got := Percentile(entries, p); !near(got, want) {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
	if entries[0].LatencyMS != 50 || Percentile(nil, 50) != 0 {
		t.Errorf("input changed or empty result wrong")
	}
}

func TestPerMinute(t *testing.T) {
	entries := []Entry{
		{Time: at(11, 59, 20)}, {Time: at(10, 59, 10)}, {Time: at(10, 59, 40)}, {Time: at(11, 0, 5)},
	}
	got := PerMinute(entries)
	if len(got) != 3 || got[0].Count != 2 || got[1].Count != 1 || got[2].Count != 1 {
		t.Fatalf("buckets = %+v", got)
	}
	if !got[0].Start.Equal(at(10, 59, 0)) || !got[1].Start.Equal(at(11, 0, 0)) || !got[2].Start.Equal(at(11, 59, 0)) {
		t.Errorf("order = %+v", got)
	}
}

func TestReport(t *testing.T) {
	log := `# sample
2026-03-01T10:59:10Z 10.0.0.1 GET /api/users?id=1 200 1024 20ms
2026-03-01T10:59:40Z 10.0.0.2 GET /api/users?id=2 200 2048 40ms

2026-03-01T11:00:05Z 10.0.0.1 POST /api/orders 500 - 1.5s
2026-03-01T11:59:20Z 10.0.0.3 GET /index.html 404 512 10ms
2026-03-01T11:59:50Z 10.0.0.3 GET /api/users?id=1 200 4096 30ms
broken line
`
	want := `リクエスト 5件（不正な行 1件）
エラー率 40.0%
転送量 7.5 KiB
応答時間 平均 320.0ms / p50 30.0ms / p95 1500.0ms / 最大 1500.0ms
最多の分 2026-03-01 10:59（2件）
上位パス:
1. /api/users 3
2. /api/orders 1
`
	if got := Report(log, 2); got != want {
		t.Errorf("Report =\n%s\nwant\n%s", got, want)
	}
}
