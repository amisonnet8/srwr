package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var japanese = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`)

// english makes the tests of this file run with no SRWR_LANG, as a person who set nothing.
func english(t *testing.T) { t.Helper(); t.Setenv("SRWR_LANG", "") }

func TestEnglishIsTheDefault(t *testing.T) {
	english(t)
	code, out, _ := run([]string{"--help"}, "")
	if code != 0 || !strings.Contains(out, "Usage:") || japanese.MatchString(out) {
		t.Errorf("--help: %d\n%s", code, out)
	}
	code, _, errOut := run([]string{"foo"}, "")
	if code != 2 || !strings.HasPrefix(errOut, `srwr: unknown command "foo"`) || japanese.MatchString(errOut) {
		t.Errorf("unknown command: %d\n%s", code, errOut)
	}
	// SRWR_LANG=ja is what srwr said before.
	t.Setenv("SRWR_LANG", "ja")
	if _, out, _ := run([]string{"--help"}, ""); !strings.Contains(out, "使い方:") {
		t.Errorf("ja --help:\n%s", out)
	}
}

// The outputs the person approved at the UI gate of R10.5, in English.
func TestInitOutputsInEnglish(t *testing.T) {
	english(t)
	empty := `Workspace: /home/me/project

  created    .srwr/                key .srwr/key created
  created    .mcp.json             registered srwr mcp
  created    .claude/settings.json registered the hook; forbade Edit, Write, etc. (strict mode)
  created    .gitignore            .srwr/key .srwr/lock .srwr/active .srwr/init-backup/

Ready. Reopen Claude Code and select / replace are available.
For lenient mode (Edit and Write stay allowed), run: srwr init --lenient.
`
	root := gitDir(t)
	if code, out, _ := initOut(t, root); code != 0 || out != empty {
		t.Errorf("empty workspace: code %d\n%s", code, out)
	}
	again := `Workspace: /home/me/project

  unchanged  .srwr/
  unchanged  .mcp.json
  unchanged  .claude/settings.json
  unchanged  .gitignore

Already set up. Nothing was rewritten.
`
	if code, out, _ := initOut(t, root); code != 0 || out != again {
		t.Errorf("second run: code %d\n%s", code, out)
	}
	broken := gitDir(t)
	write(t, broken, ".claude/settings.json", "{\n  \"a\": 1\n  \"b\": 2\n}\n")
	code, out, errOut := initOut(t, broken)
	want := "srwr init: .claude/settings.json is not valid JSON (near line 3)\nNothing was rewritten. Fix the file and run it again.\n"
	if code != 1 || out != "" || errOut != want {
		t.Errorf("broken: %d %q %q", code, out, errOut)
	}
}

func TestTapesInEnglish(t *testing.T) {
	english(t)
	root := t.TempDir()
	if code, out, _ := tapesOut(t, root); code != 0 || out != "No tapes.\n" {
		t.Errorf("no tapes: %d %q", code, out)
	}
	write(t, root, "a.txt", "1\n")
	makeTape(t, root, time.Now(), "a.txt")
	_, out, _ := tapesOut(t, root)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "  Tape ") || !strings.HasSuffix(lines[0], "Size") {
		t.Fatalf("list:\n%s", out)
	}
	if !strings.HasSuffix(lines[1], "  <- current session") || !strings.HasPrefix(lines[3], "1 tape (") {
		t.Errorf("list:\n%s", out)
	}
	if japanese.MatchString(out) {
		t.Errorf("Japanese in the English list:\n%s", out)
	}
	_, out, _ = tapesOut(t, root, "prune", "--keep", "1")
	if out != "No tapes to delete.\n" {
		t.Errorf("prune: %q", out)
	}
	if _, _, errOut := tapesOut(t, root, "prune", "--older-than", "3"); !strings.Contains(errOut, `--older-than "3" must look like 30d or 12h`) {
		t.Errorf("older-than: %q", errOut)
	}
}

// With a different time zone the same tape is listed at a different time: the tape holds UTC, the list is for the person.
func TestTapesListFollowsTheTimeZone(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n")
	makeTape(t, root, time.Date(2026, 10, 3, 8, 12, 10, 0, time.UTC), "a.txt")
	old := time.Local
	t.Cleanup(func() { time.Local = old })
	for _, tc := range []struct {
		zone *time.Location
		lang string
		want string
	}{
		{time.FixedZone("JST", 9*3600), "", "Oct 03 17:12"},
		{time.UTC, "", "Oct 03 08:12"},
		{time.FixedZone("JST", 9*3600), "ja", "10/03 17:12"},
		{time.FixedZone("PDT", -7*3600), "ja", "10/03 01:12"},
	} {
		time.Local = tc.zone
		t.Setenv("SRWR_LANG", tc.lang)
		_, out, _ := tapesOut(t, root)
		if !strings.Contains(out, "  "+tc.want+"  ") {
			t.Errorf("%s %q: want %q in\n%s", tc.zone, tc.lang, tc.want, out)
		}
	}
}

// Tapes named in the local zone (older) and in UTC (new) are put in order by when they started, not by their names.
func TestTapesAreOrderedByStartNotByName(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n")
	// started 06:00 UTC; named 1500 (an old tape, named in JST) and 0900 (new, UTC).
	makeTape(t, root, time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC), "a.txt")
	makeTape(t, root, time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC), "a.txt")
	renameTape(t, root, "20261003-0600", "20261003-1500") // 06:00 UTC, named as if in JST
	_, out, _ := tapesOut(t, root)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[1], "20261003-0900") || !strings.Contains(lines[2], "20261003-1500") {
		t.Errorf("not ordered by the start time:\n%s", out)
	}
}

// renameTape renames the tape whose ID starts with prefix to the ID newPrefix + the rest.
func renameTape(t *testing.T, root, prefix, newPrefix string) {
	t.Helper()
	dir := filepath.Join(root, ".srwr", "tapes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			if err := os.Rename(filepath.Join(dir, e.Name()), filepath.Join(dir, newPrefix+strings.TrimPrefix(e.Name(), prefix))); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no tape %s", prefix)
}

// Every subcommand that takes --root says so in the usage, in both languages (the docs say it; the usage once left two out).
func TestUsageNamesRootForEverySubcommand(t *testing.T) {
	for _, lang := range []string{"", "ja"} {
		t.Setenv("SRWR_LANG", lang)
		_, out, _ := run([]string{"--help"}, "")
		for _, cmd := range []string{"srwr mcp", "srwr hook", "srwr view-server", "srwr view ", "srwr init", "srwr tapes"} {
			found := false
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, cmd) {
					found = true
					if !strings.Contains(line, "--root") {
						t.Errorf("SRWR_LANG=%q: the usage line of %q has no --root:\n%s", lang, cmd, line)
					}
				}
			}
			if !found {
				t.Errorf("SRWR_LANG=%q: no usage line for %q", lang, cmd)
			}
		}
	}
}
