#!/usr/bin/env bash
#
# Tests for the event stream ralph.sh writes for its renderers.
#
#   ./tests/events.test.sh
#
# Every run of the loop leaves one JSON event per line in its log directory
# (docs/events.md). These tests run the real loop against the fake API, as
# api.test.sh does, and read that file back. Copy is not under test: only the
# types, the order and the fields a renderer depends on.

# shellcheck disable=SC2034  # the globals below are read by the sourced functions
# shellcheck disable=SC2329  # _http is replaced here and called by the sourced functions
# shellcheck disable=SC2016  # single-quoted $vars are jq program variables

set -uo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export RALPH_DIR="$TMP/ralph"
export CLAUDE_CONFIG_DIR="$TMP/cfg"
mkdir -p "$RALPH_DIR" "$CLAUDE_CONFIG_DIR"

export RALPH_STASH=0
export RALPH_API_RETRIES=0
export RALPH_MIN_DELAY_S=0
export RALPH_USAGE_PROBE=0
export RALPH_API_BASE="http://127.0.0.1:1"
export RALPH_API_URL="http://ralph.test/api/ralph"
export RALPH_API_TOKEN="test-token"
export RALPH_PROJECT="myapp"
export RALPH_WORKER="vps-1"
export RALPH_UI="plain"

# shellcheck source=/dev/null
source "$TEST_DIR/../ralph.sh"
set +e
ui_init

PASS=0 FAIL=0
PIPELINE='["implement","review_functional","review_code","merge"]'

check() { # check <label> <expected> <actual>
  if [[ "$2" == "$3" ]]; then
    PASS=$((PASS + 1))
    printf '  ok   %s\n' "$1"
  else
    FAIL=$((FAIL + 1))
    printf '  FAIL %s\n       expected: %s\n       actual:   %s\n' "$1" "$2" "$3"
  fi
}

_http() { _http_fake "$@"; }

PROMPTS_DIR="$TMP/prompts"
mkdir -p "$PROMPTS_DIR"
for s in _common implement review_functional review_code merge doctor; do
  printf 'Prompt for %s.\n' "$s" >"$PROMPTS_DIR/$s.md"
done

# A stand-in agent that answers by the position of the stage. AGENT_MODE makes
# it misbehave in one way.
cat >"$TMP/agent.sh" <<'EOF'
#!/usr/bin/env bash
prompt="$(cat)"
work="$(printf '%s\n' "$prompt" | sed -n '/^<work>$/,/^<\/work>$/p' | sed '1d;$d')"
stage="$(jq -r '.stage' <<<"$work")"
first="$(jq -r '.project.pipeline[0]' <<<"$work")"
last="$(jq -r '.project.pipeline[-1]' <<<"$work")"
result="$(printf '%s\n' "$prompt" | sed -n 's/^    \(.*\.result.*\.json\)$/\1/p' | head -1)"
printf 'working on %s\n' "$stage"
case "${AGENT_MODE:-}" in
  reject-once)
    if [[ "$stage" == "review_code" && ! -e "$AGENT_LOG.rejected" ]]; then
      : >"$AGENT_LOG.rejected"
      jq -n '{outcome: "reject", body: "Missing a test."}' >"$result"
      exit 0
    fi
    ;;
  stop-after-first)
    [[ "$stage" == "$first" ]] && : >"$AGENT_STOP_FILE"
    ;;
esac
case "$stage" in
  doctor) jq -n '{outcome: "pass", body: "Nothing wrong."}' ;;
  "$first") jq -n '{outcome: "pass", body: "Implemented.", assumptions: ["Assumed X."],
                    pr: "https://github.com/acme/myapp/pull/1"}' ;;
  "$last") jq -n '{outcome: "pass", body: "Merged.", summary: "What to know."}' ;;
  *) jq -n '{outcome: "pass", body: "Looks good."}' ;;
