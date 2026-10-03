package main

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tiny builds a program that does nothing for t, much faster than srwr itself, to test the check of the file format.
func tiny(t *testing.T, tg target) []byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tiny\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, tg.exe())
	cmd := exec.Command("go", "build", "-o", out, ".") //nolint:gosec // fixed arguments
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+tg.goos, "GOARCH="+tg.goarch)
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", tg, err, msg)
	}
	b, err := os.ReadFile(out) //nolint:gosec // a file just built
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCheckBinaryKnowsEveryOSAndCPU(t *testing.T) {
	if testing.Short() {
		t.Skip("builds six programs")
	}
	built := map[target][]byte{}
	for _, tg := range targets {
		built[tg] = tiny(t, tg)
	}
	for _, tg := range targets {
		if err := checkBinary(built[tg], tg); err != nil {
			t.Errorf("%s: %v", tg, err)
		}
		// Every other target's binary is refused: the wrong OS and the wrong CPU.
		for _, other := range targets {
			if other != tg && checkBinary(built[other], tg) == nil {
				t.Errorf("a %s binary passed as %s", other, tg)
			}
		}
	}
	if err := checkBinary([]byte("#!/bin/sh\n"), target{"linux", "amd64"}); err == nil {
		t.Error("a script passed as an executable")
	}
}

func TestArchiveRoundTripAndCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	dir := t.TempDir()
	for _, tg := range []target{{"linux", "arm64"}, {"windows", "amd64"}} {
		path := filepath.Join(dir, tg.archiveName("v1.2.3"))
		files := []file{{tg.exe(), tiny(t, tg), 0o755}, {"LICENSE", []byte("MIT"), 0o644}, {"README.md", []byte("# srwr"), 0o644}}
		if err := writeArchive(path, files); err != nil {
			t.Fatal(err)
		}
		if err := checkArchive(path, tg); err != nil {
			t.Errorf("%s: %v", tg, err)
		}
		// The same files make the same archive.
		again := filepath.Join(dir, "again-"+tg.archiveName("v1.2.3"))
		if err := writeArchive(again, files); err != nil {
			t.Fatal(err)
		}
		a, _ := os.ReadFile(path)  //nolint:gosec // see above
		b, _ := os.ReadFile(again) //nolint:gosec // see above
		if !bytes.Equal(a, b) {
			t.Errorf("%s: two archives of the same files differ", tg)
		}
		// Without the license it is refused; for the wrong CPU it is refused.
		bad := filepath.Join(dir, "bad-"+tg.archiveName("v1.2.3"))
		if err := writeArchive(bad, files[:1]); err != nil {
			t.Fatal(err)
		}
		if err := checkArchive(bad, tg); err == nil {
			t.Errorf("%s: an archive without LICENSE passed", tg)
		}
		if err := checkArchive(path, target{tg.goos, map[string]string{"arm64": "amd64", "amd64": "arm64"}[tg.goarch]}); err == nil {
			t.Errorf("%s: passed for the other CPU", tg)
		}
	}
}

