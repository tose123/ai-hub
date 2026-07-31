---
name: newapi-main-sync
description: Fetch upstream newapi/main, merge it into local main, preserve versioned local behavior contracts across textual and semantic conflicts, pause for high-confidence security conflicts, audit post-merge repairs for regressions, summarize updates, and recommend key test areas.
---

# Newapi Main Sync

## Purpose

Update the local `main` branch with the latest code from the `newapi` remote's `main` branch. Use this when the user asks to pull, sync, update, or merge `newapi/main` into local `main`.

The versioned feature ledger at `.agents/skills/newapi-main-sync/local-feature-ledger.yaml` is the authority for local behavior that must survive synchronization. A feature is affected only when the upstream diff touches its recorded paths/symbols/callers or changes the recorded observable behavior; sharing a file alone is not enough. For an affected protected feature, preserve local behavior by default and only carry over upstream pieces that are clearly compatible. Apply this rule to semantic conflicts and post-merge repairs even when Git reports no textual conflict.

Do not use a broad "ours" merge strategy. It can silently discard useful upstream work outside a protected feature. Resolve protected behavior locally, but evaluate unlisted behavior on its own merits.

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

If a merge, rebase, cherry-pick, or revert is already in progress, stop and report the in-progress operation before doing anything else. The only exception is an explicit user decision in this same task that resolves the exact pending security/semantic question; resume that existing operation without fetching again.

If the working tree or index has uncommitted changes:

- Do not discard, stash, commit, or overwrite them unless the user explicitly asks.
- Stop before fetching or switching branches. Ask the user to commit, stash, or cancel; an uncommitted candidate-ledger draft is the sole exception and must stay unmerged until the requested confirmation arrives.
- Never begin an upstream merge with a non-empty index. Existing staged work could otherwise be included in the merge commit.

Never run destructive commands such as `git reset --hard`, `git checkout -- <path>`, branch deletion, or force push as part of this skill.

### 2. Fetch upstream

After preflight confirms a clean tree, switch to `main` before establishing the local baseline:

```bash
git switch main
git diff --quiet
git diff --cached --quiet
```

Fetch the latest upstream `main`:

```bash
LOCAL_BEFORE=$(git rev-parse HEAD)
git rev-parse --short "$LOCAL_BEFORE"
git fetch newapi main
UPSTREAM_HEAD=$(git rev-parse newapi/main)
git rev-parse --short "$UPSTREAM_HEAD"
git log --oneline --left-right --cherry-pick main...newapi/main
```

Record the full pre-merge local `HEAD` as `LOCAL_BEFORE` and the full fetched upstream commit as `UPSTREAM_HEAD` in working notes. Shell variables may not persist between tool calls; use the recorded literal hashes later or reassign the variables in the same shell invocation. Summarize whether local `main` is behind, ahead, diverged, or already up to date. If `UPSTREAM_HEAD` is already an ancestor of `LOCAL_BEFORE`, mark the upstream result as `no-op`; still finish any required ledger bootstrap, but skip the merge and never create an empty merge commit.

### 3. Build and confirm the persistent local feature ledger

Before merging, read `.agents/skills/newapi-main-sync/local-feature-ledger.yaml`. It is versioned project data, not temporary working notes. Validate that it has a supported `schema_version`, the `newapi/main` upstream target, and a `features` array. If it is missing or malformed, stop and ask for repair; do not silently invent an incomplete replacement.

Identify behavior that exists only on local `main`:

```bash
MERGE_BASE=$(git merge-base "$LOCAL_BEFORE" "$UPSTREAM_HEAD")
git rev-parse --short "$MERGE_BASE"
git log --oneline --no-merges "$MERGE_BASE".."$LOCAL_BEFORE"
git diff --name-status "$MERGE_BASE".."$LOCAL_BEFORE"
git diff --name-status "$MERGE_BASE".."$UPSTREAM_HEAD"
```