esac >"$result"
EOF
chmod +x "$TMP/agent.sh"
export RALPH_AGENT_CMD="$TMP/agent.sh"

cat >"$TMP/one.json" <<EOF
{
  "project": "myapp",
  "description": "One task",
  "pipeline": $PIPELINE,
  "tasks": {
    "US-001": {"title": "Add priority field", "description": "d", "acceptanceCriteria": ["Typecheck passes"], "priority": 1, "model": "low", "dependsOn": []}
  }
}
EOF

# loop <name> <agent mode> <ralph args...> -> runs the whole worker; sets LOOP_RC
# and EVENTS, the event file it wrote.
LOOP_RC=0 EVENTS=""
loop() {
  local name="$1" mode="$2"
  shift 2
  FAKE_STATE="$TMP/$name.json"
  LOG_DIR="$TMP/$name/logs"
  SCENARIO="${LOOP_SCENARIO:-mixed}" fake_init "${LOOP_PRD:-$TMP/one.json}"
  export AGENT_LOG="$TMP/$name.agent.log"
  export AGENT_MODE="$mode"
  export AGENT_STOP_FILE="$LOG_DIR/stop"
  LOOP_RC=0
  (
    set -e
    main "$@"
  ) >"$TMP/$name.out" 2>&1 || LOOP_RC=$?
  EVENTS="$LOG_DIR/events.ndjson"
  return 0
}
# ev_q <jq filter over the array of events>
ev_q() { jq -sr "$1" "$EVENTS" 2>/dev/null; }

printf '\nthe life of a worker\n'

loop happy "" 10
check "the run leaves an event file" "yes" "$([[ -s "$EVENTS" ]] && echo yes || echo no)"
check "every line is a versioned event with a time and a type" "true" \
  "$(ev_q 'all(.v == 1 and (.ts | type) == "number" and (.type | type) == "string")')"
check "it opens with the session and closes with its end" "session.start session.end" \
  "$(ev_q '[first.type, (map(select(.type != "session.exit")) | last.type)] | join(" ")')"
check "the session says what it works on" "myapp vps-1 4" \
  "$(ev_q 'first | "\(.project) \(.worker) \(.pipeline | length)"')"
check "each stage starts and ends" \
  "implement review_functional review_code merge|implement review_functional review_code merge" \
  "$(ev_q '[map(select(.type == "stage.start") | .stage), map(select(.type == "stage.end") | .stage)]
           | map(join(" ")) | join("|")')"
check "a stage start names the task, its place and its claim" "US-001 1/4 true" \
  "$(ev_q 'map(select(.type == "stage.start")) | first
           | "\(.task.id) \(.pos)/\(.count) \(.claimExpires > 0)"')"
check "a stage end carries the outcome and where the task went" "pass review_functional" \
  "$(ev_q 'map(select(.type == "stage.end")) | first | "\(.outcome) \(.next)"')"
check "the first stage end carries the assumptions and the PR" "1 https://github.com/acme/myapp/pull/1" \
  "$(ev_q 'map(select(.type == "stage.end")) | first | "\(.assumptions | length) \(.pr)"')"
check "the session end says how it ended" "complete 4" \
  "$(ev_q 'map(select(.type == "session.end")) | first | "\(.state) \(.ran)"')"
check "the exit code is the last event" "session.exit 0" \
  "$(ev_q 'last | "\(.type) \(.code)"')"

check "a plain-text agent's output arrives line by line" "4" \
  "$(ev_q 'map(select(.type == "agent.line" and (.text | startswith("working on")))) | length')"

printf '\nthe agent, from its stream\n'

# A dry run plays a recorded claude stream through the real formatter.
loop stream "" --dry-run --fast --scenario complete
check "the session of the agent and its model" "agent.init claude-opus-5" \
  "$(ev_q 'map(select(.type == "agent.init")) | first | "\(.type) \(.model)"')"
