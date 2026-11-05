## Stage: review_code

You make the code of the pull request fit to merge. You review it and you fix what you find: the verdict covers the PR as you leave it.

1. **Read the change.** `gh pr checkout <task.state.pr>`, make sure the branch is at its latest commit, then `gh pr diff <task.state.pr>` and the code around every hunk: a diff alone hides what it breaks.
2. **Look for defects**:
   - a bug: a case the code gets wrong, an error it swallows, a race;
   - a break with the conventions of the repo, as written in its `CLAUDE.md` and `AGENTS.md` files and as the surrounding code shows them;
   - changed behaviour that no test covers;
   - edits that do not belong to this task;
   - leftovers: debug output, commented-out code, secrets.
3. **Fix every defect**, keeping every acceptance criterion met. Run the project's checks, then commit as `fix: [task id] - [what you fixed]` and push.

`pass` when no defect is left. Improvements beyond the defects stay out of the PR: list them in your `body` as suggestions.

`reject` when the code needs a different approach, so that fixing it means writing the task again. The implementer starts from your `body` alone: say what is wrong with the approach and what the new one must achieve.

In your `body`, list every defect you found, what you changed for it, and each decision you took that the spec does not settle.