Save the merge base as `MERGE_BASE`. Compare local-only functional commits with `source_commits` in every reviewed ledger entry (`approved` or `excluded`) and any already-pending candidate, so a rejected candidate does not return on every sync. Exclude the ledger and this skill's own paths. For every unrecorded functional commit, generate a candidate with `status: pending_confirmation`, a stable `id`, category (`backend`, `ui`, or `ops`), exact paths, important symbols/callers, observable behavior, and focused verification. Documentation-only, formatting-only, lockfile-only, and mechanical commits are not candidates unless they change a recorded behavior. Only entries with `status: approved` protect behavior during a merge; an `excluded` entry must contain the user's `review_reason`.

For every candidate, inspect its patch and relevant tests before classifying it. Never create a feature entry from its commit subject or a top-level directory alone. Group related candidates only when they form one observable product contract; preserve source commit/path evidence inside the grouped entry.

When `bootstrap.state` is `required`, use `bootstrap.source_merge_base` as `LOCAL_FEATURE_BASE` after verifying it is an ancestor of `LOCAL_BEFORE`; do not replace it with the current merge base. If it is not an ancestor, stop and report the invalid bootstrap anchor. Derive candidates from `LOCAL_FEATURE_BASE..LOCAL_BEFORE`, update the draft in the ledger, and present one compact approval table grouped by category. Use the default of protecting all functional groups. The user may reply `确认默认项` to approve all, or name only groups/IDs to exclude or amend. Mark excluded items as `status: excluded` with a `review_reason`; do not delete their source evidence. Do not merge, stage, or commit while any candidate is unconfirmed. After approval, persist reviewed entries, set `bootstrap.state: complete`, and make a separate focused commit for the ledger before restarting the sync from preflight. This one-time stop prevents an unreviewed historical fork from being silently overwritten even if the upstream merge base moves before bootstrap finishes.

On later syncs, apply the same candidate process for new local functional commits that are not covered by an approved ledger entry. Keep the confirmation burden low: omit mechanical candidates, group coherent behavior, state the default, and accept `确认默认项` as a single approval. Never mark a candidate approved merely because it is adjacent to an approved feature.

For each approved feature, calculate upstream impact using `git diff --find-renames "$MERGE_BASE".."$UPSTREAM_HEAD"`, paths, symbols, callers, public API contracts, configuration keys, and tests. Classify it as `none`, `adjacent`, `direct`, or `security-direct`; record the evidence in working notes. `adjacent` requires review but does not permit overwriting the feature. Treat paths changed on both sides as semantic-conflict candidates even if Git would merge them cleanly. For each candidate, inspect the local commits and representative patches:

```bash
git log --oneline "$MERGE_BASE".."$LOCAL_BEFORE" -- <path>
git log -p "$MERGE_BASE".."$LOCAL_BEFORE" -- <path>
git diff "$MERGE_BASE".."$UPSTREAM_HEAD" -- <path>
```

Include behavior affected indirectly by renames, moved packages, shared helpers, tests, defaults, configuration parsing, and call-site contracts. When feasible, run focused tests for semantic-conflict candidates before merging and record pre-existing failures; do not later misclassify a baseline failure as merge fallout.

#### Security-overlap gate

Do not label an upstream change "strongly recommended security update" from a commit subject, a `security` keyword, or a dependency version bump alone. It needs one of: a CVE/GHSA or upstream security advisory; an upstream maintainer's explicit security notice; or patch/test evidence of a concrete high-risk remote vulnerability such as authentication bypass, privilege escalation, injection, secret exposure, or remote code execution. Obtain and record the advisory/commit evidence before escalating.

If such a change directly affects an approved feature, pause before creating a merge commit. Present a compact decision card containing the affected feature, vulnerability evidence, upstream remediation, local behavior at risk, focused verification, and these choices:

1. Recommended: port an equivalent mitigation while preserving the local behavior contract.
2. Adopt the upstream behavioral change and retire/amend the ledger entry.
3. Defer the upstream change and stop the sync without a merge commit.

Do not choose any of these on the user's behalf. Group choices only when the same vulnerability and remediation affect multiple features, and accept `按推荐方案处理全部安全项` as a single explicit approval. A security update not overlapping an approved feature should be integrated normally and reported with its evidence.

### 4. Merge upstream

Start a reviewable merge without a global conflict preference:

```bash
git merge --no-commit --no-ff newapi/main
```

