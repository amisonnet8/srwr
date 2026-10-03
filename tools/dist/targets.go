package main

import (
	"fmt"
	"strings"
)

// target is one machine a binary of srwr is made for.
type target struct{ goos, goarch string }

// targets are the six builds that GitHub Releases carries (.claude/rules/distribution.md).
var targets = []target{
	{"linux", "amd64"}, {"linux", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"windows", "amd64"}, {"windows", "arm64"},
}

// exe is the name of the executable inside the archive.
func (t target) exe() string {
	if t.goos == "windows" {
		return "srwr.exe"
	}
	return "srwr"
}

// archiveName is srwr_<version>_<os>_<arch>.tar.gz (.zip for Windows).
func (t target) archiveName(version string) string {
	ext := ".tar.gz"
	if t.goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("srwr_%s_%s_%s%s", version, t.goos, t.goarch, ext)
}

func (t target) String() string { return t.goos + "/" + t.goarch }

// cleanVersion turns the output of `git describe` into something fit for a file name. An empty string is "dev".
func cleanVersion(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "dev"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '-', r == '_', r == '+':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
