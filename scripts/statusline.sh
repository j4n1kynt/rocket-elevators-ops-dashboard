#!/usr/bin/env bash
# statusline.sh — Claude Code status bar
# Reads JSON from stdin (provided by Claude Code) and prints a one-line status.
#
# Fields displayed:
#   model        – display name of the active model
#   ctx%         – percentage of the context window currently used
#   tokens       – total input tokens in context / context window size
#   in / out     – raw input and output tokens from the last API call
#   cache-r      – cache_read_input_tokens: tokens served from the prompt cache
#   cache-w      – cache_creation_input_tokens: tokens written into the prompt cache
#   cost         – estimated session cost in USD (from .cost.total_cost_usd when available)

input=$(cat)

jq -r '
  (.model.display_name // .model.id // "unknown")              as $model      |
  (.context_window.current_usage.input_tokens          // 0)   as $in_tok     |
  (.context_window.current_usage.output_tokens         // 0)   as $out_tok    |
  (.context_window.current_usage.cache_read_input_tokens  // 0) as $cache_r   |
  (.context_window.current_usage.cache_creation_input_tokens // 0) as $cache_w |
  (.context_window.total_input_tokens                  // 0)   as $total_in   |
  (.context_window.context_window_size                 // 200000) as $ctx_size |
  (.context_window.used_percentage                     // null) as $used_pct   |
  (.cost.total_cost_usd                                // null) as $cost_raw   |

  ($ctx_size / 1000 | floor | tostring + "K")                  as $ctx_max    |
  (if $used_pct == null then "?%" else ($used_pct | floor | tostring + "%") end) as $pct_str |

  # Format cost: show "$X.XXXX" when present, otherwise omit the field
  (if $cost_raw != null
   then " | cost: $" + ($cost_raw * 10000 | round / 10000 | tostring)
   else ""
   end)                                                         as $cost_str   |

  "\($model) | ctx: \($pct_str) (\($total_in)/\($ctx_max)) | in: \($in_tok) out: \($out_tok) | cache-r: \($cache_r) cache-w: \($cache_w)\($cost_str)"
' <<< "$input"
