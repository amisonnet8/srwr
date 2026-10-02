#!/usr/bin/env bash
# Build after a Go source, module file or extension source is edited, and report
# failures to Claude.
set -euo pipefail

file=$(jq -r '.tool_input.file_path // empty')
case "$file" in
  *.go | */go.mod | */go.sum) target=build ;;
  */extension/src/*.ts | */extension/test/*.ts | */extension/package.json | */extension/tsconfig.json) target=ext-build ;;
  *) exit 0 ;;
esac

cd "$CLAUDE_PROJECT_DIR"
if ! output=$(qsoku "$target" 2>&1); then
  echo "$output" >&2
  exit 2
fi
