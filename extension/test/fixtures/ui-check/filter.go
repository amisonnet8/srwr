package accesslog

import (
	"strings"
	"time"
)

// Filter はエントリを残すかどうかを決める条件。
type Filter func(Entry) bool

// Apply は条件に合うエントリだけを、元の順序で返す。
func Apply(entries []Entry, f Filter) []Entry {
	var out []Entry
	for _, e := range entries {
		if f(e) {
			out = append(out, e)
		}
	}
	return out
}

// And はすべての条件に合うときだけ true になる。
func And(fs ...Filter) Filter {
	return func(e Entry) bool {
		for _, f := range fs {
			if !f(e) {
				return false
			}
		}
		return true
	}
}

// ByStatusClass は 2xx、4xx のような区分（2、4 など）で絞る。
func ByStatusClass(class int) Filter {
	return func(e Entry) bool { return e.Status/100 == class }
}

// ByMethod は大文字小文字を区別せずにメソッドで絞る。
func ByMethod(methods ...string) Filter {
	return func(e Entry) bool {
		for _, m := range methods {
			if strings.EqualFold(m, e.Method) {
				return true
			}
		}
		return false
	}
}

// ByPathPrefix はパスの先頭で絞る。
func ByPathPrefix(prefix string) Filter {
	return func(e Entry) bool { return strings.HasPrefix(e.Path, prefix) }
}

// Between は from 以上 to 未満の時刻で絞る。
func Between(from, to time.Time) Filter {
	return func(e Entry) bool {
		return !e.Time.Before(from) && e.Time.Before(to)
	}
}