Run this command only when the upstream result is not `no-op`.
For a `no-op`, skip sections 4-6 and proceed directly to section 7; do not define `MERGED_HEAD` or create a merge commit.

Whether the merge is clean or conflict-resolved, keep it uncommitted until the preliminary feature-preservation and security-overlap gates pass. A clean merge does not prove that local behavior survived. For every impacted approved feature, inspect the staged merge result against both parents and run its focused verification when feasible:

```bash
git diff --cached --find-renames "$LOCAL_BEFORE" -- <local-feature-paths>
git diff --cached --find-renames "$UPSTREAM_HEAD" -- <local-feature-paths>
```

If the staged result weakens a recorded behavior, repair it before committing. If the safe repair is ambiguous or security-sensitive, pause for the decision gate instead. After the preliminary gate passes, commit the merge, save `MERGED_HEAD`, then continue to the full preservation audit:

```bash
git commit --no-edit
MERGED_HEAD=$(git rev-parse HEAD)
git rev-parse --short "$MERGED_HEAD"
```

### 5. Conflict handling

When conflicts occur, first list them:

```bash
git diff --name-only --diff-filter=U
rg -n '^(<<<<<<<|=======|>>>>>>>)' .
```

Inspect each conflicted file and classify the conflict before editing.

For an approved feature with `direct` impact, keep the recorded local behavior. Treat the local implementation as the behavioral baseline and selectively re-apply only upstream edits that are obviously safe, such as comments, imports, tests, translations, formatting, or adjacent helper code that does not change runtime behavior. For unlisted behavior, inspect both contracts; do not apply an implicit local preference merely because the hunk conflicts.

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

If all conflicts are safely resolved, stage only the resolved files, perform the preliminary ledger gate above, then complete the merge. Do not stage unrelated user changes.

```bash
git add <resolved-files>
git commit --no-edit
MERGED_HEAD=$(git rev-parse HEAD)
git rev-parse --short "$MERGED_HEAD"
```

### 6. Ledger preservation gate and post-sync review

After a successful merge, inspect what changed between `LOCAL_BEFORE` and the merged `HEAD` before reporting:

```bash
git diff --stat "$LOCAL_BEFORE"..HEAD
git diff --name-status "$LOCAL_BEFORE"..HEAD
git log --oneline --no-merges "$LOCAL_BEFORE"..HEAD
```

#### Local behavior preservation gate

Verify every approved feature with `adjacent`, `direct`, or `security-direct` impact against the merged result. For renamed or refactored code, verify the observable behavior rather than requiring identical source text. A `none` impact still needs its recorded paths checked for accidental merge fallout.

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

If any approved ledger entry is missing, weakened, or cannot coexist safely with a required upstream contract, stop and report the conflict with concrete options. Passing tests, lint, typecheck, or build does not waive this gate. Do not change or remove a ledger entry merely to make the gate pass; an approved security-overlap decision or explicit user approval is required.

After a user-approved feature is intentionally introduced, retired, or materially changed, update the ledger in a focused follow-up commit. Do not bundle ledger administration into the upstream merge commit, and do not create an entry for this skill or its ledger files themselves.

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

### 7. Verification

Run focused checks based on the files changed by the merge. Do not report the sync as complete until the required checks are clean or a concrete blocker is reported:

For a `no-op`, run only ledger validation and any checks needed for a separately approved ledger change; do not run unrelated backend or frontend suites.

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

### 8. Report

Reply in Chinese with:

- the fetched `newapi/main` short commit;
- whether the result was no-op, a clean no-ff merge commit, a conflict-resolved merge commit, or blocked by ambiguous conflicts;
- conflicts automatically resolved, if any, and whether they were unrelated-compatible or local-main-preferred;
- the local behavior ledger and preservation result for each semantic-conflict candidate;
- newly generated ledger candidates, their group-confirmation result, and any focused ledger commit;
- security-overlap evidence, user decision, and whether equivalent mitigation preserved local behavior;
- post-merge repair commits or working-tree edits, with each behavioral hunk justified;
- escalation-worthy conflicts and options, if any;
- backend main updates, if any;
- frontend main updates, if any;
- other config/deploy/doc updates, if any;
- recommended test focus based on the changed areas;
- verification commands and results;
- current `git status --short`.
