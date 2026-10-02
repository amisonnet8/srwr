#!/usr/bin/env bash
set -euo pipefail

# wget, gnupg,
# lsb-release: Adding the Trivy apt repository below.
# gcc:         qsoku race (CGO_ENABLED=1 go test -race) needs a C compiler.
#              The container itself runs with CGO_ENABLED=0 (devcontainer.json).
# jq:          Used by .claude/hooks/build.sh and for inspecting --json output.
# ShellCheck:  Static analysis of tracked *.sh and *.bash files (qsoku shellcheck).
#              Comment lines must not start with the lowercase directive word,
#              or ShellCheck parses them as directives (SC1072/SC1073).
# vim:         The full Vim (not vim-tiny) for the Vim client (vim/, srwr view) and
#              qsoku vim-test. Needs 9.0.0784 or later; bookworm's Vim is 9.0.1378.
# Node.js:     For the VSCode extension (extension/). The "node" feature in
#              devcontainer.json, not installed here.
# gh:          GitHub CLI (issues, pull requests, Actions runs) is the "github-cli"
#              feature in devcontainer.json, not installed here. The feature is left
#              unpinned (no "version" option): each rebuild installs the latest release.
sudo apt-get update
sudo apt-get install -y wget gnupg lsb-release gcc jq shellcheck vim

# The Bash sandbox (.claude/settings.json "sandbox") needs bubblewrap (bwrap)
# and socat on Linux; without them it silently stays off even with
# "enabled": true. Install only what is missing, since some base images
# already have them. Project-independent (.claude/rules/testing.md).
missing_sandbox_deps=()
command -v bwrap >/dev/null 2>&1 || missing_sandbox_deps+=(bubblewrap)
command -v socat >/dev/null 2>&1 || missing_sandbox_deps+=(socat)
if [ "${#missing_sandbox_deps[@]}" -gt 0 ]; then
  sudo apt-get update
  sudo apt-get install -y "${missing_sandbox_deps[@]}"
fi

# Trivy: known vulnerabilities (CVE) and license compatibility of dependencies
# (qsoku trivy, .claude/rules/testing.md). Installed from the official apt repository.
wget -qO - https://aquasecurity.github.io/trivy-repo/deb/public.key | gpg --dearmor | sudo tee /usr/share/keyrings/trivy.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb $(lsb_release -sc) main" | sudo tee /etc/apt/sources.list.d/trivy.list >/dev/null
sudo apt-get update
sudo apt-get install -y trivy

# The Bash sandbox (.claude/settings.json) only honours an allowWrite path that
# already exists, and ~/.cache itself is read-only there. Create every
# filesystem.allowWrite path up front, or the first qsoku lint / qsoku trivy in
# a fresh container fails with "read-only file system" (.claude/rules/testing.md).
# The list is read from settings.json, so this block needs no change when
# allowWrite does, and it is project-independent. "~/x" and "$HOME/x" are created
# under the home directory; other absolute paths (e.g. /go) are created if they do
# not exist yet, and a failure only warns (no sudo is used). Relative paths, "."
# and $TMPDIR, and the special paths /dev, /proc and /sys are left alone.
sandbox_settings="$(dirname "$0")/../.claude/settings.json"
tilde='~'
if [ -f "$sandbox_settings" ]; then
  while IFS= read -r allow_path; do
    case "$allow_path" in
      "$tilde/"*) allow_path="$HOME/${allow_path#"$tilde/"}" ;;
      /dev/* | /proc/* | /sys/*) continue ;;
      /*) ;;
      *) continue ;;
    esac
    mkdir -p "$allow_path" || echo "warning: cannot create allowWrite path $allow_path" >&2
  done < <(jq -r '.sandbox.filesystem.allowWrite[]?' "$sandbox_settings")
fi

# golangci-lint: lint (qsoku check, .golangci.yaml). The official install script
# puts the binary into GOPATH/bin. The version is pinned so that lint results
# do not change when the container is rebuilt; .golangci.yaml was verified with it.
wget -qO - https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.13.2

go install golang.org/x/tools/gopls@latest
go install golang.org/x/tools/cmd/goimports@latest

# qsoku: build/check/test entry points (qsokufile).
go install github.com/amisonnet8/qsoku/cmd/qsoku@latest

# mtqg: memo/todo/qa/bug/glossary/rule journal (.mtqg/, CLAUDE.md, .mcp.json).
go install github.com/amisonnet8/mtqg/cmd/mtqg@latest

# Wire up qsoku's bash integration (working-directory carry-back and
# completion). Idempotent: skipped if already present, so re-running
# postCreate.sh does not duplicate the line. The single quotes are intentional
# -- the line is meant to land in ~/.bashrc unexpanded.
# shellcheck disable=SC2016
grep -qF 'qsoku .shell bash' ~/.bashrc 2>/dev/null || echo 'eval "$(qsoku .shell bash)"' >>~/.bashrc

# mtqg's own bash completion, for interactive use in this container.
mkdir -p ~/.local/share/bash-completion/completions
mtqg completion bash >~/.local/share/bash-completion/completions/mtqg

# Install the extension's dev dependencies (TypeScript, @types) once
# extension/package.json exists (qsokufile ext-deps).
if [ -f extension/package.json ]; then
  qsoku ext-deps
fi
