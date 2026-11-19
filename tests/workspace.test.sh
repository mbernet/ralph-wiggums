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

# ls-remote matches a pattern against the end of a ref: user/f9 is not f9.
git_quiet -C "$TMP/project" push origin main:refs/heads/user/f9
(
  cd "$TMP/project" || exit 1
  RALPH_WORKTREE="" RALPH_BASE_BRANCH="f9"
  preflight_workspace
) >/dev/null 2>&1
check "RALPH_BASE_BRANCH that only ends another branch's name: stops" "1" "$(($? != 0))"

(
  cd "$TMP/project" || exit 1
  DRY_RUN=1 RALPH_WORKTREE="" RALPH_BASE_BRANCH="f9"
  preflight_workspace
) >/dev/null 2>&1
check "dry run: the base branch is not looked up" "0" "$?"

printf '\nrelative paths with RALPH_WORKTREE\n'

# Relative RALPH_DIR and RALPH_PROMPTS_DIR mean where ralph.sh was started, not
# the worktree it moves into.
cp -R "$TEST_DIR/../prompts" "$TMP/project/prompts"
(
  cd "$TMP/project" || exit 1
  RALPH_DIR=.ralph RALPH_PROMPTS_DIR=prompts RALPH_WORKTREE=../project-wt \
    "$TEST_DIR/../ralph.sh" --dry-run --fast --scenario complete --quiet
) >"$TMP/out" 2>&1
check "a dry run with relative paths and a worktree ends well" "0" "$?"
check "its logs stay where ralph.sh was started" "1" "$(find "$TMP/project/.ralph/logs" -name '*.result.json' 2>/dev/null | head -1 | grep -c .)"
check "nothing of ralph's is written into the worktree" "0" "$(find "$TMP/project-wt/.ralph" 2>/dev/null | grep -c .)"
rm -rf "$TMP/project/prompts" "$TMP/project/.ralph"

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

# The same mistake, relative to ralph's own directory instead of the project.
rm -f "$want"
short=".ralph/logs/run/iter-01.result.json"
(
  cd "$TMP/project-wt" || exit 1
  mkdir -p "$(dirname "$short")"
  printf '{"outcome":"short"}' >"$short"
  result_recover "$want"
  [[ ! -e "$short" ]] && cat "$want"
) >"$TMP/out" 2>/dev/null
check "a result relative to ralph's directory is moved too" '{"outcome":"short"}' "$(cat "$TMP/out")"
rm -rf "$TMP/project-wt/.ralph"

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

# gh answers for the task's pull request: its base is $FAKE_PR_BASE, and an
# empty one is a gh that fails.
mkdir -p "$TMP/bin"
cat >"$TMP/bin/gh" <<'SH'
#!/bin/sh
[ "$1 $2" = "pr view" ] && [ -n "$FAKE_PR_BASE" ] || exit 1
printf '%s\n' "$FAKE_PR_BASE"
SH
chmod +x "$TMP/bin/gh"
PATH="$TMP/bin:$PATH"
export FAKE_PR_BASE=""

base_in_prompt() { # base_in_prompt <branch> -> how many times the prompt names origin/<branch>
  task_base >/dev/null 2>&1
  build_prompt | grep -c "origin/$1"
}

RALPH_BASE_BRANCH="" TASK_PR=""
check "no base branch: the prompt names origin/HEAD" "1" "$(base_in_prompt HEAD)"
RALPH_BASE_BRANCH="f3"
check "a base branch: the prompt names it" "1" "$(base_in_prompt f3)"

TASK_PR="https://github.com/o/r/pull/7" STAGE_FIRST=0
FAKE_PR_BASE="f3" RALPH_BASE_BRANCH=""
check "a later stage follows the base of the task's PR, not this worker's" "1" "$(base_in_prompt f3)"
FAKE_PR_BASE="f3" RALPH_BASE_BRANCH="f4"
check "the PR's base wins over a different RALPH_BASE_BRANCH" "0" "$(base_in_prompt f4)"
FAKE_PR_BASE="" RALPH_BASE_BRANCH="f4"
check "gh cannot answer: the configured base" "1" "$(base_in_prompt f4)"
DRY_RUN=1 FAKE_PR_BASE="f3" RALPH_BASE_BRANCH="f4"
check "dry run: gh is not asked" "1" "$(base_in_prompt f4)"
DRY_RUN=0 TASK_PR="" STAGE_FIRST=1 FAKE_PR_BASE="" RALPH_BASE_BRANCH="f3"
check "the prompt asks for the exact result path" "1" "$(build_prompt | grep -c 'to this exact absolute path')"

printf '\n%s passed, %s failed\n' "$PASS" "$FAIL"
((FAIL == 0))
