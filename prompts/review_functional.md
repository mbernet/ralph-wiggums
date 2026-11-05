## Stage: review_functional

You make sure the pull request does what the spec asks, and you change it where it does not.

1. `gh pr checkout <task.state.pr>`, at its latest commit.
2. **Understand the feature** from the code of the change, then hold it against the spec: every acceptance criterion, and every assumption in the first-stage runs.
3. **Fix what does not fit**: a criterion the code does not meet, a case it gets wrong, an assumption that contradicts the spec or the project description. After a fix, run the project's checks, commit as `fix: [task id] - [what you fixed]` and push.

`pass` when the PR fits the spec. `reject` only when fitting it takes a different approach: say what is wrong with it and what the new one must achieve, since the implementer starts from your `body` alone.

In your `body`, go through the criteria one by one, then list what you changed and each decision you took that the spec does not settle.
