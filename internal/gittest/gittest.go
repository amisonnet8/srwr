// Package gittest keeps the git that tests run (directly, or through srwr) away from the
// developer's settings: a global ignore file, a template or a default branch on the developer's
// machine must not change what a test sees (.claude/rules/testing.md). It is used only by tests.
package gittest

import (
	"os"
	"path/filepath"
)

// Isolate points HOME, XDG_CONFIG_HOME and GIT_CONFIG_GLOBAL into a new temporary directory and
// turns the system config off, for the whole test binary. Call it first in TestMain, and call the
// returned function when the tests end. The Go build cache is pinned where it was before HOME moved,
// so tests that run go build do not start from an empty cache. A test that needs a user name or an
// e-mail for a commit still sets them itself.
func Isolate() (func(), error) {
	if os.Getenv("GOCACHE") == "" {
		if cache, err := os.UserCacheDir(); err == nil {
			if err := os.Setenv("GOCACHE", filepath.Join(cache, "go-build")); err != nil {
				return nil, err
			}
		}
	}
	dir, err := os.MkdirTemp("", "srwr-test-home-")
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	config := filepath.Join(dir, ".gitconfig")
	if err := os.WriteFile(config, nil, 0o600); err != nil {
		cleanup()
		return nil, err
	}
	for k, v := range map[string]string{
		"HOME":                dir,
		"XDG_CONFIG_HOME":     filepath.Join(dir, ".config"),
		"GIT_CONFIG_GLOBAL":   config,
		"GIT_CONFIG_NOSYSTEM": "1",
	} {
		if err := os.Setenv(k, v); err != nil {
			cleanup()
			return nil, err
		}
	}
	return cleanup, nil
}
