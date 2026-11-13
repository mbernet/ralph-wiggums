#!/usr/bin/env bash
#
# Tests for how an agent run is classified from its result event.
#
#   ./tests/classify.test.sh
#
# ralph.sh only runs its loop when executed, so sourcing it here just loads the
# functions.

set -uo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export RALPH_DIR="$TMP/ralph"
mkdir -p "$RALPH_DIR"

# shellcheck source=/dev/null
source "$TEST_DIR/../ralph.sh"
ui_init

PASS=0 FAIL=0

check() { # check <label> <expected> <actual>
  if [[ "$2" == "$3" ]]; then
    PASS=$((PASS + 1))
    printf '  ok   %s\n' "$1"
  else
    FAIL=$((FAIL + 1))
    printf '  FAIL %s\n       expected: %s\n       actual:   %s\n' "$1" "$2" "$3"
  fi
}

# A result event as the CLI emits it, trimmed to the fields ralph reads.
result() { # result <subtype> <is_error> <text>
  jq -nc --arg s "$1" --argjson e "$2" --arg t "$3" \
    '{type: "result", subtype: $s, is_error: $e, result: $t, num_turns: 3, total_cost_usd: 0.1}'
}

classify() { # classify <stream line> [stderr text] -> prints ITER_CLASS
  printf '%s\n' "$1" >"$TMP/stream.jsonl"
  printf '%s' "${2:-}" >"$TMP/stderr.log"
  result_parse "$TMP/stream.jsonl"
  classify_result "$TMP/stderr.log"
  printf '%s' "$ITER_CLASS"
}

printf '\nclassify_result\n'

# A finished run whose summary talks about the app it built is not a failure:
# a registration task naturally mentions /login, a test count can contain 429.
check "successful run mentioning /login is ok" ok \
  "$(classify "$(result success false 'Registration redirects to /login after sign-up.')")"
check "successful run mentioning 429 is ok" ok \
  "$(classify "$(result success false 'Suite green: 429 tests passed.')")"
check "successful run mentioning a rate limit is ok" ok \
  "$(classify "$(result success false 'Added a rate limit to the contact form.')")"

# Real failures come back flagged as errors, often with subtype success.
check "error asking to log in is auth" auth \
  "$(classify "$(result success true 'Invalid API key · Please run /login')")"
check "error about the usage limit is ratelimit" ratelimit \
  "$(classify "$(result success true 'Claude AI usage limit reached')")"
check "auth failure reported only on stderr is auth" auth \
  "$(classify "" 'Error: not logged in')"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
((FAIL == 0))
