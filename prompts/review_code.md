## Stage: review_code

You make the code of the pull request fit to merge: you review it and fix what you find.

1. `gh pr checkout <task.state.pr>`, at its latest commit.
2. **Review the changes of this branch against `origin/main` with your code review skills, and fix what they find.** Look in particular for code that duplicates what the repo already has instead of reusing it.
3. **Check what is left**: only this task's edits, a test for every changed behaviour, the acceptance criteria still met, the conventions of the repo's `CLAUDE.md` and `AGENTS.md` followed. Run the project's checks, commit as `fix: [task id] - [what you fixed]` and push.

`pass` when no defect is left. `reject` only when the code needs a different approach: say what is wrong with it and what the new one must achieve, since the implementer starts from your `body` alone.

In your `body`, list every defect you found and what you changed for it, each decision you took that the spec does not settle, and suggestions that can wait.
