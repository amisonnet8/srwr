package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The extension's README shows pictures by relative paths. vsce turns them into absolute URLs when it packs, and it does not
// know the extension is in a subdirectory of the repository, so the base is given (and the files must be on main when the
// extension is published).
const (
	baseContentURL = "https://github.com/amisonnet8/srwr/blob/main/extension"
	baseImagesURL  = "https://github.com/amisonnet8/srwr/raw/main/extension"
)

// manifest is the part of extension/package.json that is checked.
type manifest struct {
	Name        string   `json:"name"`
	Publisher   string   `json:"publisher"`
	Version     string   `json:"version"`
	Icon        string   `json:"icon"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

func readManifest(root string) (manifest, error) {
	var m manifest
	b, err := os.ReadFile(filepath.Join(root, "extension", "package.json")) //nolint:gosec // the package.json of this repository
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// buildVsix packs the extension (already compiled) into dist/<name>-<version>.vsix and returns the path.
func buildVsix(root, dir string) (string, error) {
	m, err := readManifest(root)
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, fmt.Sprintf("%s-%s.vsix", m.Name, m.Version))
	cmd := exec.Command("npx", "--yes", "@vscode/vsce", "package", "--no-dependencies", //nolint:gosec // fixed arguments
		"--baseContentUrl", baseContentURL, "--baseImagesUrl", baseImagesURL, "-o", out)
	cmd.Dir = filepath.Join(root, "extension")
	if msg, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("vsce package: %w\n%s", err, msg)
	}
	return out, nil
}

var imageRe = regexp.MustCompile(`(?:src="|\]\()([^")\s]+\.(?:png|svg|gif|jpe?g))`)

// readZip returns the files of a zip by name.
func readZip(path string) (map[string][]byte, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a path made by this tool
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = data
	}
	return files, nil
}

// checkVsix checks a .vsix against what the Marketplace and the decisions of the project ask for:
//   - the listing: package.json, the README, the license, the icon (PNG), the compiled code, the translations
//   - nothing that must not be there: the srwr binary (it is installed separately), tests, sources, node_modules, README_ja.md
//   - the README: every picture is an absolute URL under the images base (a relative one would be broken on the Marketplace)
func checkVsix(path string) (manifest, error) {
	var m manifest
	files, err := readZip(path)
	if err != nil {
		return m, err
	}
	var problems []string
	for _, want := range []string{"extension.vsixmanifest", "extension/package.json", "extension/readme.md", "extension/LICENSE.txt",
		"extension/package.nls.json", "extension/package.nls.ja.json", "extension/media/icon.png", "extension/out/src/extension.js"} {
		if _, ok := files[want]; !ok {
			problems = append(problems, "missing: "+want)
		}
	}
	for name, data := range files {
		for _, bad := range []string{"extension/node_modules/", "extension/test/", "extension/out/test/", "extension/src/"} {
			if strings.HasPrefix(name, bad) {
				problems = append(problems, "must not be packed: "+name)
			}
		}
		if strings.EqualFold(filepath.Base(name), "README_ja.md") || strings.HasSuffix(name, ".exe") {
			problems = append(problems, "must not be packed: "+name)
		}
		if looksExecutable(data) {
			problems = append(problems, "an executable is packed (srwr is installed separately): "+name)
		}
	}
	if pkg, ok := files["extension/package.json"]; ok {
		if err := json.Unmarshal(pkg, &m); err != nil {
			problems = append(problems, "package.json: "+err.Error())
		}
	}
	if m.Publisher == "" || m.Name == "" || m.Version == "" {
		problems = append(problems, "package.json needs publisher, name and version")
	}
	if len(m.Keywords) == 0 {
		problems = append(problems, "package.json has no keywords")
	}
	imgs := imageRe.FindAllStringSubmatch(string(files["extension/readme.md"]), -1)
	if len(imgs) == 0 {
		problems = append(problems, "the README shows no picture")
	}
	for _, im := range imgs {
		u := im[1]
		if strings.HasPrefix(u, "https://img.shields.io/") {
			continue // a badge, which the Marketplace trusts
		}
		if !strings.HasPrefix(u, baseImagesURL+"/media/") || !strings.HasSuffix(u, ".png") {
			problems = append(problems, "a README picture is not a PNG under "+baseImagesURL+"/media/: "+u)
			continue
		}
		if _, ok := files["extension/"+strings.TrimPrefix(u, baseImagesURL+"/")]; !ok {
			problems = append(problems, "a README picture is not in the package: "+u)
		}
	}
	if len(problems) > 0 {
		return m, fmt.Errorf("%s:\n  %s", filepath.Base(path), strings.Join(problems, "\n  "))
	}
	return m, nil
}
