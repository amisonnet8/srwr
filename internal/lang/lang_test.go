package lang

import "testing"

func TestJa(t *testing.T) {
	for env, want := range map[string]bool{"": false, "en": false, "ja": true, "ja_JP.UTF-8": true, "JA": true, " ja": true, "jp": false, "xja": false} {
		t.Setenv("SRWR_LANG", env)
		if Ja() != want {
			t.Errorf("SRWR_LANG=%q: Ja() = %v, want %v", env, Ja(), want)
		}
	}
}

func TestPickAndSprintf(t *testing.T) {
	t.Setenv("SRWR_LANG", "")
	if got := Pick("en", "ja"); got != "en" {
		t.Errorf("Pick = %q", got)
	}
	t.Setenv("SRWR_LANG", "ja")
	if got := Sprintf("%d files", "%d 件", 3); got != "3 件" {
		t.Errorf("Sprintf = %q", got)
	}
}
