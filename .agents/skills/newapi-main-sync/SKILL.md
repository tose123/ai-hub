---
name: newapi-main-sync
description: Fetch upstream newapi/main, merge it into local main, preserve local behavior across textual and semantic conflicts, audit post-merge repairs for regressions, summarize updates, and recommend key test areas.
---

# Newapi Main Sync

## Purpose

Update the local `main` branch with the latest code from the `newapi` remote's `main` branch. Use this when the user asks to pull, sync, update, or merge `newapi/main` into local `main`.

Conflict precedence rule: when `newapi/main` and local `main` change the same functionality, local `main` is the source of truth. Preserve local `main` behavior by default, and only carry over upstream pieces that are clearly compatible and do not alter the local behavior contract. Apply this rule to semantic conflicts and post-merge repairs even when Git reports no textual conflict.

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
LOCAL_BEFORE=$(git rev-parse HEAD)
git rev-parse --short "$LOCAL_BEFORE"
git fetch newapi main
UPSTREAM_HEAD=$(git rev-parse newapi/main)
git rev-parse --short "$UPSTREAM_HEAD"
git log --oneline --left-right --cherry-pick main...newapi/main
```

Record the full pre-merge local `HEAD` as `LOCAL_BEFORE` and the full fetched upstream commit as `UPSTREAM_HEAD` in working notes. Shell variables may not persist between tool calls; use the recorded literal hashes later or reassign the variables in the same shell invocation. Summarize whether local `main` is behind, ahead, diverged, or already up to date.

### 3. Build the local behavior ledger

Before merging, identify the behavior that exists only on local `main`:

```bash
MERGE_BASE=$(git merge-base "$LOCAL_BEFORE" "$UPSTREAM_HEAD")
git rev-parse --short "$MERGE_BASE"
git log --oneline --no-merges "$MERGE_BASE".."$LOCAL_BEFORE"
git diff --name-status "$MERGE_BASE".."$LOCAL_BEFORE"
git diff --name-status "$MERGE_BASE".."$UPSTREAM_HEAD"
```

Save the merge base as `MERGE_BASE`. Build a working ledger of local-only functional commits, including each commit's intent, affected paths, observable behavior, and relevant tests or configuration. Documentation-only and mechanical formatting commits may be summarized together.

Treat paths changed on both sides as semantic-conflict candidates even if Git would merge them cleanly. For each candidate, inspect the local commits and representative patches:

```bash
git log --oneline "$MERGE_BASE".."$LOCAL_BEFORE" -- <path>
git log -p "$MERGE_BASE".."$LOCAL_BEFORE" -- <path>
git diff "$MERGE_BASE".."$UPSTREAM_HEAD" -- <path>
```

Include behavior affected indirectly by renames, moved packages, shared helpers, tests, defaults, configuration parsing, and call-site contracts. When feasible, run focused tests for semantic-conflict candidates before merging and record pre-existing failures; do not later misclassify a baseline failure as merge fallout.

### 4. Switch to local main

If the current branch is not `main` and the working tree is clean, switch to `main`:

```bash
git switch main
```

### 5. Merge upstream

Merge upstream into local `main`, preferring local `main` on conflicting hunks:

```bash
git merge --no-edit -X ours newapi/main
```

`-X ours` only resolves overlapping textual hunks. A clean merge does not prove that local behavior survived. After any successful merge, save the merged `HEAD` as `MERGED_HEAD` before making follow-up repairs, then continue to the preservation audit:

```bash
MERGED_HEAD=$(git rev-parse HEAD)
git rev-parse --short "$MERGED_HEAD"
```

### 6. Conflict handling

When conflicts occur, first list them:

```bash
git diff --name-only --diff-filter=U
rg -n '^(<<<<<<<|=======|>>>>>>>)' .
```

Inspect each conflicted file and classify the conflict before editing.

Default rule: if both sides touch the same functionality or behavior, keep local `main`'s implementation. Treat the local side as the baseline and selectively re-apply only upstream edits that are obviously safe, such as comments, imports, tests, translations, formatting, or adjacent helper code that does not change runtime behavior.

Safe-to-resolve conflicts include both:

- unrelated conflicts, where the sides touch independent features, comments, formatting, generated ordering, translations, or adjacent code that can be combined without changing either side's behavior;
- same-feature conflicts where local `main` should win, and the upstream side only contributes non-behavioral or clearly compatible pieces around the local implementation.

Resolve these conservatively by preserving local `main` behavior and the existing project conventions. After resolving, report each automatically resolved file and whether it was merged as unrelated-compatible or local-main-preferred.

Tests are evidence of a contract, not automatic authority over newer local behavior. If an older test conflicts with a newer local functional commit, preserve the newer local behavior and update or replace the stale test. Use commit chronology, `git blame`, commit messages, and surrounding tests to identify the intended contract. If intent remains ambiguous, escalate instead of changing production behavior merely to make a test pass.

Escalate only when keeping local `main` behavior is not mechanically clear. Treat a conflict as escalation-worthy when it involves rename/delete conflicts, binary files, generated artifacts whose source is unknown, or structural changes where simply preferring local hunks may leave the code broken. Also escalate if the upstream side introduces a required dependency, schema shape, interface, or call-site contract that must be partially adopted for the local implementation to keep building or running.

For escalation-worthy conflicts, do not guess. Report:

- the conflicted file;
- what local `main` is trying to keep;
- what `newapi/main` is trying to introduce;
- why a simple local-main-preferred resolution may still be unsafe;
- 2-3 concrete resolution options, with the recommended option biased toward preserving local `main` behavior when feasible.

If all conflicts are safely resolved, stage only the resolved files and complete the merge:

```bash
git add <resolved-files>
git commit --no-edit
```

Do not stage unrelated user changes.

### 7. Post-sync update review

After a successful merge, inspect what changed between `LOCAL_BEFORE` and the merged `HEAD` before reporting:

```bash
git diff --stat "$LOCAL_BEFORE"..HEAD
git diff --name-status "$LOCAL_BEFORE"..HEAD
git log --oneline --no-merges "$LOCAL_BEFORE"..HEAD
```

#### Local behavior preservation gate

Verify every entry in the local behavior ledger against the merged result. For renamed or refactored code, verify the observable behavior rather than requiring identical source text.

```bash
git diff --find-renames "$UPSTREAM_HEAD"..HEAD -- <local-feature-paths>
git log --oneline "$MERGE_BASE".."$LOCAL_BEFORE" -- <local-feature-paths>
git blame "$MERGED_HEAD" -- <path>
```

Before making a post-merge build, lint, typecheck, or test repair, classify it as mechanical or behavioral. Import-path updates, symbol renames, formatting, and removal of duplicated merge residue are mechanical only when runtime behavior is unchanged. Keep post-merge repairs uncommitted until the preservation gate passes; do not create a generic `fix(sync)` commit first and audit it afterward.

A post-merge repair must not delete or change behavior last introduced by a local-only functional commit unless one of these conditions holds:

- the replacement demonstrably preserves the same local observable contract and a focused regression test covers it; or
- the user explicitly approves changing the local contract.

When a repair changes runtime defaults, explicit configuration semantics, billing, retry policy, auth, routing, persisted data, API responses, or UI workflows, inspect the affected line history and local commit before editing. For configuration logic, verify unset/default, explicit-value, and disabling-sentinel cases separately. Never restore an older upstream implementation solely because an older test or message describes it.

After all repairs, review the repair-only diff and account for every behavioral hunk:

```bash
git diff --stat "$MERGED_HEAD"..HEAD
git diff --find-renames "$MERGED_HEAD"..HEAD
```

If any local ledger entry is missing, weakened, or cannot coexist safely with a required upstream contract, stop and report the conflict with concrete options. Passing tests, lint, typecheck, or build does not waive this gate.

Summarize main updates by product area, not by raw file list:

- Backend: Go modules and changes under `controller/`, `service/`, `model/`, `relay/`, `middleware/`, `setting/`, `common/`, `dto/`, `constant/`, `i18n/`, `oauth/`, `pkg/`, `router/`.
- Frontend UI: changes under `web/`, especially `src/`, `package.json`, `bun.lock`, `rsbuild.config.*`, Tailwind/CSS, i18n locale files.
- Deployment/config/docs: Docker, compose, scripts, config examples, CI, docs, migrations.

When building the update summary, inspect representative diffs for important files instead of inferring only from filenames:

```bash
git diff "$LOCAL_BEFORE"..HEAD -- <important-paths>
```

Call out likely user-visible behavior, provider/channel changes, billing/quota changes, auth/security changes, DB/schema changes, config/env changes, dependency upgrades, and UI/i18n changes. Keep the summary concise, but include enough detail for the user to know what changed.

Derive testing focus from the changed areas. Prefer concrete checks such as:

- Backend relay/provider paths changed: test affected provider chat/completions, streaming, tool calls, images/files if touched, and error mapping.
- Billing/quota/model pricing changed: test pre-consume, settlement, log display, zero/false optional request values, and group/model ratio cases.
- DB/model/migration changed: test SQLite, MySQL, and PostgreSQL migration/startup paths where feasible.
- Auth/middleware/rate-limit changed: test login/token/OAuth/passkey or limit behavior as applicable.
- Frontend UI changed: build `web/`, open affected pages, test create/edit/delete flows, loading/error states, and responsive layout.
- i18n changed: run or inspect i18n sync and test language switching for touched screens.
- Config/deployment changed: test fresh config defaults and upgraded existing config.

### 8. Verification

Run focused checks based on the files changed by the merge. Do not report the sync as complete until the required checks are clean or a concrete blocker is reported:

- Go backend changes: `go test ./...`
- Frontend changes: from `web/`, use Bun. Always run both `bun run typecheck` and `bun run build`; a successful Rsbuild build is not sufficient because it can miss TypeScript duplicate declarations, missing imports, and invalid lazy route chunks. `bun run typecheck` must be clean, not merely grepped for known errors.
- i18n-only frontend changes: from `web/`, run `bun run i18n:sync` when keys or locale files changed

For any merge that changes `web/src/`, also run targeted lint on changed source files before reporting success:

```bash
{
  git diff --name-only "$LOCAL_BEFORE"..HEAD -- 'web/src/**/*.ts' 'web/src/**/*.tsx'
  git diff --name-only -- 'web/src/**/*.ts' 'web/src/**/*.tsx'
} | sort -u | sed 's#^web/##' > /tmp/newapi-main-sync-src-files.txt
test ! -s /tmp/newapi-main-sync-src-files.txt || (cd web && xargs bunx oxlint -c .oxlintrc.json < /tmp/newapi-main-sync-src-files.txt)
```

If targeted lint reports duplicate imports, missing imports/exports, unused imports/state, hook dependency issues, non-null assertions, nested ternaries, or impossible `ReactNode`/value types in changed files, fix them before reporting success. Do not dismiss these as style-only during a sync; they are common signs of partial conflict resolution.

Also run a frontend merge-residue audit:

```bash
cd web
bun run typecheck 2>&1 | tee /tmp/newapi-main-sync-typecheck.txt
cd ..
rg -n '^(<<<<<<<|=======|>>>>>>>)' web/src
rg -n "Identifier '.+' has already been declared|Duplicate identifier|Cannot find name|Cannot find module|has no exported member" /tmp/newapi-main-sync-typecheck.txt
```

If `bun run typecheck` reports duplicate identifiers, missing names/modules/exports, or impossible types in changed files, treat the sync as not complete even when `bun run build` passes. Inspect changed files for these merge-residue patterns:

- duplicated import blocks or two versions of the same import list;
- duplicated handlers after a hook or action refactor, such as old and new `handle*` functions coexisting;
- old and new component implementations left in the same file after adopting an upstream rewrite;
- stale query data, state, or helper names left after a refactor, such as `modelsData`/`groupsData` style variables no longer used by the active UI;
- JSX blocks spliced into the wrong drawer/form/table section, especially model selectors, system prompts, filters, and action footers;
- partial adoption of a component path rename, leaving missing imports, wrong named exports, or deleted components still referenced;
- commented-out old UI paired with live unused state/imports.

When a changed frontend file is a route, drawer, dialog, table column file, editor, filter toolbar, or playground-like large module, inspect it manually after automated checks. Confirm imports, hooks, state, handlers, and JSX order form one coherent implementation rather than a concatenation of local and upstream versions.

When large frontend modules, route files, drawers, table columns, or lazily loaded pages changed, perform a dev-server smoke check after `typecheck` and `build` pass:

```bash
cd web
bun run dev
curl -I http://127.0.0.1:3001/
curl -s http://127.0.0.1:3001/static/js/index.js | rg '<changed-symbol-or-component>'
```

For touched routes or lazy chunks, open or request each affected route/module when feasible. In the built main bundle or affected chunk, confirm changed high-risk symbols are single definitions when the bug class is duplication-related, for example handlers, drawers, toolbars, editors, and column factories. Confirm the browser console or dev-server output does not contain syntax errors such as duplicate declarations, missing exports/imports, or failed chunk evaluation. Stop the dev server after the smoke check.

If a check is too expensive or cannot run because dependencies or services are missing, say that plainly and include the next best manual check.

### 9. Report

Reply in Chinese with:

- the fetched `newapi/main` short commit;
- whether the merge was fast-forward, merge commit, clean merge, conflict-resolved merge, or blocked by ambiguous conflicts;
- conflicts automatically resolved, if any, and whether they were unrelated-compatible or local-main-preferred;
- the local behavior ledger and preservation result for each semantic-conflict candidate;
- post-merge repair commits or working-tree edits, with each behavioral hunk justified;
- escalation-worthy conflicts and options, if any;
- backend main updates, if any;
- frontend main updates, if any;
- other config/deploy/doc updates, if any;
- recommended test focus based on the changed areas;
- verification commands and results;
- current `git status --short`.
