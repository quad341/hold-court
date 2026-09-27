# Release gate: hc-1qi

**Verdict:** **PASS**

- **Deploy bead:** hc-1qi
- **Build bead:** hc-o5t (operator request, 5-point exit_contract, TDD red/green)
- **Review bead:** hc-7l4 (verdict: pass)
- **Reviewed / deploy commit:** `767e1a164ea0857331935047320c364b50e5e98b`
- **Deploy branch:** `deploy/hc-1qi-gate`, cut from the reviewed commit directly (not from `builder/hc-o5t`)
- **Base:** `origin/main` @ `f84a5609cc65c8bf9ecb2381d217f1460dffe3f2` — this is also the exact merge-base, i.e. a pure fast-forward, zero divergence
- **Diff:** 10 files, +360/-4 (`git diff --stat origin/main...HEAD`, re-verified fresh at gate time)
  `DESIGN.md, adapters/mpr/export.py, adapters/mpr/test_export.py, internal/feed/feed.go, internal/feed/feed_test.go, internal/ruling/ruling.go, internal/ruling/ruling_test.go, internal/server/server.go, internal/server/server_test.go, internal/server/static/app.js`
- **Commits in range:** `da7037a` (test, red) → `767e1a1` (feat, green), both `(refs hc-o5t)`
- **Ancestry/citation scope:** `hc-1qi`, `hc-o5t` — hc-o5t is hc-1qi's own `build_bead`; hc-7l4 is `DISCOVERED FROM` hc-o5t. Confirmed legitimate sibling, not scope creep.

All evidence below was independently reproduced against the current source and a fresh command run on this branch — not copied from the reviewer's notes.

## Criteria

**1. Review PASS present — PASS**
hc-7l4 closed `reason=pass`, `verdict=pass`, naming this exact commit (`deploy_commit: 767e1a164ea0857331935047320c364b50e5e98b`).

**2. Acceptance criteria met — PASS**
Checked hc-o5t's exit_contract directly against current source, not just against the review notes:
- `adapters/mpr/export.py:201` `jev_would_pick()` reads `jev-split.json` via the existing read-only `optional_json` helper; populates `jev_would` only when `status == "determined"`. `test_adapter_never_writes_jev_split_json` (`adapters/mpr/test_export.py`) records the file's exact text and `mtime_ns` before a full `refresh()` and asserts both byte-identical after — read the body myself, it's a real write-guard, not a smoke check.
- `internal/feed/feed.go:38` — `JevWould *JevWould \`json:"jev_would,omitempty"\`` — pointer + omitempty, so feed consumers without the field still parse unchanged.
- `internal/server/static/app.js:248-250` — the literal rendered string is `"jev would (log only — not a recommendation): "`. Read the source directly; the LOG-ONLY label is actually in the shipped markup, not merely asserted.
- `TestJevWould_NeverAffectsOrderingFoldersOrDefaultFolder` (`internal/server/server_test.go`) — read the full body: builds matched with/without-jev_would fixtures and asserts identical hold order, identical hold state, `reflect.DeepEqual` on Folders, and identical default-folder selection across both. A substantive differential test, not vacuous.
- `internal/ruling/ruling.go:64` (`Ruling.JevWould`) snapshots from `internal/server/server.go:560-561` (`JevWould: view.JevWould`) — sourced from the server-computed view, confirmed not settable from client input.
- `DESIGN.md:167-208` documents the new optional `jev_would` field's shape and semantics.
- TDD order honored: `da7037a` (red) is the direct parent of `767e1a1` (green).

Carried-over, non-blocking gap (reviewer flagged it; re-confirmed here rather than re-hidden): this repo's `make check`/CI has no browser/JS test lane, so the label text above was verified by direct code read, not by an automated JS test. Not treated as a coverage failure — verified true, just not test-enforced.

