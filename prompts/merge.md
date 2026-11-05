## Stage: merge

The pull request is reviewed. You get it into `main` and write down what the project learnt from it.

1. **Bring the branch up to date.** `gh pr checkout <task.state.pr>`, `git fetch origin`, merge `origin/main` into the branch. Resolve a conflict when the resolution is mechanical. When it needs the feature rethought, `reject` and name the conflicting files and why.
2. **Run the project's checks again** on the merged result, then push.
3. **Wait for CI.** `gh pr checks <pr> --watch`. When it reports no checks, the repo has no CI: go on to the merge. A check that fails because of this change is a `reject`: give the check and its output. A check that fails for a cause outside the task gets one re-run; if it fails again, the task is `sick`.
4. **Merge.** `gh pr merge <pr> --squash --delete-branch`, then confirm with `gh pr view <pr> --json state` that the state is `MERGED`.

The stage is done when the pull request is `MERGED`. Only then `pass`.

Your `summary` is the memory of the project: every later task receives it. Distil the `body` of each run into what was done and where it lives in the code, in two or three lines. Add a lesson only when a run found something out the hard way that the code does not show and a later task would have to find out again. Most tasks leave no lesson. Leave out the story of how the task got there.
