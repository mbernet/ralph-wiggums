# Events

Every run of `ralph.sh` writes what it does to `$RALPH_DIR/logs/<run>/events.ndjson`: one JSON object per line, appended as it happens. A renderer follows that file, live or afterwards, and never talks to the Ralph API. `ralph-ui` (`ui/`) is the renderer this repository ships; `jq` is another:

```bash
tail -f .ralph/logs/*/events.ndjson | jq -c 'select(.type == "stage.end")'
```

The plain output of `ralph.sh` is written as it always was. The events are an addition, and they are written with `--quiet` too.

## Envelope

| Field | Meaning |
|---|---|
| `v` | Version of the format, `1`. A field added is not a new version; a field renamed or removed is. |
| `ts` | Unix seconds. |
| `type` | One of the types below. A renderer ignores the types it does not know. |

Money is integer micro-dollars (`costU`), like everywhere in `ralph.sh`. Times are unix seconds. A field with no value is `null`.

## Order

`session.start` is the first event and `session.exit` the last; nothing follows it. In between, each stage of a task is a `stage.start`, the `agent.*` events of its agent, and a `stage.end`. A retry of the same stage repeats the three with `attempt` one higher. The `agent.*` events of a stage are on disk before its `stage.end`.

## Types

### Session

- **`session.start`**: `run`, `project`, `description`, `worker`, `api` (the URL, or `fake` in a dry run), `branch`, `pipeline` (stage names), `tool`, `toolVersion`, `tiers` (`{low, med, high}` → model), `pace` (`{mode, budgetPct, weekGuardPct, reason}`), `maxIterations`, `dryRun`, `scenario`, `story` (the `--story`, or null), `logs`, `pid` (of `ralph.sh`), `control` (`{stop, skip}`, see below).
- **`session.end`**: `state` (`complete`, `blocked`, `exhausted`, `api`, `stuck`, `stopped`, `incomplete`), `ran`, `registered`, `failed`, `retried`, `costU`, `tokens` (`{in, out, cacheRead, cacheWrite}`), `wallS`, `agentS`, `throttledS`, `tiers` (`{low|med|high: {runs, costU}}`), `ledger`, `logs`, `next` (the command that carries on, or null). Missing when `ralph.sh` dies before the end of the session.
- **`session.exit`**: `code`, the exit code of `ralph.sh`. Always written, from the EXIT trap.

### The project

- **`project`**: `counts` (`{total, done, claimed, sick, needsHuman, blocked}`, as in section 4.1 of the spec). Sent whenever `ralph.sh` reads the project.
- **`board`**: `tasks`, each the card of the panel (section 9 of the spec): `id`, `title`, `priority`, `stage`, `runs` (how many), `sick`, `needsHuman`, `claim` (`{worker, expiresAt}` while active, else null), `pr`, `rejects`, `assumptions` (how many were declared), `doneAt`, `why` (the last run's body, while sick), `blockedBy` (the dependency that waits for a person). Sent at the start, after every run registered or lost, and when there is no work.
- **`work.none`**: `finished`, `idleS`, `counts`. The server had nothing eligible.

### Quota and pacing

- **`window`**: `src` (`none` when nothing could read it), `fiveHour` and `sevenDay` (`{pct, resetsAt}`), `fresh`, `fetchedAt`, `spendU`, `budgetPct`, `weekGuardPct`. Sent after every reading.
- **`pace`**: `action` (`throttle`, `wait_window`, `stop`), `sleepS`, `why`, `estimatePct`, `tier`.
- **`wait.start`**: `kind` (`throttle`, `window`, `idle`, `quota`, `retry`, `backoff`, `api`), `seconds`, `until`.
- **`wait.end`**: `kind`, `skipped`.

### A stage

- **`stage.start`**: `iter`, `maxIterations`, `attempt` (1 on the first), `task` (`{id, title, priority, tier, pr}`), `stage` (`doctor` for a sick task), `pos` and `count` (place in the pipeline; 0 for the doctor), `taskStage`, `model`, `tool`, `claimExpires`, `stream` (the raw log of the agent).
- **`stage.end`**: `iter`, `attempt`, `task`, `stage`, `class` (`ok`, `turns`, `budget`, `crash`, `ratelimit`, `auth`, `error`), `subtype`, `outcome` (`pass`, `reject`, `sick`, `conflict`, `dropped`, `none`), `next` (where the task stands now), `byScript` (the sick run was written by `ralph.sh`), `error` (why there is no result), `durS`, `turns`, `costU`, `runCostU`, `tokens`, `model`, `commit` (`{sha, subject}` or null), `winDelta`, `pr`. What the agent handed in, as sent to the API: `body`, `assumptions`, `summary`.

### The agent

From the claude stream, one event per block:

- **`agent.init`**: `session`, `model`.
- **`agent.text`**: `text`, on one line, up to 2000 characters.
- **`agent.thinking`**: no fields.
- **`agent.tool`**: `id`, `name`, `arg` (the path, the description or the command; up to 500 characters).
- **`agent.result`**: `id`, `ok`, `error` (when not ok).
- **`agent.compact`**: the context was compacted.
- **`agent.ratelimit`**: `status`, `kind`, `resetsAt`, `pct`.

From an agent that writes plain text (amp, `RALPH_AGENT_CMD`): **`agent.line`** with `text`.

### Notices

- **`notice`**: `level` (`info`, `ok`, `warn`, `error`) and `text`: every message of the plain output that is not one of the events above.

## Control

A renderer that owns the keyboard asks the loop for two things through files, named in `session.start.control`. A person can create them too, for a worker started with `nohup`:

- **`stop`**: the worker registers the stage in progress, starts no other, and ends the session as `stopped`. It also ends a wait between stages at once. Removing the file before then takes the request back.
- **`skip`**: ends the wait in progress, as Ctrl-C does in the plain output. Consumed when seen.

## Changing the format

The writer is `ev` and its callers in `ralph.sh`; the reader is `ui/internal/event`. Change both, and this page with them. The tests of `ui/internal/view` replay real dry runs of `ralph.sh`, so a mismatch between the two shows up there.