**3. Tests pass — PASS**
- **3a (non-diff-owned failures):** n/a — 0 FAIL anywhere in either suite this run; nothing to attribute.
- **Go:**
  `test_cmd: make test-race` (→ `go test -race ./...`)
  `test_cmd_scope: full-suite`
  `test_counts: 59 PASS, 0 FAIL, 0 SKIP` (top-level `--- PASS:` lines, independently grepped from the run log; 73 including subtests) across `internal/feed`, `internal/ruling`, `internal/server`, `internal/store` (all `ok`) — `cmd/hold-court` has no test files.
  Re-ran clean via direct redirect (no pipe, unambiguous exit code): `REAL_EXIT_CODE:0`.
  `diff_tests_executed`: every test in `internal/feed/feed_test.go`, `internal/ruling/ruling_test.go`, `internal/server/server_test.go` resolved individually by exact name against the verbose log — all `--- PASS`, none missing, none skipped.
- **Python:**
  `test_cmd: make test-adapters` (→ `python3 -m unittest discover -s adapters/mpr -p 'test_*.py'`)
  `test_cmd_scope: full-suite`
  `test_counts: 26 PASS, 0 FAIL, 0 SKIP`
  `diff_tests_executed`: `adapters/mpr/test_export.py`'s new/changed cases included and passing, incl. `test_adapter_never_writes_jev_split_json` (body read in full, see criterion 2).
- **3b (lint/policy lane):**
  `make lint` (`golangci-lint run`) first attempt returned 7 gosec findings resolving to a deleted path (`/var/tmp/review.L0u9QC/...`) — diagnosed as a **stale golangci-lint cache** left over from another worktree/session sharing this host's cache, not a real defect. Verified directly: every one of the 7 flagged lines already carries an explicit, justified `//nolint:gosec` in current source, and one flagged file (`internal/server/render.go`) isn't even touched by this diff. `golangci-lint cache clean` + re-run: **0 issues**. Also found (separately, lower-stakes) the installed linter was v2.12.0 against the Makefile-pinned `v2.13.2`; ran `make tools` to install the exact pinned version and re-ran clean once more: **0 issues, exit 0**, no remaining version drift.
- **3c (CI-config diff):** `ci_lane_run: n/a (no CI-config change in this diff)` — `git diff --name-only origin/main...HEAD -- .github/workflows/` is empty; confirmed against the actual `.github/workflows/ci.yml` (build → vet → fmt-check → test-race → test-adapters → golangci-lint@v2.13.2, matrix: ubuntu/macos/ubuntu-fedora).

**4. No open HIGH-severity findings — PASS**
hc-7l4 recorded zero blocker/major security findings. Independently re-checked the highest-risk surface myself rather than accepting that at face value: `app.js`'s `innerHTML`-based render of `hold.jev_would` — `category` passes through the real `escapeHTML()` (DOM `textContent`→`innerHTML` round-trip, read the definition); `probability`/`confidence` arrive as real JS numbers via `JSON.parse(text)` (confirmed at the call site), so their string form can't carry HTML metacharacters — safe by construction. One cosmetic, non-security gap remains (reviewer-flagged, re-confirmed, not gate-blocking): an absent `answers.category.choice` on a `"determined"` split renders an empty `<strong></strong>` rather than omitting the block.

**5. Final branch is clean — PASS**
`git status --porcelain` on `deploy/hc-1qi-gate` at `767e1a1`: only `?? .gitkeep` — untracked, pre-existing in this worktree, not a tracked-file modification, not part of this diff, and won't appear in the PR. No uncommitted changes to any tracked file.

**6. Branch diverges cleanly from main — PASS**
`origin/main` tip (`f84a5609`) equals the merge-base with the reviewed commit exactly — a pure fast-forward. No rebase was needed.

**7. Single feature theme — PASS**
All 10 changed files belong to the one jev_would/LOG-ONLY feature (hc-o5t): the Go feed/ruling/server structs and tests, the app.js render line, the Python export/test, and the design doc. No unrelated files. Ancestry/citation-scope check passed citing `hc-1qi, hc-o5t` (confirmed legitimate sibling per header).

## Notes

- This is the first release-gate file written for hold-court; no prior filename/format precedent existed in this repo. Followed `release-gate-criteria.md.tmpl`'s 7 criteria and `test-evidence-integrity.md.tmpl`'s required evidence fields (cmd, scope, counts, diff-tests-by-name, skip/waiver n/a, CI-lane n/a).
- No self-granted waiver was used or needed — there were no diff-owned or non-diff-owned failures of any kind to waive.
