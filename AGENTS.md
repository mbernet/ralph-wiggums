# Ralph Agent Instructions

If a person asked you to set up Ralph for their project, not to change Ralph, follow [`SETUP.md`](SETUP.md) instead: this file is about developing Ralph itself.

## Overview

Ralph is an autonomous agent loop. `ralph.sh` runs an AI coding tool (Amp or Claude Code) once per stage of a task, each time in a fresh context, until the Ralph task API has no more work for this worker. The API is shared by several workers; `docs/central-prd-spec.md` is its spec.

## Commands

```bash
# Tests. No network: the API is faked and the agent is a shell script.
./tests/api.test.sh
./tests/window.test.sh
./tests/events.test.sh
make -C ui test        # ralph-ui; its view tests replay real dry runs of ralph.sh

# Lint
shellcheck -x -S style ralph.sh tests/api.test.sh tests/events.test.sh

# Iterate on ralph.sh output with no API calls
./ralph.sh --dry-run --fast
./ralph.sh --dry-run --scenario throttle|hardstop|ratelimit|maxturns|complete|reject|sick|nowork|conflict

# Build ralph-ui, then see a past run as it draws it
make -C ui
ui/bin/ralph-ui replay .ralph/logs/<run>

# Show the resolved config, the project and the pace decision. Claims nothing.
./ralph.sh --explain

# Load a PRD into the API, then work on it
./ralph.sh --import prd.json
./ralph.sh --tool claude [max_iterations]
```

## Key Files

- `ralph.sh` - The bash loop: asks the API for work, spawns a fresh AI instance for it, registers the result
- `prompts/` - `_common.md`, one prompt per pipeline stage, and `doctor.md`
- `prd.json.example` - A PRD in the import format. It also seeds the fake API of a dry run
- `docs/central-prd-spec.md`, `docs/task.schema.json` - The API the loop talks to
- `skills/` - `prd` writes a PRD, `ralph` converts it to `prd.json` and imports it
- `docs/events.md` - The event stream `ralph.sh` writes for its renderers
- `ui/` - `ralph-ui` (Go, Bubble Tea): `event` decodes the stream, `state` folds it, `view` draws it, `app` runs it

## Patterns

- Each iteration spawns a fresh AI instance with clean context
- Memory persists via git and the API: the runs of each task and the summaries of the finished ones
- A task must be small enough to implement in one context window

### ralph.sh conventions

- Only `ralph.sh` talks to the API. The agent gets the whole `POST /work` answer in its
  prompt, minus the claim, answers through the result file named there, and is started
  without the token in its environment.
- The script, like the server, looks only at the **position** of a stage: first, later,
  last. Never branch on a stage name. A new stage is a new file in `prompts/`.
- What the result must carry is decided by position and injected by `result_rules`, so
  the stage prompts only describe the work. Keep it that way: one source of truth.
- A retry keeps the claim, and is only allowed while `claim.expiresAt` leaves
  `RALPH_CLAIM_MARGIN_S` of room. Past that the task may be on another worker's branch.
- `_http` is the single way out to the API. The tests and `--dry-run` swap it for
  `_http_fake`, a server written in jq that keeps its state in a file. When the API
  changes, change the fake with it, then check against a real server: a green suite only
  proves the loop agrees with the fake.
- Must run on **bash 3.2** (stock macOS): no associative arrays, no `declare -g`.
- Money is carried as **integer micro-dollars**, so bash needs no float arithmetic.
- All date parsing goes through jq's `fromdateiso8601`; bash only ever calls `date +%s`
  (`date -d` does not exist on BSD).
- Records are joined with `\x1f`, never a tab: `read` treats tab as IFS whitespace and
  collapses runs of it, so an absent field would silently shift every later one.
- Under `set -e`, a function must not end in `((cond)) && x` - it returns 1 and aborts
  the caller. End such functions with an explicit `return 0`.
- Every opportunistic JSON read goes through `jqf <default>` so a half-written file
  cannot abort the run.
- Rate-limit state comes from the API rate-limit headers, the stream's own
  `rate_limit_event`, and `~/.claude.json` → `cachedUsageUtilization`, in that order.

### Events and ralph-ui

- Everything a renderer shows is an event: `ev <type> [jq args] <filter>` in
  `ralph.sh`, catalogued in `docs/events.md`. A new event, or a field, is a change to
  `ralph.sh`, `docs/events.md` and `ui/internal/event` together.
- ralph-ui only reads `events.ndjson`. It asks the loop for something through the files
  in `session.start.control` (`stop`, `skip`), never through the API or a signal of its
  own; Ctrl-C is passed on as the terminal would.
- The plain output stays complete without ralph-ui. With it, the plain output goes to
  `console.log`, so `ralph.sh` re-runs `ui_init` and sees no terminal from then on.
- `ui/internal/view` returns strings for a given width and is tested without a terminal.
  Assert on data (ids, numbers, the feedback text) and on every line fitting the width,
  never on the wording.
