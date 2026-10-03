package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeArchive(t *testing.T, dir string) string {
	t.Helper()
	var buf bytes.Buffer
	name := "srwr"
	path := filepath.Join(dir, "srwr_v0.1.0_"+runtime.GOOS+"_"+runtime.GOARCH+".tar.gz")
	if runtime.GOOS == "windows" {
		name = "srwr.exe"
		path = filepath.Join(dir, "srwr_v0.1.0_windows_"+runtime.GOARCH+".zip")
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte("EXE"))
		w, _ = zw.Create("LICENSE")
		_, _ = w.Write([]byte("MIT"))
		_ = zw.Close()
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for n, c := range map[string]string{name: "EXE", "LICENSE": "MIT"} {
			_ = tw.WriteHeader(&tar.Header{Name: n, Mode: 0o755, Size: int64(len(c)), Typeflag: tar.TypeReg})
			_, _ = tw.Write([]byte(c))
		}
		_ = tw.Close()
		_ = gz.Close()
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDistArchiveAndExtractExe(t *testing.T) {
	dist := t.TempDir()
	if _, err := distArchive(dist); err == nil {
		t.Error("no archive in dist/ should be an error that says to run qsoku dist")
	}
	archive := fakeArchive(t, dist)
	got, err := distArchive(dist)
	if err != nil || got != archive {
		t.Fatalf("distArchive = %q, %v", got, err)
	}
	exe, err := extractExe(archive, filepath.Join(t.TempDir(), "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "EXE" { //nolint:gosec // the file just extracted
		t.Errorf("the extracted file holds %q", b)
	}
	if info, _ := os.Stat(exe); runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Error("the extracted file is not executable")
	}
	// An archive without the executable is an error.
	empty := filepath.Join(dist, "empty.tar.gz")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_ = tar.NewWriter(gz).Close()
	_ = gz.Close()
	_ = os.WriteFile(empty, buf.Bytes(), 0o600)
	if _, err := extractExe(empty, t.TempDir()); err == nil {
		t.Error("an archive without srwr was accepted")
	}
}

func TestRenderDistTryPage(t *testing.T) {
	ok := renderDistTryPage([]string{"○ a"}, "/d/x.vsix", "/d/srwr", true)
	for _, want := range []string{"機械で確かめられることは全部", "/d/x.vsix", "/d/srwr", "Forward", "]]", "dist/ の srwr に埋め込まれたスクリプト"} {
		if !strings.Contains(ok, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	ng := renderDistTryPage([]string{"× b"}, "/d/x.vsix", "/d/srwr", false)
	if !strings.Contains(ng, "× があります") || !strings.Contains(ng, `class="ng"`) {
		t.Error("a failed check must show on the page")
	}
}
