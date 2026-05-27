#!/bin/bash
# protect-data.sh: Block modifications to /data directory
# Hook: PreToolUse on Edit tool with if: "Edit(data/*)"

# Read hook input from stdin (JSON)
read -r hook_input

# Extract file path from the hook input
file_path=$(echo "$hook_input" | jq -r '.tool_input.file_path // empty')

# Check if the file is in the /data directory
if [[ "$file_path" == data/* ]]; then
  # Block the operation
  exit 2
fi

# Allow the operation
exit 0