func TestArchiveNames(t *testing.T) {
	if got := (target{"linux", "amd64"}).archiveName("v0.1.0"); got != "srwr_v0.1.0_linux_amd64.tar.gz" {
		t.Errorf("name = %s", got)
	}
	if got := (target{"windows", "arm64"}).archiveName("dev"); got != "srwr_dev_windows_arm64.zip" {
		t.Errorf("name = %s", got)
	}
	if (target{"windows", "amd64"}).exe() != "srwr.exe" || (target{"darwin", "arm64"}).exe() != "srwr" {
		t.Error("exe names")
	}
	for in, want := range map[string]string{"": "dev", "v0.1.0\n": "v0.1.0", "v0.1.0-3-gabc1234": "v0.1.0-3-gabc1234", "a/b c": "a-b-c"} {
		if got := cleanVersion(in); got != want {
			t.Errorf("cleanVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChecksums(t *testing.T) {
	dir := t.TempDir()
	for n, c := range map[string]string{"a.tar.gz": "one", "b.zip": "two"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeChecksums(dir, []string{"b.zip", "a.tar.gz"}); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksums(dir); err != nil {
		t.Fatal(err)
	}
	// A changed file, and a file left out of the list, are found.
	if err := os.WriteFile(filepath.Join(dir, "a.tar.gz"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksums(dir); err == nil || !strings.Contains(err.Error(), "a.tar.gz") {
		t.Errorf("a changed file: %v", err)
	}
	if err := writeChecksums(dir, []string{"a.tar.gz", "b.zip"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.zip"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksums(dir); err == nil || !strings.Contains(err.Error(), "c.zip") {
		t.Errorf("a file left out: %v", err)
	}
}

// vsixWith writes a zip with the given files and returns its path.
func vsixWith(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(c))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "x.vsix")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func goodVsix() map[string]string {
	return map[string]string{
		"extension.vsixmanifest":            "<x/>",
		"extension/package.json":            `{"name":"srwr-view","publisher":"amisonnet8","version":"0.1.0","keywords":["ai"]}`,
		"extension/readme.md":               "![a](" + baseImagesURL + "/media/readme/replay.png) ![b](https://img.shields.io/x.svg)",
		"extension/LICENSE.txt":             "MIT",
		"extension/package.nls.json":        "{}",
		"extension/package.nls.ja.json":     "{}",
		"extension/media/icon.png":          "png",
		"extension/media/readme/replay.png": "png",
		"extension/out/src/extension.js":    "js",
	}
}

func TestCheckVsix(t *testing.T) {
	m, err := checkVsix(vsixWith(t, goodVsix()))
	if err != nil || m.Version != "0.1.0" || m.Publisher != "amisonnet8" {
		t.Fatalf("a good package: %+v, %v", m, err)
	}
	cases := []struct {
		name   string
		change func(map[string]string)
		want   string
	}{
		{"an ELF binary inside", func(f map[string]string) { f["extension/bin/srwr"] = "\x7fELF....." }, "an executable is packed"},
		{"a Windows binary inside", func(f map[string]string) { f["extension/bin/x"] = "MZ......" }, "an executable is packed"},
		{"README_ja.md inside", func(f map[string]string) { f["extension/README_ja.md"] = "x" }, "must not be packed"},
		{"tests inside", func(f map[string]string) { f["extension/out/test/a.js"] = "x" }, "must not be packed"},
		{"no icon", func(f map[string]string) { delete(f, "extension/media/icon.png") }, "missing: extension/media/icon.png"},
		{"a relative picture", func(f map[string]string) { f["extension/readme.md"] = "![a](media/readme/replay.png)" }, "not a PNG under"},
		{"an SVG picture", func(f map[string]string) { f["extension/readme.md"] = "![a](" + baseImagesURL + "/media/readme/x.svg)" }, "not a PNG under"},
		{"a picture that is not packed", func(f map[string]string) {
			f["extension/readme.md"] = "![a](" + baseImagesURL + "/media/readme/none.png)"
		}, "not in the package"},
		{"no keywords", func(f map[string]string) { f["extension/package.json"] = `{"name":"a","publisher":"b","version":"1"}` }, "no keywords"},
	}
	for _, c := range cases {
		f := goodVsix()
		c.change(f)
		if _, err := checkVsix(vsixWith(t, f)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v, want it to contain %q", c.name, err, c.want)
		}
	}
}

func TestLooksExecutable(t *testing.T) {
	for _, s := range []string{"\x7fELF\x02", "MZ\x90\x00", "\xcf\xfa\xed\xfe", "\xca\xfe\xba\xbe"} {
		if !looksExecutable([]byte(s)) {
			t.Errorf("%q should look like an executable", s)
		}
	}
	for _, s := range []string{"", "ab", "{\"a\":1}", "// MZ is in the text", "#!/bin/sh"} {
		if looksExecutable([]byte(s)) {
			t.Errorf("%q should not look like an executable", s)
		}
	}
}
