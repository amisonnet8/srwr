// Command dist makes and checks the .vsix of the extension (docs: .claude/rules/distribution.md). The archives of the srwr
// binary and checksums.txt are made by goreleaser (.goreleaser.yaml); its before hook runs `go run ./tools/dist vsix`
// (qsoku vsix), which packs the .vsix into bin/vsix/ and checks it, and goreleaser attaches it to the Release.
// `go run ./tools/dist publish-check [file.vsix]` (qsoku publish-check) checks a .vsix before a person uploads it to the
// Marketplace by hand.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// vsixDir is where qsoku vsix puts the .vsix, relative to the root: goreleaser's release.extra_files and
// checksum.extra_files take it from there.
const vsixDir = "bin/vsix"

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
	case len(args) == 1 && args[0] == "vsix":
		return makeVsix(root, out)
	case len(args) >= 1 && args[0] == "publish-check" && len(args) <= 2:
		path := ""
		if len(args) == 2 {
			path = args[1]
		}
		return publishCheck(root, path, out)
	}
	return errors.New("usage: dist vsix | dist publish-check [file.vsix]")
}

// makeVsix packs the extension (already compiled) into root/bin/vsix/, alone, and checks it.
func makeVsix(root string, out io.Writer) error {
	dir := filepath.Join(root, filepath.FromSlash(vsixDir))
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	vsix, err := buildVsix(root, dir)
	if err != nil {
		return err
	}
	m, err := checkVsix(vsix)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "packed %s (version %s)\n", filepath.Join(vsixDir, filepath.Base(vsix)), m.Version)
	return nil
}

// publishCheck checks the .vsix that a person is about to upload at the Marketplace by hand, and says how.
func publishCheck(root, path string, out io.Writer) error {
	if path == "" {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(vsixDir), "*.vsix"))
		if err != nil || len(matches) == 0 {
			return errors.New("no .vsix in " + vsixDir + "/; run qsoku vsix first")
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
		return fmt.Errorf("the .vsix is version %s but extension/package.json says %s: run qsoku vsix again", m.Version, src.Version)
	}
	// Everything the READMEs reach over the network is fetched now, before anything is uploaded.
	files, err := readZip(path)
	if err != nil {
		return err
	}
	if err := readmeMatchesRepo(root, string(files["extension/readme.md"])); err != nil {
		return err
	}
	urls := readmeURLs(root, string(files["extension/readme.md"]))
	problems := checkRemoteImages(fetchClient, urls)
	page, err := writePreviewPage(root, urls, problems)
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return fmt.Errorf("DO NOT UPLOAD. %d picture(s) of the READMEs do not work:\n  %s\n(look at %s)", len(problems), strings.Join(problems, "\n  "), page)
	}
	info, err := os.Stat(path) //nolint:gosec // the .vsix a person named, or one found in bin/vsix/
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "OK  %s  (%s.%s %s, %d KB)\n", path, m.Publisher, m.Name, m.Version, info.Size()/1024)
	_, _ = fmt.Fprintf(out, "OK  %d pictures of the READMEs were fetched and all work. Look at them too: %s\n\n", len(urls), page)
	_, _ = fmt.Fprintf(out, `Upload it by hand (dev/publish.md):
  1. Push main first: the README pictures are read from main (%s/media/readme/).
  2. Open https://marketplace.visualstudio.com/manage/publishers/%s
  3. New extension -> Visual Studio Code (or, for an update, the ... menu of %s -> Update), and choose the file above.
  4. Wait for the verification, then look at the page: the icon, the pictures, the badges.
  5. The version of an update must be higher than the published one (package.json "version").
`, baseImagesURL, m.Publisher, m.Name)
	return nil
}
