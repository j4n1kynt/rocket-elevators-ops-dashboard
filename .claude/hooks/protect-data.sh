#!/bin/bash
# protect-data.sh: Block modifications to /data directory
# Hook: PreToolUse on Edit tool with if: "Edit(data/*)"

read -r hook_input

# Check jq availability; fall back to sed-based extraction
if command -v jq &>/dev/null; then
  file_path=$(echo "$hook_input" | jq -r '.tool_input.file_path // empty')
else
  file_path=$(echo "$hook_input" | sed 's/.*"file_path":"\([^"]*\)".*/\1/')
fi

# Normalize backslashes to forward slashes (Windows path compatibility)
file_path="${file_path//\\//}"

if [[ "$file_path" == data/* ]]; then
  exit 2
fi

exit 0
