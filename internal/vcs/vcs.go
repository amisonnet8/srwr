// Package vcs says what state the git work tree of a workspace is in, for the header of a tape (docs/reference/tape.md).
// It only reads, and it never gets in the way: whatever goes wrong, the answer is "not under git" (null).
package vcs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Timeout is how long Detect waits for git in all.
const Timeout = 5 * time.Second

// Detect returns the vcs value of a tape header for the work tree at root: {"type":"git","head":…,"dirty":…}, or null when root
// is not under git or git cannot be asked. head is null in a repository that has no commit yet. dirty is true when there is a
// change that is not committed (tracked files changed, staged, deleted, or new files that git does not ignore); what is inside
// .srwr/ does not count, since the tape itself is there.
func Detect(root string) json.RawMessage {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	// Outside a work tree both commands fail. A repository with no commit fails only the first.
	var head *string
	if out, err := git(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		h := strings.TrimSpace(out)
		head = &h
	}
	status, err := git(ctx, root, "status", "--porcelain=v1", "--", ".", ":(exclude).srwr")
	if err != nil {
		return json.RawMessage("null")
	}
	b, err := json.Marshal(struct {
		Type  string  `json:"type"`
		Head  *string `json:"head"`
		Dirty bool    `json:"dirty"`
	}{"git", head, strings.TrimSpace(status) != ""})
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// git runs one read-only git command in dir. Nothing it reads can make it ask a question, take a lock, or start a helper.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false", "--no-optional-locks", "-C", dir}, args...)...) //nolint:gosec // git, with fixed arguments, in the workspace
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	out, err := cmd.Output()
	return string(out), err
}

// Change is one file that git says differs from HEAD or is new. Status is git's two-letter code without the blank
// ("M", "A", "D", "R", "??").
type Change struct {
	Status string
	Path   string // slash-separated, relative to the workspace
}

// Changes lists the files below root that changed in the git work tree: modified, staged, deleted, renamed (by the new
// name) and new files that git does not ignore. What is inside .srwr/ is left out. Unlike Detect it says why it failed, since
// a person asked for it: git is not installed, or root is not in a git work tree.
func Changes(root string) ([]Change, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	prefix, err := git(ctx, root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, errNotGit
	}
	out, err := git(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".", ":(exclude).srwr")
	if err != nil {
		return nil, errNotGit
	}
	return parseStatus(out, strings.TrimSpace(prefix)), nil
}

// errNotGit is what Changes says when git cannot tell: not installed, not a work tree, or too slow.
var errNotGit = errors.New("git cannot tell the state of this directory (is git installed, and is it in a git work tree?)")

// parseStatus reads the output of git status --porcelain=v1 -z: entries "XY path" separated by NUL. A rename or copy is
// followed by one more entry, the old name, which is skipped. prefix is where the workspace is inside the repository
// ("" or "sub/"); paths are made relative to it, and one that is outside is dropped.
func parseStatus(out, prefix string) []Change {
	var changes []Change
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		xy, path := e[:2], e[3:]
		if strings.ContainsAny(xy, "RC") {
			i++ // the old name
		}
		rel, ok := strings.CutPrefix(path, prefix)
		if !ok {
			continue
		}
		changes = append(changes, Change{Status: strings.ReplaceAll(xy, " ", ""), Path: rel})
	}
	return changes
}

// NewFiles lists the files below root that are new to git and that git does not ignore, as slash-separated paths relative to
// root: untracked ones, and ones added to the index since HEAD ("git add", "git add -N", "git mv"), which a command that
// writes a file and adds it in one go leaves behind. What is inside .srwr/ is left out. Like Changes it says why it failed;
// the hook, which must never stop the agent, ignores the error.
func NewFiles(root string) ([]string, error) {
	changes, err := Changes(root)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, c := range changes {
		if strings.ContainsAny(c.Status, "?ARC") {
			files = append(files, c.Path)
		}
	}
	return files, nil
}