check "what it says" "3" "$(ev_q 'map(select(.type == "agent.text")) | length')"
check "each tool it calls, with what it is called on" "Read src/components/TaskEditModal.tsx" \
  "$(ev_q 'map(select(.type == "agent.tool")) | first | "\(.name) \(.arg)"')"
check "each tool result, and the error of a failed one" "6 1 true" \
  "$(ev_q 'map(select(.type == "agent.result")) | [length, (map(select(.ok | not)) | length),
           (map(select(.ok | not)) | first | .error | test("FAIL"))] | map(tostring) | join(" ")')"
check "the agent events sit between the start and the end of the stage" "true" \
  "$(ev_q '(map(.type) | index("stage.start")) as $s | (map(.type) | index("stage.end")) as $e
           | [to_entries[] | select(.value.type | startswith("agent.")) | .key]
           | all(. > $s and . < $e)')"
check "a quiet run still writes them" "yes" \
  "$(loop quiet "" --dry-run --fast --scenario complete --quiet
     ev_q 'if any(.type == "agent.tool") then "yes" else "no" end')"

printf '\nthe project, as the server sees it\n'

loop board "" 10
check "the board is sent at the start, after every registered run and when no work is left" "6" \
  "$(ev_q 'map(select(.type == "board")) | length')"
check "the first board has the task in the first stage, unclaimed" "US-001 implement false" \
  "$(ev_q 'map(select(.type == "board")) | first | .tasks[0] | "\(.id) \(.stage) \(.claim != null)"')"
check "a task on the board carries what a card shows" \
  "id,title,priority,stage,runs,sick,needsHuman,claim,pr,rejects,assumptions,doneAt,why,blockedBy" \
  "$(ev_q 'map(select(.type == "board")) | first | .tasks[0] | keys_unsorted | join(",")')"
check "the last board has it done, with its PR and its assumptions" "done https://github.com/acme/myapp/pull/1 1" \
  "$(ev_q 'map(select(.type == "board")) | last | .tasks[0] | "\(.stage) \(.pr) \(.assumptions)"')"
check "the counts follow the project" "1/1" \
  "$(ev_q 'map(select(.type == "project")) | last | .counts | "\(.done)/\(.total)"')"
check "the window is reported, even when nothing can read it" "none" \
  "$(ev_q 'map(select(.type == "window")) | first | .src')"

loop rejected reject-once 10
check "a reject carries its feedback" "reject Missing a test." \
  "$(ev_q 'map(select(.type == "stage.end" and .outcome == "reject")) | first | "\(.outcome) \(.body)"')"
check "and the board counts it" "1" \
  "$(ev_q 'map(select(.type == "board")) | last | .tasks[0].rejects')"

RALPH_IDLE_S=0 LOOP_SCENARIO=nowork loop idle "" 10
check "nothing eligible is an event, with the counts that explain it" "false 1" \
  "$(ev_q 'map(select(.type == "work.none")) | first | "\(.finished) \(.counts.total)"')"

printf '\nstopping by request\n'

loop stopped stop-after-first 10
check "the session tells a renderer where to ask for a stop" "true" \
  "$(ev_q 'first | .control.stop | endswith("/stop")')"
check "a stop request lets the stage in progress finish, and no other starts" "implement" \
  "$(ev_q 'map(select(.type == "stage.end") | .stage) | join(" ")')"
check "the stage is registered on the server" "review_functional" "$(jq -r '.tasks[0].state.stage' "$FAKE_STATE")"
check "the session ends as stopped" "stopped" "$(ev_q 'map(select(.type == "session.end")) | first | .state')"
check "which is not a success" "1" "$LOOP_RC"

printf '\nwaits and notices\n'

EVENTS_FILE="$TMP/unit.ndjson"
: >"$EVENTS_FILE"
EVENTS="$EVENTS_FILE"
countdown 1 "idle" >/dev/null 2>&1
check "a wait says what for, how long and until when" "idle 1 true" \
  "$(ev_q 'map(select(.type == "wait.start")) | first | "\(.kind) \(.seconds) \(.until > .ts - 1)"')"
