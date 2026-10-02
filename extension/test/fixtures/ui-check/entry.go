package accesslog

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const maxLineLen = 8192 // これより長い行は壊れた行として扱う

// Entry はアクセスログの1行。
type Entry struct {
	Time      time.Time
	IP        string
	Method    string
	Path      string // クエリを除いたパス
	Query     string
	Status    int
	Bytes     int
	LatencyMS float64 // 応答時間（ミリ秒）
}

// ErrTooLong は行が maxLineLen を超えたときのエラー。
var ErrTooLong = errors.New("accesslog: line too long")

// msPerUnit は応答時間の単位を、ミリ秒に直すための倍率。
var msPerUnit = map[string]float64{"us": 0.001, "ms": 1, "s": 1000}

// parseLatency は "35ms"、"1.5s"、"500us" のような応答時間をミリ秒にする。
func parseLatency(s string) (float64, error) {
	for _, unit := range []string{"us", "ms", "s"} {
		if !strings.HasSuffix(s, unit) {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, unit), 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("accesslog: bad latency %q", s)
		}
		return v * msPerUnit[unit], nil
	}
	return 0, fmt.Errorf("accesslog: unknown latency unit %q", s)
}

// ParseLine は1行をパースする。形式は次のとおり。
//
//	時刻(RFC3339) IP メソッド パス[?クエリ] ステータス バイト数 応答時間
//
// バイト数が "-" のときは 0 とする。時刻は UTC にそろえる。
func ParseLine(line string) (Entry, error) {
	if len(line) > maxLineLen {
		return Entry{}, ErrTooLong
	}
	f := strings.Fields(line)
	if len(f) != 7 {
		return Entry{}, fmt.Errorf("accesslog: want 7 fields, got %d", len(f))
	}
	t, err := time.Parse(time.RFC3339, f[0])
	if err != nil {
		return Entry{}, fmt.Errorf("accesslog: bad time: %w", err)
	}
	status, err := strconv.Atoi(f[4])
	if err != nil || status < 100 || status > 599 {
		return Entry{}, fmt.Errorf("accesslog: bad status %q", f[4])
	}
	n := 0
	if f[5] != "-" {
		if n, err = strconv.Atoi(f[5]); err != nil || n < 0 {
			return Entry{}, fmt.Errorf("accesslog: bad bytes %q", f[5])
		}
	}
	ms, err := parseLatency(f[6])
	if err != nil {
		return Entry{}, err
	}
	path, query, _ := strings.Cut(f[3], "?")
	return Entry{
		Time: t.UTC(), IP: f[1], Method: f[2], Path: path, Query: query,
		Status: status, Bytes: n, LatencyMS: ms,
	}, nil
}

// ParseAll は複数行をパースする。空行と # で始まる行は読み飛ばし、
// 壊れた行は捨てて bad に数える。
func ParseAll(text string) (entries []Entry, bad int) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e, err := ParseLine(line)
		if err != nil {
			bad++
			continue
		}
		entries = append(entries, e)
	}
	return entries, bad
}
