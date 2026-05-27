#!/bin/bash
# protect-data.sh: Block modifications to /data directory
# Hook: PreToolUse on Edit tool with if: "Edit(data/*)"

# Read hook input from stdin (JSON)
read -r hook_input

# Extract file path from the hook input
file_path=$(echo "$hook_input" | jq -r '.tool_input.file_path // empty')

# Normalize backslashes to forward slashes (Windows path compatibility)
file_path="${file_path//\\//}"

# Block if path starts with data/ (forward or backslash already normalized)
if [[ "$file_path" == data/* ]]; then
  exit 2
fi

exit 0
