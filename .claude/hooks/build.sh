#!/usr/bin/env bash
# Build after a Go source, module file or extension source is changed, and
# report failures to Claude. srwr runs in the lenient mode here (Edit and Write
# are allowed), so this hook runs on Edit / Write (input "file_path") and on
# srwr's edit / new (input "file", or "edits[].file" for several changes at once).
set -euo pipefail

go=false
ext=false
while IFS= read -r file; do
  case "$file" in
    *.go | *go.mod | *go.sum) go=true ;;
    *extension/src/*.ts | *extension/test/*.ts | *extension/package.json | *extension/tsconfig.json) ext=true ;;
  esac
done < <(jq -r '.tool_input | [.file, .file_path, (.edits[]?.file)] | .[] | strings')

cd "$CLAUDE_PROJECT_DIR"
for target in build ext-build; do
  if { [ "$target" = build ] && [ "$go" = true ]; } || { [ "$target" = ext-build ] && [ "$ext" = true ]; }; then
    if ! output=$(qsoku "$target" 2>&1); then
      echo "$output" >&2
      exit 2
    fi
  fi
done
