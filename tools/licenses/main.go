// Command licenses gathers the license texts of the code built into a command
// of this module (the modules it links, and Go's standard library) into one
// directory, for the release archives (.goreleaser.yaml,
// .claude/rules/distribution.md).
//
//	go run ./tools/licenses <dir> <main package>
//
// <main package> is the command that is released, such as ./cmd/yourproject.
// <dir> is made again from scratch: <dir>/<module path>/<license file> for
// each module, and <dir>/go/LICENSE for the standard library. A module with
// no license file is an error, so that no release ships code without its
// license.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// releasedOS are the OSes the command is released for (.goreleaser.yaml). Some
// modules are linked on one OS only, so each is asked.
var releasedOS = []string{"linux", "darwin", "windows"}

// licensePrefixes are the starts of the names of license files, compared
// without case (LICENSE, LICENSE.md, License.txt, COPYING, NOTICE and so on).
var licensePrefixes = []string{"LICENSE", "LICENCE", "COPYING", "NOTICE"} //nolint:misspell // LICENCE is the British spelling some modules use for the file

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/licenses <dir> <main package>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "licenses:", err)
		os.Exit(1)
	}
}

func run(dir, mainPkg string) error {
	mods, err := linkedModules(mainPkg)
	if err != nil {
		return err
	}
	goroot, err := goCmd("", "env", "GOROOT")
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil { //nolint:gosec // dir is the output directory the release build names (bin/licenses), made again on every run
		return err
	}
	if err := copyFile(filepath.Join(strings.TrimSpace(goroot), "LICENSE"), filepath.Join(dir, "go", "LICENSE")); err != nil {
		return fmt.Errorf("the license of Go: %w", err)
	}
	paths := make([]string, 0, len(mods))
	for p := range mods {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		files, err := licenseFiles(mods[p])
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("%s: no license file in %s", p, mods[p])
		}
		for _, f := range files {
			if err := copyFile(filepath.Join(mods[p], f), filepath.Join(dir, filepath.FromSlash(p), f)); err != nil {
				return err
			}
		}
	}
	return nil
}

// linkedModules returns the modules (other than this module itself) whose
// packages are built into mainPkg on any released OS, with the directory of
// each.
func linkedModules(mainPkg string) (map[string]string, error) {
	mods := map[string]string{}
	for _, goos := range releasedOS {
		out, err := goCmd(goos, "list", "-deps", "-f",
			"{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Dir}}{{end}}{{end}}", mainPkg)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(out, "\n") {
			path, dir, ok := strings.Cut(strings.TrimSpace(line), "\t")
			if !ok {
				continue // a package of the standard library
			}
			if dir == "" {
				return nil, fmt.Errorf("%s: the module is not downloaded (run go mod download)", path)
			}
			mods[path] = dir
		}
	}
	return mods, nil
}

// licenseFiles returns the names of the license files directly in dir.
func licenseFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.Type().IsRegular() && isLicense(e.Name()) {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

func isLicense(name string) bool {
	upper := strings.ToUpper(name)
	return slices.ContainsFunc(licensePrefixes, func(p string) bool { return strings.HasPrefix(upper, p) })
}

// goCmd runs the go command, for goos if it is not "", and returns what it
// printed.
func goCmd(goos string, args ...string) (string, error) {
	cmd := exec.Command("go", args...) //nolint:gosec // only the go command; the arguments are written in this file, and the main package comes from .goreleaser.yaml
	cmd.Env = os.Environ()
	if goos != "" {
		cmd.Env = append(cmd.Env, "GOOS="+goos)
	}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, ee.Stderr)
		}
		return "", fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) //nolint:gosec // src is a license file in GOROOT or in the module cache, found by go list
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // license texts are public, and go into public archives
		return err
	}
	return os.WriteFile(dst, data, 0o644) //nolint:gosec // license texts are public, and go into public archives
}
