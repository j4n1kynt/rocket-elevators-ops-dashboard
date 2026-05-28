#!/bin/bash
# gofmt.sh: Auto-format Go files after Edit.
# Hook: PostToolUse on Edit tool.

read -r hook_input

if command -v jq &>/dev/null; then
  file_path=$(echo "$hook_input" | jq -r '.tool_input.file_path // empty')
else
  file_path=$(echo "$hook_input" | sed 's/.*"file_path":"\([^"]*\)".*/\1/')
fi

# Normalize backslashes to forward slashes (Windows path compatibility)
file_path="${file_path//\\//}"

# Only format Go source files
[[ "$file_path" == *.go ]] || exit 0

gofmt -w "$file_path"
