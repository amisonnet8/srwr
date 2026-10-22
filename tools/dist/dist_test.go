package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
		"extension/readme.md":               "![a](" + baseImagesURL + "/media/readme/replay.png) ![b](https://img.shields.io/x.svg) ![c](https://flat.badgen.net/vs-marketplace/v/a.b)",
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

func TestRemoteImageURLs(t *testing.T) {
	text := "![a](https://x.test/a.png) <img src=\"https://img.shields.io/badge/a-b-green\"> [link](https://x.test/page) <img src=\"https://flat.badgen.net/vs-marketplace/v/a.b\"> ![a](https://x.test/a.png) ![rel](media/x.png)"
	got := remoteImageURLs(text)
	want := []string{"https://x.test/a.png", "https://img.shields.io/badge/a-b-green", "https://flat.badgen.net/vs-marketplace/v/a.b"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("urls = %v, want %v", got, want)
	}
}

func TestCheckRemoteImage(t *testing.T) {
	svg := func(text string) string {
		return `<svg xmlns="http://www.w3.org/2000/svg"><rect width="9" height="9"/><text>` + text + `</text></svg>`
	}
	mux := http.NewServeMux()
	serve := func(path, ct string, code int, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", ct)
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		})
	}
	serve("/good.svg", "image/svg+xml", 200, svg("VS Marketplace: v0.1.1"))
	serve("/retired.svg", "image/svg+xml", 200, svg("visual-studio-marketplace: retired badge"))
	serve("/error500.svg", "image/svg+xml", 200, svg("rating: 500"))
	serve("/unavailable.svg", "image/svg+xml", 200, svg("VS Marketplace: unavailable"))
	serve("/missing.png", "text/plain", 404, "not found")
	serve("/page.png", "text/html", 200, "<html></html>")
	serve("/good.png", "image/png", 200, "\x89PNG....")
	serve("/empty.png", "image/png", 200, "")
	serve("/gone.png", "image/png", 404, "\x89PNG....")
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for path, wantBad := range map[string]bool{"/good.svg": false, "/good.png": false, "/retired.svg": true, "/error500.svg": true,
		"/unavailable.svg": true, "/missing.png": true, "/gone.png": true, "/page.png": true, "/empty.png": true} {
		got := checkRemoteImage(srv.Client(), srv.URL+path)
		if (got != "") != wantBad {
			t.Errorf("%s: problem = %q, want a problem: %v", path, got, wantBad)
		}
	}
	if got := checkRemoteImages(srv.Client(), []string{srv.URL + "/good.svg", srv.URL + "/retired.svg", srv.URL + "/missing.png"}); len(got) != 2 {
		t.Errorf("problems = %v, want 2", got)
	}
	if p := checkRemoteImage(&http.Client{Timeout: 200 * time.Millisecond}, "http://127.0.0.1:1/x.png"); p == "" {
		t.Error("an unreachable host should be a problem")
	}
}

func TestReadmeMatchesRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "extension"), 0o750); err != nil {
		t.Fatal(err)
	}
	repo := "# a\n![p](media/readme/x.png)\n[MIT](LICENSE)\n"
	if err := os.WriteFile(filepath.Join(root, "extension", "README.md"), []byte(repo), 0o600); err != nil {
		t.Fatal(err)
	}
	packed := "# a\n![p](" + baseImagesURL + "/media/readme/x.png)\n[MIT](" + baseContentURL + "/LICENSE)\n"
	if err := readmeMatchesRepo(root, packed); err != nil {
		t.Errorf("the same README was refused: %v", err)
	}
	if err := readmeMatchesRepo(root, strings.Replace(packed, "# a", "# b", 1)); err == nil {
		t.Error("a README that differs from the repository was accepted")
	}
}
