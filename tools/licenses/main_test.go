package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

// TestRunDependencies gathers the licenses of a command that links modules of
// its own making, put next to it with replace (no network): one linked on
// every OS, and one linked on Windows only. Unlike TestRun, this test stays as
// it is in every project.
func TestRunDependencies(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"app/go.mod": "module example.com/app\n\nrequire (\n\texample.com/dep v0.0.0\n\texample.com/windep v0.0.0\n)\n\n" +
			"replace example.com/dep => ../dep\n\nreplace example.com/windep => ../windep\n",
		"app/cmd/hello/main.go":         "package main\n\nimport \"example.com/dep\"\n\nfunc main() { dep.Hello() }\n",
		"app/cmd/hello/main_windows.go": "package main\n\nimport _ \"example.com/windep\"\n",
		"dep/go.mod":                    "module example.com/dep\n",
		"dep/dep.go":                    "package dep\n\nfunc Hello() {}\n",
		"dep/LICENSE":                   "the license of dep\n",
		"dep/README.md":                 "not a license\n",
		"windep/go.mod":                 "module example.com/windep\n",
		"windep/windep.go":              "package windep\n",
		"windep/COPYING":                "the license of windep\n",
	})
	t.Chdir(filepath.Join(root, "app"))
	dir := t.TempDir()
	if err := run(dir, "./cmd/hello"); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		got[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"example.com/dep/LICENSE":    "the license of dep\n",
		"example.com/windep/COPYING": "the license of windep\n",
	}
	for name, body := range want {
		if got[name] != body {
			t.Errorf("%s: %q, want %q", name, got[name], body)
		}
	}
	var names []string
	for name := range got {
		names = append(names, name)
	}
	slices.Sort(names)
	if wantNames := []string{"example.com/dep/LICENSE", "example.com/windep/COPYING", "go/LICENSE"}; !slices.Equal(names, wantNames) {
		t.Errorf("gathered:\n%q\nwant:\n%q", names, wantNames)
	}
}

// TestRunNoLicense fails on a linked module that has no license file, so that
// no release ships code without its license.
func TestRunNoLicense(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"app/go.mod":            "module example.com/app\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ../dep\n",
		"app/cmd/hello/main.go": "package main\n\nimport \"example.com/dep\"\n\nfunc main() { dep.Hello() }\n",
		"dep/go.mod":            "module example.com/dep\n",
		"dep/dep.go":            "package dep\n\nfunc Hello() {}\n",
	})
	t.Chdir(filepath.Join(root, "app"))
	err := run(t.TempDir(), "./cmd/hello")
	if err == nil || !strings.Contains(err.Error(), "example.com/dep: no license file") {
		t.Errorf("run: %v, want the error that example.com/dep has no license file", err)
	}
}

// TestRunRemovesEarlierOutput makes the directory again from scratch, so that
// the license of a dependency that was dropped does not stay in the release.
func TestRunRemovesEarlierOutput(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"app/go.mod":                         "module example.com/app\n",
		"app/cmd/hello/main.go":              "package main\n\nfunc main() {}\n",
		"out/example.com/dropped/LICENSE":    "the license of a dropped dependency\n",
		"out/example.com/dropped/README.txt": "left by hand\n",
	})
	t.Chdir(filepath.Join(root, "app"))
	dir := filepath.Join(root, "out")
	if err := run(dir, "./cmd/hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "example.com")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the earlier output is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go", "LICENSE")); err != nil {
		t.Errorf("the license of Go is not there: %v", err)
	}
}

// TestCommand runs the built command: the usage and 2 without its two
// arguments, the error and 1 when run fails, and the licenses in the given
// directory when it works.
func TestCommand(t *testing.T) {
	name := "licenses"
	if runtime.GOOS == "windows" {
		name += ".exe" // without it, Windows does not find the file as a command
	}
	bin := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"app/go.mod":            "module example.com/app\n",
		"app/cmd/hello/main.go": "package main\n\nfunc main() {}\n",
		"bad/go.mod":            "module example.com/bad\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ../dep\n",
		"bad/cmd/hello/main.go": "package main\n\nimport \"example.com/dep\"\n\nfunc main() { dep.Hello() }\n",
		"dep/go.mod":            "module example.com/dep\n",
		"dep/dep.go":            "package dep\n\nfunc Hello() {}\n",
	})
	out := filepath.Join(t.TempDir(), "out")
	for _, c := range []struct {
		name, dir string
		args      []string
		code      int
		stderr    string
	}{
		{"no arguments", "app", nil, 2, "usage: go run ./tools/licenses <dir> <main package>\n"},
		{"one argument", "app", []string{out}, 2, "usage: go run ./tools/licenses <dir> <main package>\n"},
		{"three arguments", "app", []string{out, "./cmd/hello", "x"}, 2, "usage: go run ./tools/licenses <dir> <main package>\n"},
		{"no license", "bad", []string{out, "./cmd/hello"}, 1, "licenses: example.com/dep: no license file in "},
		{"works", "app", []string{out, "./cmd/hello"}, 0, ""},
	} {
		cmd := exec.Command(bin, c.args...)
		cmd.Dir = filepath.Join(root, c.dir)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if code != c.code || !strings.HasPrefix(stderr.String(), c.stderr) || (c.stderr == "") != (stderr.Len() == 0) {
			t.Errorf("%s: exit %d, stderr %q; want exit %d, stderr starting %q", c.name, code, stderr.String(), c.code, c.stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "go", "LICENSE")); err != nil {
		t.Errorf("the license of Go is not in the given directory: %v", err)
	}
}

// writeFiles writes files (slash-separated paths under root, and their
// contents), making the directories they are in.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
