#!/usr/bin/env bash
# Measure the MCP client's tool-call idle cap with the real client (FR83).
#
# Runs two headless Claude sessions against tools/idlecap-server.py, in
# parallel, each parking one tool call past the suspected cap: one silent, one
# ticking progress notifications. The logs date what the client did.
#
#   tools/idlecap-probe.sh [SECONDS]     # default 2100 (35 min), cap suspected 1800
#
# Reading the result: in each log, `<- {"method":"notifications/cancelled"` is
# the client giving up, and its +offset IS the cap. A `returning after Ns` with
# no cancellation before it means the call survived that long.
set -u
SECONDS_PARK=${1:-2100}
HERE=$(cd "$(dirname "$0")" && pwd)
OUT=${AGENTBOX_IDLECAP_OUT:-/tmp/idlecap}
mkdir -p "$OUT"

run() { # name, tool, args-json
  local name=$1 tool=$2 args=$3
  cat >"$OUT/$name.mcp.json" <<JSON
{"mcpServers":{"idlecap":{"type":"stdio","command":"python3","args":["$HERE/idlecap-server.py"],
  "env":{"AGENTBOX_IDLECAP_LOG":"$OUT/$name.log"}}}}
JSON
  # --strict-mcp-config keeps the real agentbox child out of the run, so the
  # measurement cannot be confused by another server's traffic, and these
  # throwaway sessions never land on the human's Agents board.
  claude -p "Call the mcp__idlecap__$tool tool with $args. It is expected to take a long time; wait for it. Use no other tool. Then reply with the tool's exact result or exact error text and nothing else." \
    --model claude-haiku-4-5-20251001 \
    --mcp-config "$OUT/$name.mcp.json" --strict-mcp-config \
    --allowedTools "mcp__idlecap__$tool" --max-turns 4 \
    >"$OUT/$name.claude.txt" 2>"$OUT/$name.claude.err"
  echo "$name finished with exit $?" >>"$OUT/$name.log"
}

: >"$OUT/quiet.log"; : >"$OUT/ticking.log"
run quiet park_quiet "seconds=$SECONDS_PARK" &
run ticking park_progress "seconds=$SECONDS_PARK and every=120" &
wait
echo "--- logs in $OUT"
grep -hE "cancelled|returning after|progressToken|finished with exit" "$OUT"/quiet.log "$OUT"/ticking.log