check "and when it is over" "wait.end idle" \
  "$(ev_q 'map(select(.type == "wait.end")) | first | "\(.type) \(.kind)"')"
: >"$EVENTS_FILE"
{
  note "a note"
  ok "a success"
  warn "a warning"
  err "an error"
} >/dev/null 2>&1
check "what the loop tells the user is a notice, with its level" "info ok warn error" \
  "$(ev_q 'map(select(.type == "notice") | .level) | join(" ")')"
: >"$EVENTS_FILE"
PACE_ACTION="throttle" PACE_SLEEP_S=0 PACE_WHY="ahead of even pace" pace_enforce >/dev/null 2>&1
check "a pacing decision is an event" "throttle ahead of even pace" \
  "$(ev_q 'map(select(.type == "pace")) | first | "\(.action) \(.why)"')"

: >"$EVENTS_FILE"
SKIP_FILE="$TMP/skip"
: >"$SKIP_FILE"
started="$(date +%s)"
countdown 60 "throttle" >/dev/null 2>&1
check "a skip request ends a wait at once" "1" "$(($(date +%s) - started < 5))"
check "and is consumed" "no" "$([[ -e "$SKIP_FILE" ]] && echo yes || echo no)"
check "the wait says it was cut short" "true" \
  "$(ev_q 'map(select(.type == "wait.end")) | first | .skipped')"
SKIP_FILE=""
EVENTS_FILE=""

printf '\nchoosing the renderer\n'

check "plain is plain, whatever is installed" "plain" "$(ui_choose plain /bin/ui 1)"
check "auto takes ralph-ui on a terminal" "rich" "$(ui_choose auto /bin/ui 1)"
check "auto keeps to plain output without a terminal" "plain" "$(ui_choose auto /bin/ui 0)"
check "auto keeps to plain output without ralph-ui" "plain" "$(ui_choose auto "" 1)"
check "rich cannot do without a terminal" "plain" "$(ui_choose rich /bin/ui 0)"

# A stand-in for ralph-ui that says how it was called and what it was given.
cat >"$TMP/ui.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$UI_LOG"
while (($#)); do
  [[ "$1" == "--events" ]] && cp "$2" "$UI_LOG.events"
  shift
done
EOF
chmod +x "$TMP/ui.sh"
export UI_LOG="$TMP/ui.log"

board() { # board <name> -> runs ./ralph.sh --board against the fake API; sets LOOP_RC
  local name="$1"
  FAKE_STATE="$TMP/$name.json"
  LOG_DIR="$TMP/$name/logs"
  SCENARIO=mixed fake_init "$TMP/one.json"
  LOOP_RC=0
  (
    set -e
    main --board
  ) >"$TMP/$name.out" 2>&1 || LOOP_RC=$?
}

RALPH_UI_BIN="$TMP/ui.sh" board withui
check "the board is handed to ralph-ui" "0 board" "$LOOP_RC $(cut -d' ' -f1 "$UI_LOG")"
check "as a stream that holds the project and its tasks" "session.start board US-001" \
  "$(jq -sr '[(map(.type) | if index("session.start") then "session.start" else "-" end),
              (map(.type) | if index("board") then "board" else "-" end),
              (map(select(.type == "board")) | first | .tasks[0].id)] | join(" ")' "$UI_LOG.events")"
check "without claiming anything" "0" "$(jq '.seq' "$FAKE_STATE")"

RALPH_UI_BIN="$TMP/none" board noui
check "without ralph-ui the board is printed as text" "0 1" \
  "$LOOP_RC $(grep -c 'US-001' "$TMP/noui.out")"

printf '\n%d passed, %d failed\n\n' "$PASS" "$FAIL"
((FAIL == 0))
