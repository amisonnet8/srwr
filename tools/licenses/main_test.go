package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestIsLicense(t *testing.T) {
	for name, want := range map[string]bool{
		"LICENSE":      true,
		"LICENSE.md":   true,
		"License.txt":  true,
		"LICENCE":      true, //nolint:misspell // the British spelling of the file name
		"COPYING":      true,
		"NOTICE":       true,
		"license-mit":  true,
		"README.md":    false,
		"go.mod":       false,
		"MY_LICENSE":   false,
		"licenses.txt": true,
	} {
		if got := isLicense(name); got != want {
			t.Errorf("isLicense(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestLicenseFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"LICENSE", "NOTICE.txt", "main.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A directory named like a license file is not one.
	if err := os.Mkdir(filepath.Join(dir, "LICENSES"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := licenseFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"LICENSE", "NOTICE.txt"}; !slices.Equal(got, want) {
		t.Errorf("licenseFiles: %q, want %q", got, want)
	}
}

// TestRun gathers the licenses of srwr itself. When a dependency is added or
// dropped, this list changes: check the license of the new module, then
// update the list.
func TestRun(t *testing.T) {
	t.Chdir(filepath.Join("..", "..")) // the module root, where ./cmd/srwr is
	dir := t.TempDir()
	if err := run(dir, "./cmd/srwr"); err != nil {
		t.Fatal(err)
	}
	var got []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			got = append(got, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"go/LICENSE",
	}
	if !slices.Equal(got, want) {
		t.Errorf("gathered:\n%q\nwant:\n%q", got, want)
	}
}
