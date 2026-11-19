---
name: niffler-sync-upstream
description: Safely synchronize the configured upstream main branch into Niffler main, merge that main into dev without overwriting dev work, report upstream changes and file-level overlap, then verify and publish both branches. Use when the repository needs an upstream refresh or when comparing upstream main with local dev before a merge.
---

# Synchronize upstream into `main` and `dev`

Use this workflow only in the Niffler repository when the caller has identified
or confirmed the upstream remote. The repository remotes and Git history are
the authority; do not infer an upstream URL from a claim or silently use an
unconfigured remote.

## Safety rules

- Never use `reset --hard`, force-push, delete branches, or replace `dev` with
  upstream content.
- Preserve existing local commits on both `main` and `dev`.
- Do not read `.env`, credentials, or other secret material.
- Before changing Git state, report the current branch, worktree status, local
  and remote tips, and any in-progress merge or rebase.
- Stop on conflicts until they are reviewed. Resolve by preserving local
  behavior and explicitly incorporating compatible upstream behavior; never
  accept one side wholesale for a conflict in a file changed by `dev`.

## Procedure

1. Inspect the repository:

   ```bash
   git status --short --branch
   git branch -vv
   git remote -v
   git log --oneline --decorate --graph --max-count=12
   ```

   Confirm that `upstream` is the intended upstream remote. If it is missing,
   ask for its URL. Fetch only after the remote is confirmed:

   ```bash
   git fetch upstream main
   git fetch origin main dev
   ```

2. Describe upstream before merging. Compare the previous local `main` tip
   with `upstream/main`:

   ```bash
   git log --oneline --decorate main..upstream/main
   git diff --stat main...upstream/main
   git diff --name-status main...upstream/main
   ```

   Group the report by subsystem (core, components, SDK, tests, docs/build)
   and include the number of commits and changed files. Do not call a change
   merged until the merge commit exists.

3. Synchronize local `main` with upstream using a normal merge. Preserve any
   local main-only commits:

   ```bash
   git switch main
   git merge --no-ff upstream/main -m "merge: sync upstream main"
   ```

   If `main` is already identical to `upstream/main`, say so and do not create
   a needless merge. If conflicts occur, inspect each conflict, resolve it,
   run the relevant checks, and commit only after `git diff --check` and
   `git ls-files -u` is empty.

4. Record the pre-merge `dev` tip and compute its common ancestor with the
   fetched upstream tip (`git merge-base dev upstream/main`). Use that commit
   as `<dev-baseline>` below and save both tips in the inline report. Compare
   the two independent sets of changes before merging:

   ```bash
   git diff --name-only <dev-baseline>..upstream/main
   git diff --name-only <dev-baseline>..dev
   comm -12 <(git diff --name-only <dev-baseline>..upstream/main | sort) \
            <(git diff --name-only <dev-baseline>..dev | sort)
   ```

   The intersection is the file-level overlap. For each overlapping file,
   inspect the actual hunks with `git diff` and report whether the changes are
   independent, compatible, or conflicting. File overlap alone is not proof
   of semantic conflict.

5. Merge the synchronized `main` into `dev` without rewriting history:

   ```bash
   git switch dev
   git merge --no-ff main -m "merge: sync upstream main into dev"
   ```

   Resolve conflicts one file at a time. Keep the established `dev` features
   (including provider protocols, Responses API support, and Rocky Linux
   compatibility) while integrating upstream additions. Run `git diff --check`
   and `git ls-files -u` before committing.

6. Verify the result before publishing:

   ```bash
   git status --short --branch
   git log --oneline --decorate --graph --max-count=12
   git rev-parse main origin/main dev origin/dev
   git merge-base --is-ancestor origin/main main
   git merge-base --is-ancestor origin/dev dev
   ```

   Run targeted tests for every conflict area, then the repository's normal
   build/test gate when feasible. Report failures exactly; a partial or
   interrupted test run is not a pass.

7. Publish only after verification:

   ```bash
   git push origin main
   git push origin dev
   ```

   Re-run the branch-tip checks and confirm both local branches match their
   corresponding `origin/*` tips. Report the merge commit, upstream commit,
   overlap files and classifications, checks run, and any remaining limits.

## Failure handling

- Missing `upstream`: stop and request the exact repository URL.
- Dirty unrelated worktree: stop or ask whether it may be included; never
  silently commit it.
- Conflict or failed check: leave the repository recoverable, list the files
  and failed command, and do not push the affected branch.
- Network or authentication failure: preserve local commits and report the
  failed remote operation; do not retry destructively.
- If upstream contains changes that overlap a local `dev` feature, retain the
  local feature unless the caller explicitly approves a behavioral replacement.
