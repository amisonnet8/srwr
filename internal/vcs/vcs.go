// Package vcs says what state the git work tree of a workspace is in, for the header of a tape (docs/reference/tape.md).
// It only reads, and it never gets in the way: whatever goes wrong, the answer is "not under git" (null).
package vcs

import (
	"context"
	"encoding/json"
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
