# Release gate: hc-p5q (re-gate)

**Verdict:** **PASS**

- **Deploy bead:** hc-p5q
- **Re-gate tracking bead:** hc-wrw — main moved (PR #15/hc-1qi merged as `a00787eed6`) after this gate's first PASS on `6f98a5fe`, putting PR #16 into CONFLICTING; the rebase-resolution round then surfaced and fixed a stored-XSS finding, so this is a full fresh gate on a new commit, not a rubber-stamp of the old one.
- **Build chain:** hc-8fo (original feature, curator groupings/tags with one proposed ruling per group, decided in one action; TDD red/green) → hc-hqi (rebase PR #16 onto post-#15 main, union both features' `server.go` hunks; TDD red/green, adapted for a conflict-resolution bead per its own notes) → hc-jgb round 2 (TDD fix for the round-1 XSS blocker)
- **Review chain:** hc-dsy (original feature review, verdict: pass) → hc-jgb round 1 (verdict: request-changes — stored-XSS blocker) → hc-ov2 (round 2, verdict: pass)
- **Reviewed / deploy commit:** `d7841b587095c984f81438e3027273d753046ef6`
- **Deploy branch:** `deploy/hc-p5q-gate`, force-moved from the stale `5f321193addc4bfb8be0124c17b0c44827f985e1` (the pre-conflict, pre-fix commit this gate previously PASSed) directly to `d7841b587095c984f81438e3027273d753046ef6` — the exact commit hc-ov2 reviewed
- **Base:** `origin/main` @ `a00787eed6a3f7ba6a7e21e16710a07f81ade27b` — re-fetched and re-confirmed at gate time; also the exact merge-base with the reviewed commit, i.e. a pure fast-forward, zero divergence
- **Diff:** 9 files, +647/-8 (`git diff --stat origin/main...HEAD`, re-verified fresh at gate time)
  `cmd/hold-court/main.go, internal/group/group.go, internal/group/group_test.go, internal/server/group_test.go, internal/server/server.go, internal/server/static/app.js, internal/server/static/style.css, internal/server/static_escape_test.go, release-gates/hc-p5q-gate.md`
- **Commits in range:** `80a2ce1` (test, red, hc-8fo) → `05a9335` (feat, green, hc-8fo) → `58b9994` (chore, this gate's own prior PASS record for `6f98a5f`/hc-p5q) → `2f00b84` (test, red, hc-jgb round 2) → `d7841b5` (feat, green, hc-jgb round 2 — XSS fix)
- **Ancestry/citation scope:** `hc-p5q, hc-wrw, hc-8fo, hc-hqi, hc-jgb` — hc-wrw is hc-p5q's own re-gate tracking bead; hc-hqi and hc-jgb are both `DISCOVERED FROM`/mail-chained off hc-wrw; hc-ov2's `deploy_commit`/`deploy_bead` metadata cross-reference this exact bead and commit. Confirmed legitimate chain, not scope creep.

All evidence below was independently reproduced against the current source and fresh command runs on this branch at the reviewed SHA — not copied from reviewer/builder notes, though it corroborates them throughout.

## Criteria

**1. Review PASS present — PASS**
hc-ov2 closed `verdict: pass` directly against this exact commit (`deploy_commit: d7841b587095c984f81438e3027273d753046ef6`, `deploy_bead: hc-fz21` — a duplicate deploy bead auto-filed off hc-ov2's pass and consolidated into this one, `hc-wrw`, to avoid a competing PR for the same commit; confirmed via `gh pr view 16` that only one PR/branch pair exists for this SHA). This is a direct reviewer PASS on the exact deploy SHA, not the weaker `review_carryover` patch-id path.

**2. Acceptance criteria met — PASS**
Two exit_contracts apply here, both independently checked against current source:
- **hc-8fo's original feature contract** (grouping/tags): re-confirmed unchanged by the rebase+fix — `internal/group/group.go`'s `Proposal{Group, Action, Note}`/`Read`/`Key`, `server.go`'s `GroupsFile` config, `holdViews`/`buildHoldView` group-field wiring, `buildFolders`'s `group:` folder section, `filterByFolder`'s `group:` prefix case, the `--groups` CLI flag, and `app.js`'s banner/accept-group/reject-group flow are all present and match hc-dsy's original review, which walked every one of these sites in full against `6f98a5fe`. Diffing `6f98a5fe..d7841b5` on these specific regions shows no behavioral change beyond the union conflict resolution and the escape fix below.
- **hc-hqi's conflict-resolution contract**: `server.go`'s two real conflicts (holdJSON struct field list; `buildHoldView`'s return literal) are resolved as the union — read both sites directly: `holdJSON` carries both `JevWould` (PR #15) and `Group`/`ProposedAction`/`ProposedNote` (PR #16) fields, and `buildHoldView`'s literal sets all of them. `internal/server/group_test.go` (172 lines, PR #16's tests replayed) and PR #15's own JevWould tests both live in `internal/server` and both pass together (see criterion 3) — direct proof the union didn't silently drop either feature.
- **hc-jgb round-2 contract** (XSS fix): `escapeHTML()` in `static/app.js` now chains `.replace(/"/g,'&quot;').replace(/'/g,'&#39;')` after the existing DOM-based `&`/`<`/`>` encoding — read the function directly at its current location. `internal/server/static_escape_test.go` (new, 118 lines) extracts this exact function's source out of the shipped `app.js` via regex and executes it under Node against both the two vulnerable attribute call sites (`data-folder-id`, `data-hold-id`) and a plain-text non-adversarial case, confirming (a) quotes are now encoded and (b) ordinary group/hold names render unchanged — matching hc-jgb's round-2 exit_contract ("no visible behavior change for non-adversarial input") exactly.
- TDD order honored throughout: `80a2ce1`→`05a9335` (hc-8fo), `2f00b84`→`d7841b5` (hc-jgb round 2); hc-hqi's union work has no separate red/green pair by its own design (conflict-resolution bead, not new-feature work — verified empirically instead via a scratch-worktree RED check on the replayed test-only commit, per its notes).

Same carried-over, non-blocking gap as the original gate: no browser/JS test lane in CI, so the banner UI's non-security behavior is verified by direct code read, not automated JS test. The one JS behavior that *is* security-relevant (`escapeHTML`) now has a real executable test (`static_escape_test.go`), closing exactly the gap that mattered.

**3. Tests pass — PASS**
- **3a (non-diff-owned failures):** n/a — 0 FAIL anywhere in either suite this run; nothing to attribute.
- **Go:**
  `test_cmd: go test -race -v -count=1 ./...` (the `-count=1` is deliberate: a first run without it returned every package `(cached)` from a prior run sharing this host's GOCACHE — not real evidence the suite executed against this commit — so it was re-run and only this re-run is cited as evidence)
  `test_cmd_scope: full-suite`
  `test_counts: 68 top-level PASS + 14 subtest PASS = 82 total, 0 FAIL, 0 SKIP` (top-level via `grep -cE '^--- PASS'`, subtests via `grep -cE '^\s+--- PASS'`, reconciled explicitly to avoid conflating the two — 68 matches hc-ov2's own recorded count exactly, the 14 are table-driven subtest cases the aggregate figure doesn't separately name)
  All 5 packages ran with real, non-cached elapsed times (confirmed `grep -c cached` → 0): `internal/feed` 1.016s, `internal/group` 1.012s, `internal/ruling` 1.022s, `internal/server` 3.460s, `internal/store` 1.243s.
  `diff_tests_executed`: all 9 diff-owned test functions resolved individually by exact name against the verbose log, each `--- PASS`: `TestRead_ValidFileParses`, `TestRead_MissingFileReturnsEmptyMapNilError`, `TestRead_MalformedJSONReturnsError`, `TestKey_RoundTrips` (`internal/group/group_test.go`); `TestHandleHolds_SetsGroupFieldsOnlyForGroupedHold`, `TestHandleHolds_NoGroupsFileLeavesGroupFieldsUnset`, `TestBuildFolders_GroupFolderHasCorrectCountAndLabel`, `TestFilterByFolder_GroupPrefixReturnsOnlyMatchingHolds` (`internal/server/group_test.go`); `TestEscapeHTML_SafeForDoubleQuotedAttribute` (`internal/server/static_escape_test.go`). Read every one of these bodies in full — all substantive.
- **Python:**
  `test_cmd: make test-adapters` (→ `python3 -m unittest discover -s adapters/mpr -p 'test_*.py'`)
  `test_cmd_scope: full-suite`
  `test_counts: 26 PASS, 0 FAIL, 0 SKIP`, exit 0.
  `diff_tests_executed`: n/a — this diff has no Python changes; suite runs in full regardless and stays green.
- **3b (lint/policy lane):**
  `golangci-lint version`: `v2.13.2` — matches the Makefile pin exactly, confirmed by two independent methods: (1) `run-pinned-lint.sh -- make lint` → `0 issues`, exit 0; (2) direct `which golangci-lint` / `golangci-lint version` against the binary that actually resolves on PATH, because hold-court's Makefile (`GOLANGCI_LINT ?= golangci-lint`) resolves the linter via bare PATH rather than through `run-pinned-lint.sh`'s isolated per-pin GOPATH, so its own `GLT_RECORD` self-check reported `resolved: null, mismatch: false` (the "nothing installed at the isolated path to compare" branch, not a real confirmation) — filed as `hc-y15c` (P3, informational, cross-referenced to `gm-gd2k1a`/`hc-kht`) since this is a real gap in the wrapper for this Makefile shape, not a blocker: the manual check confirms the binary that actually ran is the correct pinned version.
  `make lint`: **0 issues**.
- **3c (CI-config diff):** `ci_lane_run: n/a (no CI-config change in this diff)` — `git diff --name-only origin/main...HEAD -- .github/workflows/` is empty; `.github/workflows/ci.yml` read in full and confirmed its steps (`make build`, `make vet`, `make fmt-check`, `make test-race`, `make test-adapters`, `golangci-lint-action@v9` pinned `v2.13.2`) match this gate's own command set exactly.

**4. No open HIGH-severity findings — PASS**
The one HIGH/blocker finding in this entire lineage — hc-jgb round 1's stored-XSS finding (`static/app.js` `escapeHTML()` not encoding quote characters, exploitable via curator-authored group names breaking out of the `data-folder-id` attribute; no CSP in repo) — is confirmed FIXED: read the current `escapeHTML()` source directly (quote-encoding chain present) and independently re-ran `TestEscapeHTML_SafeForDoubleQuotedAttribute`, which exercises the real shipped function under Node against the exact vulnerable call sites (PASS). hc-ov2's round-2 review independently confirms the same fix with its own security_findings walk. hc-dsy's original review (base feature, `6f98a5fe`) recorded a full OWASP Top 10 walk with zero findings — the vulnerability didn't exist at that point in the diff; it was introduced by the later conflict-resolution rework (hc-hqi) and caught by hc-jgb's round-1 pass, which is exactly why a fresh review round was required rather than a carryover.
Two open beads exist in this lineage but are both tooling/process bugs against the *gate machinery itself*, not review findings against the shipped code: `hc-kht` (P2, stale-sibling-worktree lint cwd drift) and this turn's own `hc-y15c` (P3, lint-isolation PATH gap). Neither is a security or correctness finding on the reviewed diff — out of scope for this criterion by definition (criterion 4 counts unresolved review findings on the diff, not open tooling bugs about the process that gates it).

**5. Final branch is clean — PASS**
`git status --porcelain` on `deploy/hc-p5q-gate` at `d7841b5`: only `?? .gitkeep` — untracked, pre-existing in this worktree (noted on every prior gate at this same location), not a tracked-file modification, not part of this diff.

**6. Branch diverges cleanly from main — PASS**
Re-fetched `origin/main` immediately before writing this record: tip `a00787eed6a3f7ba6a7e21e16710a07f81ade27b`, confirmed via `git merge-base --is-ancestor origin/main d7841b587095c984f81438e3027273d753046ef6` to be a strict ancestor of the reviewed commit — a pure fast-forward, zero divergence, no rebase needed for this specific re-gate (the rebase onto post-#15 main already happened as hc-hqi's own build step).

**7. Single feature theme — PASS**
Every file in the diff exists to safely ship the one curator-groupings/tags feature (hc-8fo): the feature itself (`internal/group/*`, server wiring, UI), the conflict-resolution union required to land it after PR #15 merged first (hc-hqi's `server.go` changes — mandatory integration plumbing, not a second feature), and the XSS fix required because the feature's own new attribute-rendering surface introduced the vulnerability (hc-jgb's `static_escape_test.go` + `app.js` fix — a required security fix for this same feature, not an unrelated concern). `release-gates/hc-p5q-gate.md` is this gate's own process artifact, not product code. No unrelated files. Ancestry/citation-scope check passed citing `hc-p5q, hc-wrw, hc-8fo, hc-hqi, hc-jgb` (confirmed legitimate chain per header).

## Notes

- **Test-caching pitfall (this gate):** the first `go test -race -v ./...` run returned all 5 packages `(cached)` — a prior run's result, not genuine execution against this commit. Caught before being relied on, re-run with `-count=1` for real evidence. Recorded here as a reminder that `(cached)` must always be checked for on this host.
- **Lint-isolation gap (this gate):** see criterion 3b above and `hc-y15c` — `run-pinned-lint.sh`'s per-pin GOPATH isolation is inert for a Makefile that resolves `golangci-lint` via bare PATH (hold-court's shape). Not a blocker here (the actual binary was independently confirmed correct), but worth fixing at the tooling level so a future drifted-PATH-binary case doesn't silently pass as it would have if the ambient binary hadn't happened to already be pinned correctly.
- Cross-reference (informational, carried from the original gate): `hc-kht` (stale-sibling-worktree lint cwd drift) and the fleet-level `gm-17g0ym` tracker remain open; not touched by this re-gate.
- No self-granted waiver was used or needed — there were no diff-owned or non-diff-owned failures of any kind to waive.
