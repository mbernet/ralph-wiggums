#!/usr/bin/env bash
#
# Tests for the optional workspace settings (RALPH_WORKTREE, RALPH_BASE_BRANCH)
# and for recovering a result the agent wrote under the wrong directory.
#
#   ./tests/workspace.test.sh
#
# No network: origin is a bare repository in a temporary directory. ralph.sh
# only runs its loop when executed, so sourcing it here just loads the functions.

# shellcheck disable=SC2034  # the globals below are read by the sourced functions
# shellcheck disable=SC2016  # the backticks in the patterns are literal

set -uo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export RALPH_DIR="$TMP/ralph"
mkdir -p "$RALPH_DIR"

# shellcheck source=/dev/null
source "$TEST_DIR/../ralph.sh"
ui_init
set +e # ralph.sh turns errexit on; some checks below expect a failure

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

git_quiet() { git -c init.defaultBranch=main -c user.name=t -c user.email=t@t "$@" >/dev/null 2>&1; }

# A project with an origin that has `main`, and a worktree of it.
git_quiet init --bare "$TMP/origin.git"
git_quiet init "$TMP/project"
git_quiet -C "$TMP/project" commit --allow-empty -m init
git_quiet -C "$TMP/project" remote add origin "$TMP/origin.git"
git_quiet -C "$TMP/project" push origin main
git_quiet -C "$TMP/project" worktree add --detach "$TMP/project-wt" main

DRY_RUN=0

printf '\npreflight_workspace\n'

(
  cd "$TMP/project" || exit 1
  RALPH_WORKTREE="" RALPH_BASE_BRANCH=""
  preflight_workspace
  printf '%s|%s' "$(pwd)" "$RALPH_BASE_BRANCH"
) >"$TMP/out" 2>/dev/null
check "neither set: stays where it started, no base branch" "$TMP/project|" "$(cat "$TMP/out")"

(
  cd "$TMP/project" || exit 1
  RALPH_WORKTREE="$TMP/project-wt" RALPH_BASE_BRANCH=""
  preflight_workspace
  pwd
) >"$TMP/out" 2>/dev/null
check "RALPH_WORKTREE: moves into it" "$(cd "$TMP/project-wt" && pwd)" "$(cat "$TMP/out")"

(
  RALPH_WORKTREE="$TMP/missing" RALPH_BASE_BRANCH=""
  preflight_workspace
) >/dev/null 2>&1
check "RALPH_WORKTREE that does not exist: stops" "1" "$(($? != 0))"

mkdir -p "$TMP/plain"
(
  RALPH_WORKTREE="$TMP/plain" RALPH_BASE_BRANCH=""
  preflight_workspace
) >/dev/null 2>&1
check "RALPH_WORKTREE that is not a git work tree: stops" "1" "$(($? != 0))"

(
  cd "$TMP/project" || exit 1
  RALPH_WORKTREE="" RALPH_BASE_BRANCH="main"
  preflight_workspace
) >/dev/null 2>&1
check "RALPH_BASE_BRANCH on origin: goes on" "0" "$?"

(
  cd "$TMP/project" || exit 1
  RALPH_WORKTREE="" RALPH_BASE_BRANCH="f9"
  preflight_workspace
) >/dev/null 2>&1
check "RALPH_BASE_BRANCH missing on origin: stops" "1" "$(($? != 0))"

(
  cd "$TMP/project" || exit 1
  DRY_RUN=1 RALPH_WORKTREE="" RALPH_BASE_BRANCH="f9"
  preflight_workspace
) >/dev/null 2>&1
check "dry run: the base branch is not looked up" "0" "$?"

printf '\nresult_recover\n'

# ralph lives in the project's scripts/ralph; the agent works in the worktree and
# writes the result to the path relative to the project.
SCRIPT_DIR="$TMP/project/scripts/ralph"
mkdir -p "$SCRIPT_DIR"
rel="scripts/ralph/.ralph/logs/run/iter-01.result.json"
want="$TMP/project/$rel"

(
  cd "$TMP/project-wt" || exit 1
  mkdir -p "$(dirname "$rel")"
  printf '{"outcome":"pass"}' >"$rel"
  result_recover "$want"
  [[ -s "$want" && ! -e "$rel" ]] && echo moved
) >"$TMP/out" 2>/dev/null
check "a result under the worktree is moved to where ralph reads it" "moved" "$(cat "$TMP/out")"
check "the moved result keeps its content" '{"outcome":"pass"}' "$(cat "$want")"

(
  cd "$TMP/project-wt" || exit 1
  printf '{"outcome":"stray"}' >"$rel"
  result_recover "$want"
  cat "$want"
) >"$TMP/out" 2>/dev/null
check "a result already in place is left alone" '{"outcome":"pass"}' "$(cat "$TMP/out")"
rm -f "$TMP/project-wt/$rel"

rm -f "$want"
(
  cd "$TMP/project" || exit 1
  result_recover "$want"
  [[ -e "$want" ]] && echo created || echo none
) >"$TMP/out" 2>/dev/null
check "working in ralph's own repo: nothing to recover" "none" "$(cat "$TMP/out")"

printf '\nbuild_prompt\n'

mkdir -p "$TMP/prompts"
printf 'common\n' >"$TMP/prompts/_common.md"
printf 'stage\n' >"$TMP/prompts/implement.md"
printf '{"task":{}}' >"$TMP/work.json"
PROMPTS_DIR="$TMP/prompts" WORK_STAGE="implement" WORK_FILE="$TMP/work.json"
STORY_ID="US-001" STORY_TITLE="t" STORY_TIER="med" MODEL="m"
STAGE_POS=1 STAGE_COUNT=4 STAGE_FIRST=1 TASK_PR="" PIPE_FIRST="implement"
RESULT_FILE="$TMP/result.json" ATTEMPT_NOTE=""

RALPH_BASE_BRANCH=""
check "no base branch: the prompt names none" "0" "$(build_prompt | grep -c 'The base branch of this run')"
RALPH_BASE_BRANCH="f3"
check "a base branch: the prompt names it" "1" "$(build_prompt | grep -c 'The base branch of this run is `f3`')"
check "the prompt asks for the result path as written" "1" "$(build_prompt | grep -c 'Use the path exactly as written')"

printf '\n%s passed, %s failed\n' "$PASS" "$FAIL"
((FAIL == 0))
