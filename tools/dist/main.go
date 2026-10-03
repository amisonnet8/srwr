// Command dist makes what is handed to the users (docs: .claude/rules/distribution.md): the srwr binary for six machines,
// each in an archive, with a list of SHA-256 sums, and the .vsix of the extension. `go run ./tools/dist` (qsoku dist) makes them
// in dist/ and checks what it made. `go run ./tools/dist publish-check [file.vsix]` (qsoku publish-check) checks a .vsix before
// a person uploads it to the Marketplace by hand.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "dist:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	switch {
	case len(args) == 0:
		return makeDist(root, out)
	case args[0] == "publish-check" && len(args) <= 2:
		path := ""
		if len(args) == 2 {
			path = args[1]
		}
		return publishCheck(root, path, out)
	}
	return errors.New("usage: dist [publish-check [file.vsix]]")
}

// describe is the version the archives are named for: the tag when HEAD is on one (v0.1.0), else what `git describe` says,
// else "dev" (not a git checkout).
func describe(root string) string {
	cmd := exec.Command("git", "describe", "--tags", "--always")
	cmd.Dir = root
	b, err := cmd.Output()
	if err != nil {
		return "dev"
	}
	return cleanVersion(string(b))
}

// makeDist builds, packs and checks everything into root/dist.
func makeDist(root string, out io.Writer) error {
	dir := filepath.Join(root, "dist")
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	version := describe(root)
	license, err := os.ReadFile(filepath.Join(root, "LICENSE")) //nolint:gosec // the license of this repository
	if err != nil {
		return err
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md")) //nolint:gosec // the README of this repository
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "srwr-dist-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()

	var names []string
	for _, t := range targets {
		bin := filepath.Join(work, t.goos+"_"+t.goarch, t.exe())
		if err := build(root, t, bin); err != nil {
			return err
		}
		data, err := os.ReadFile(bin) //nolint:gosec // just built
		if err != nil {
			return err
		}
		archive := t.archiveName(version)
		files := []file{{t.exe(), data, 0o755}, {"LICENSE", license, 0o644}, {"README.md", readme, 0o644}}
		if err := writeArchive(filepath.Join(dir, archive), files); err != nil {
			return err
		}
		if err := checkArchive(filepath.Join(dir, archive), t); err != nil {
			return err
		}
		names = append(names, archive)
		_, _ = fmt.Fprintf(out, "built  %-14s %s (%d KB)\n", t, archive, len(data)/1024)
	}
	vsix, err := buildVsix(root, dir)
	if err != nil {
		return err
	}
	m, err := checkVsix(vsix)
	if err != nil {
		return err
	}
	names = append(names, filepath.Base(vsix))
	_, _ = fmt.Fprintf(out, "packed %-14s %s (version %s)\n", "extension", filepath.Base(vsix), m.Version)
	if err := writeChecksums(dir, names); err != nil {
		return err
	}
	if err := verifyChecksums(dir); err != nil {
		return err
	}
	if err := smokeTest(dir, version, out); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "done   %d archives, the .vsix and checksums.txt in %s\n", len(targets), dir)
	return nil
}

// build makes the binary of t: no cgo, the paths of the machine left out, symbols stripped. The version of the binary is
// the one Go records from the checkout (a tag gives v0.1.0).
func build(root string, t target, out string) error {
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", out, "./cmd/srwr") //nolint:gosec // fixed arguments
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.goos, "GOARCH="+t.goarch)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build for %s: %w\n%s", t, err, msg)
	}
	return nil
}

// smokeTest runs the binary of this machine out of its archive: it says its version and its usage.
func smokeTest(dir, version string, out io.Writer) error {
	here := target{runtime.GOOS, runtime.GOARCH}
	found := false
	for _, t := range targets {
		found = found || t == here
	}
	if !found {
		_, _ = fmt.Fprintf(out, "skip   running the binary: %s is not one of the targets\n", here)
		return nil
	}
	files, err := readArchive(filepath.Join(dir, here.archiveName(version)))
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "srwr-run-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	exe := filepath.Join(tmp, here.exe())
	if err := os.WriteFile(exe, files[here.exe()], 0o755); err != nil { //nolint:gosec // it is an executable
		return err
	}
	for _, args := range [][]string{{"--version"}, {"--help"}} {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(exe, args...) //nolint:gosec // the binary this tool just built
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("srwr %s: %w\n%s", args[0], err, stderr.String())
		}
		if args[0] == "--version" {
			if !strings.HasPrefix(stdout.String(), "srwr ") {
				return fmt.Errorf("srwr --version printed %q", stdout.String())
			}
			_, _ = fmt.Fprintf(out, "ran    %s: %s", here, stdout.String())
		}
	}
	return nil
}

// publishCheck checks the .vsix that a person is about to upload at the Marketplace by hand, and says how.
func publishCheck(root, path string, out io.Writer) error {
	if path == "" {
		matches, err := filepath.Glob(filepath.Join(root, "dist", "*.vsix"))
		if err != nil || len(matches) == 0 {
			return errors.New("no .vsix in dist/; run qsoku dist first")
		}
		sort.Strings(matches)
		path = matches[len(matches)-1]
	}
	m, err := checkVsix(path)
	if err != nil {
		return err
	}
	src, err := readManifest(root)
	if err != nil {
		return err
	}
	if src.Version != m.Version {
		return fmt.Errorf("the .vsix is version %s but extension/package.json says %s: run qsoku dist again", m.Version, src.Version)
	}
	info, err := os.Stat(path) //nolint:gosec // the .vsix a person named, or one found in dist/
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "OK  %s  (%s.%s %s, %d KB)\n\n", path, m.Publisher, m.Name, m.Version, info.Size()/1024)
	_, _ = fmt.Fprintf(out, `Upload it by hand (dev/publish.md):
  1. Push main first: the README pictures are read from main (%s/media/readme/).
  2. Open https://marketplace.visualstudio.com/manage/publishers/%s
  3. New extension -> Visual Studio Code (or, for an update, the ... menu of %s -> Update), and choose the file above.
  4. Wait for the verification, then look at the page: the icon, the pictures, the badges.
  5. The version of an update must be higher than the published one (package.json "version").
`, baseImagesURL, m.Publisher, m.Name)
	return nil
}
