package textkit

import (
	"reflect"
	"testing"
)

func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", 0}, {"abc", 3}, {"日本語", 6}, {"ab日本", 6}, {"ｱｲｳ", 3}, {"ＡＢ", 4}, {"a\tb", 2}, {"…", 1},
	}
	for _, c := range cases {
		if got := DisplayWidth(c.s); got != c.want {
			t.Errorf("DisplayWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		s    string
		w    int
		want string
	}{
		{"hello", 6, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"日本語", 6, "日本語"},
		{"日本語テキスト", 7, "日本語…"},
		{"日本語テキスト", 8, "日本語…"},
		{"日本", 3, "日…"},
		{"", 3, ""},
	}
	for _, c := range cases {
		if got := Truncate(c.s, c.w); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.w, got, c.want)
		}
	}
}

func TestPad(t *testing.T) {
	cases := []struct {
		s    string
		w    int
		a    Align
		want string
	}{
		{"ab", 5, Left, "ab   "},
		{"ab", 5, Right, "   ab"},
		{"ab", 6, Center, "  ab  "},
		{"ab", 5, Center, " ab  "},
		{"日本", 6, Right, "  日本"},
		{"日本", 7, Center, " 日本  "},
		{"abcdef", 3, Left, "abcdef"},
		{"abc", 3, Right, "abc"},
	}
	for _, c := range cases {
		if got := Pad(c.s, c.w, c.a); got != c.want {
			t.Errorf("Pad(%q, %d, %d) = %q, want %q", c.s, c.w, c.a, got, c.want)
		}
	}
}

func TestWrap(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"英語", "the quick brown fox jumps over", 10,
			[]string{"the quick", "brown fox", "jumps over"}},
		{"ちょうど収まる", "aaaa bbbb", 9, []string{"aaaa bbbb"}},
		{"1文字あふれる", "aaaa bbbb", 8, []string{"aaaa", "bbbb"}},
		{"日本語", "今日は 良い 天気 です", 11,
			[]string{"今日は 良い", "天気 です"}},
		{"日本語と英語", "今日は fine です", 9,
			[]string{"今日は", "fine です"}},
		{"長すぎる語", "a extraordinarily b", 5,
			[]string{"a", "extraordinarily", "b"}},
		{"空行と空白", "a   b\n\nc  d", 10, []string{"a b", "", "c d"}},
		{"空文字列", "", 10, []string{""}},
	}
	for _, c := range cases {
		if got := Wrap(c.text, c.width); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Wrap(%q, %d) = %q, want %q", c.name, c.text, c.width, got, c.want)
		}
	}
}
