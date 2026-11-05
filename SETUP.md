# Set up Ralph Wiggums for a project

You are an agent, and a person asked you to set up Ralph Wiggums so that their project is developed autonomously. This file is the whole job. `AGENTS.md` is about changing Ralph itself; you can leave it aside.

The end state: a **worker clone** of their project with Ralph inside it, connected to their Ralph API, with the project's tasks imported and a start command you checked. Starting the loop is the person's call, since it spends their agent quota and merges pull requests into their repo: hand them the command, and run it only if they ask you to.

Call the checkout of this repository `<ralph>`. If you are reading this on GitHub, clone `https://github.com/mbernet/ralph-wiggums` first, anywhere outside their project.

## 1. Ask the person

Ask once, in a single message, for whatever you cannot find yourself:

- **The Ralph server and its token.** People give the host, such as `https://liquia.frb.io/`. The API lives under `/api/ralph` on it.
- **The project repository**: a GitHub URL, or a local clone you can read the remote from.
- **The spec**: a file in the repo, a document, or a description in their words.
- **The agent CLI** the worker runs: `claude` unless they say `amp`.

Tell them in the same message what Ralph will do: open one pull request per task and merge it into the default branch, with no person in between unless something goes wrong.

Done when you have all four.

## 2. Check the machine

Run each check and fix what you can. What you cannot fix, tell the person with the exact command they have to run.

- `jq`, `curl`, `git` and `gh` are installed, and `gh auth status` is logged in.
- `gh repo view <repo> --json viewerPermission` answers `WRITE`, `MAINTAIN` or `ADMIN`: the agents push branches and merge.
- The agent CLI answers: `claude --version` (2.1 or newer) or `amp --version`.
- The project's own build and test commands work from a non-interactive shell, because that is where the agents run them. A tool found only through an interactive shell profile (a `PATH` line in `~/.zshrc`) is invisible to them: put it on the `PATH` the worker starts with.
- The token works: `curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer <token>" <host>/api/ralph/projects/<project>` answers `404` (the project does not exist yet) or `200`. `401` means the token is wrong.
- Optional: `go` 1.24 or newer, to build `ralph-ui`, the terminal interface. Without it Ralph prints plain output.

Done when every check passes.

## 3. Make the worker clone

A worker switches branches and stashes what it finds uncommitted, so it never runs in a working copy a person uses.

1. Clone the project next to the person's copy, as `<project>-ralph`, unless they want it elsewhere.
2. Copy Ralph into it: `<ralph>/ralph.sh` and `<ralph>/prompts/` into `scripts/ralph/`. With `go`, run `make -C <ralph>/ui` and copy `<ralph>/ui/bin/ralph-ui` to `scripts/ralph/ui/bin/`.
3. Add `scripts/ralph/` to the clone's `.git/info/exclude`. The agents commit everything in the tree; this keeps Ralph, its logs and the token out of their commits without touching the project's `.gitignore`.
4. Write `scripts/ralph/.ralph/env`, one `KEY=value` per line:

   ```
   RALPH_API_URL=<host>/api/ralph
   RALPH_API_TOKEN=<token>
   RALPH_PROJECT=<project>
   RALPH_WORKER=<worker>
   ```

   `<project>` is a slug that matches `^[a-z0-9][a-z0-9-]{0,62}$`, the repository name by default. `<worker>` names this machine and stays the same across runs: the server gives a worker back its own tasks first.

The token goes in that file and nowhere else: never in a tracked file, a commit or a pull request.

Done when `git status` in the clone shows a clean tree.

## 4. Turn the spec into tasks

Follow `<ralph>/skills/ralph/SKILL.md`, working in `scripts/ralph/` of the clone. If the spec is only an idea, follow `<ralph>/skills/prd/SKILL.md` first to write it out.

Before the import, show the person the tasks: id, title, dependencies and the acceptance criteria. Splitting the spec is where their judgement matters most, so import once they agree.

Done when `./ralph.sh --import prd.json` reports every task created and none refused.

## 5. Check and hand over

From the clone, run `./scripts/ralph/ralph.sh --tool <cli> --explain`. It claims nothing. It must show the API, the project, its pipeline, the tasks you imported and a pace action of `go`. Fix whatever it complains about.

Then give the person:

- The start command: `cd <clone> && ./scripts/ralph/ralph.sh --tool <cli>`. It stops by itself when no task is left; each task takes one run per pipeline stage.
- Where to follow it: the pull requests on GitHub, and the logs in `scripts/ralph/.ralph/logs/`.
- One option worth knowing: `RALPH_STAGE_IMPLEMENT=low` and `RALPH_MODEL_LOW=haiku` in `.ralph/env` make a cheap model implement while the default one reviews.

Done when `--explain` is clean and the person has the command.
