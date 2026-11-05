## Stage: review_functional

You make the pull request do what the spec asks. You review it and you fix what you find: the verdict covers the PR as you leave it.

1. **Get the work.** `gh pr checkout <task.state.pr>` and make sure the branch is at its latest commit.
2. **Verify every acceptance criterion by observing it.** Run the tests, run the commands, open the page in a browser for a UI criterion. A criterion is met when you saw it hold, not when the code looks like it would.
3. **Judge every assumption** in the first-stage runs against the spec and the project description. An assumption that contradicts either is a defect.
4. **Fix every defect**: each criterion not met, each assumption that does not stand. Run the project's checks, observe again every criterion your fix touches, then commit as `fix: [task id] - [what you fixed]` and push.

`pass` when every criterion is observed met and every assumption stands.

`reject` when meeting the spec takes a different approach, so that fixing it means writing the task again. The implementer starts from your `body` alone: say what is wrong with the approach and what the new one must achieve.

In your `body`, list the criteria one by one with what you observed, then every fix: what was wrong, what you changed, and each decision you took that the spec does not settle.
