---
name: newapi-main-sync
description: Fetch upstream newapi/main, merge it into local main with local-main precedence on same-feature conflicts, summarize backend/frontend updates after sync, and recommend key test areas.
---

# Newapi Main Sync

## Purpose

Update the local `main` branch with the latest code from the `newapi` remote's `main` branch. Use this when the user asks to pull, sync, update, or merge `newapi/main` into local `main`.

Conflict precedence rule: when `newapi/main` and local `main` change the same functionality, local `main` is the source of truth. Preserve local `main` behavior by default, and only carry over upstream pieces that are clearly compatible and do not alter the local behavior contract.

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
git rev-parse HEAD
git fetch newapi main
git rev-parse --short newapi/main
git log --oneline --left-right --cherry-pick main...newapi/main
```

Save the pre-merge local `HEAD` as `LOCAL_BEFORE` and the fetched upstream commit as `UPSTREAM_HEAD`. Summarize whether local `main` is behind, ahead, diverged, or already up to date.

### 3. Switch to local main

If the current branch is not `main` and the working tree is clean, switch to `main`:

```bash
git switch main
```

### 4. Merge upstream

Merge upstream into local `main`, preferring local `main` on conflicting hunks:

```bash
git merge --no-edit -X ours newapi/main
```

If the merge completes cleanly, continue to verification and reporting.

### 5. Conflict handling

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

### 6. Post-sync update review

After a successful merge, inspect what changed between `LOCAL_BEFORE` and the merged `HEAD` before reporting:

```bash
git diff --stat "$LOCAL_BEFORE"..HEAD
git diff --name-status "$LOCAL_BEFORE"..HEAD
git log --oneline --no-merges "$LOCAL_BEFORE"..HEAD
```

Summarize main updates by product area, not by raw file list:

- Backend: Go modules and changes under `controller/`, `service/`, `model/`, `relay/`, `middleware/`, `setting/`, `common/`, `dto/`, `constant/`, `i18n/`, `oauth/`, `pkg/`, `router/`.
- Frontend default UI: changes under `web/default/`, especially `src/`, `package.json`, `bun.lock`, `rsbuild.config.*`, Tailwind/CSS, i18n locale files.
- Frontend classic UI: changes under `web/classic/`.
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
- Frontend UI changed: build `web/default`, open affected pages, test create/edit/delete flows, loading/error states, and responsive layout.
- i18n changed: run or inspect i18n sync and test language switching for touched screens.
- Config/deployment changed: test fresh config defaults and upgraded existing config.

### 7. Verification

Run focused checks based on the files changed by the merge. Do not report the sync as complete until the required checks are clean or a concrete blocker is reported:

- Go backend changes: `go test ./...`
- Default frontend changes: from `web/default/`, use Bun. Always run both `bun run typecheck` and `bun run build`; a successful Rsbuild build is not sufficient because it can miss TypeScript duplicate declarations, missing imports, and invalid lazy route chunks. `bun run typecheck` must be clean, not merely grepped for known errors.
- i18n-only frontend changes: from `web/default/`, run `bun run i18n:sync` when keys or locale files changed

For any merge that changes `web/default/src/`, also run targeted lint on changed source files before reporting success:

```bash
{
  git diff --name-only "$LOCAL_BEFORE"..HEAD -- 'web/default/src/**/*.ts' 'web/default/src/**/*.tsx'
  git diff --name-only -- 'web/default/src/**/*.ts' 'web/default/src/**/*.tsx'
} | sort -u | sed 's#^web/default/##' > /tmp/newapi-main-sync-src-files.txt
test ! -s /tmp/newapi-main-sync-src-files.txt || (cd web/default && xargs bunx oxlint -c .oxlintrc.json < /tmp/newapi-main-sync-src-files.txt)
```

If targeted lint reports duplicate imports, missing imports/exports, unused imports/state, hook dependency issues, non-null assertions, nested ternaries, or impossible `ReactNode`/value types in changed files, fix them before reporting success. Do not dismiss these as style-only during a sync; they are common signs of partial conflict resolution.

Also run a frontend merge-residue audit:

```bash
cd web/default
bun run typecheck 2>&1 | tee /tmp/newapi-main-sync-typecheck.txt
cd ../..
rg -n '^(<<<<<<<|=======|>>>>>>>)' web/default/src
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
bun run dev
curl -I http://127.0.0.1:3001/
curl -s http://127.0.0.1:3001/static/js/index.js | rg '<changed-symbol-or-component>'
```

For touched routes or lazy chunks, open or request each affected route/module when feasible. In the built main bundle or affected chunk, confirm changed high-risk symbols are single definitions when the bug class is duplication-related, for example handlers, drawers, toolbars, editors, and column factories. Confirm the browser console or dev-server output does not contain syntax errors such as duplicate declarations, missing exports/imports, or failed chunk evaluation. Stop the dev server after the smoke check.

If a check is too expensive or cannot run because dependencies or services are missing, say that plainly and include the next best manual check.

### 8. Report

Reply in Chinese with:

- the fetched `newapi/main` short commit;
- whether the merge was fast-forward, merge commit, clean merge, conflict-resolved merge, or blocked by ambiguous conflicts;
- conflicts automatically resolved, if any, and whether they were unrelated-compatible or local-main-preferred;
- escalation-worthy conflicts and options, if any;
- backend main updates, if any;
- frontend main updates, if any;
- other config/deploy/doc updates, if any;
- recommended test focus based on the changed areas;
- verification commands and results;
- current `git status --short`.
