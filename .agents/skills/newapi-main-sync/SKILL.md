---
name: newapi-main-sync
description: Fetch upstream newapi/main, merge it into the local main branch, safely resolve unrelated conflicts, and ask the user to choose when conflicts are ambiguous.
---

# Newapi Main Sync

## Purpose

Update the local `main` branch with the latest code from the `newapi` remote's `main` branch. Use this when the user asks to pull, sync, update, or merge `newapi/main` into local `main`.

## Workflow

Run from the repository root.

### 1. Preflight

Inspect the repository before changing anything:

```bash
git rev-parse --show-toplevel
git status --short
git branch --show-current
git remote get-url newapi
git rev-parse --verify main
```

If `newapi` is missing, stop and ask the user to add or confirm the upstream remote. If local `main` is missing, stop and ask which branch should receive the merge.

If a merge, rebase, cherry-pick, or revert is already in progress, stop and report the in-progress operation before doing anything else.

If the working tree has uncommitted changes:

- Do not discard, stash, commit, or overwrite them unless the user explicitly asks.
- If already on `main`, decide whether the merge can proceed without touching those files. If not clearly safe, stop and ask the user whether to stash, commit, or cancel.
- If not on `main`, stop and ask the user how to handle the uncommitted changes before switching branches.

Never run destructive commands such as `git reset --hard`, `git checkout -- <path>`, branch deletion, or force push as part of this skill.

### 2. Fetch upstream

Fetch the latest upstream `main`:

```bash
git fetch newapi main
git rev-parse --short newapi/main
git log --oneline --left-right --cherry-pick main...newapi/main
```

Summarize whether local `main` is behind, ahead, diverged, or already up to date.

### 3. Switch to local main

If the current branch is not `main` and the working tree is clean, switch to `main`:

```bash
git switch main
```

### 4. Merge upstream

Merge upstream into local `main`:

```bash
git merge --no-edit newapi/main
```

If the merge completes cleanly, continue to verification and reporting.

### 5. Conflict handling

When conflicts occur, first list them:

```bash
git diff --name-only --diff-filter=U
rg -n '<<<<<<<|=======|>>>>>>>' .
```

Inspect each conflicted file and classify the conflict before editing.

Safe-to-resolve unrelated conflicts include cases where the sides touch independent features, comments, formatting, generated ordering, translations, or adjacent code that can be combined without changing either side's behavior. Resolve these conservatively by preserving both intended changes and the existing project conventions. After resolving, report each automatically resolved file and the reason it was safe.

Ambiguous conflicts must be left for the user to choose. Treat a conflict as ambiguous when it changes the same behavior, public API, database schema or migration, billing logic, authentication, quota/rate-limit behavior, provider routing, model pricing, request/response DTO semantics, protected project identity, or any code path where keeping both sides would be a product decision.

For ambiguous conflicts, do not guess. Report:

- the conflicted file;
- what local `main` is trying to keep;
- what `newapi/main` is trying to introduce;
- the practical impact of each side;
- 2-3 concrete resolution options, with a recommended option only when the evidence is strong.

If all conflicts are safely resolved, stage only the resolved files and complete the merge:

```bash
git add <resolved-files>
git commit --no-edit
```

Do not stage unrelated user changes.

### 6. Verification

Run focused checks based on the files changed by the merge:

- Go backend changes: `go test ./...`
- Default frontend changes: from `web/default/`, use Bun, usually `bun run build`
- i18n-only frontend changes: from `web/default/`, run `bun run i18n:sync` when keys or locale files changed

If a check is too expensive or cannot run because dependencies or services are missing, say that plainly and include the next best manual check.

### 7. Report

Reply in Chinese with:

- the fetched `newapi/main` short commit;
- whether the merge was fast-forward, merge commit, clean merge, conflict-resolved merge, or blocked by ambiguous conflicts;
- conflicts automatically resolved, if any;
- ambiguous conflicts and options, if any;
- verification commands and results;
- current `git status --short`.
