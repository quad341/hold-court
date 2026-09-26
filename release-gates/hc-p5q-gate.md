# Release gate: hc-p5q

**Verdict:** **PASS**

- **Deploy bead:** hc-p5q
- **Build bead:** hc-8fo (feature, curator groupings/tags with one proposed ruling per group, decided in one action; TDD red/green)
- **Review bead:** hc-dsy (verdict: pass)
- **Reviewed / deploy commit:** `6f98a5fe46a8072f13453ac6ace08e843ddb3aa6`
- **Deploy branch:** `deploy/hc-p5q-gate`, cut from the reviewed commit directly (not from `builder/hc-8fo`, which is provenance only)
- **Base:** `origin/main` @ `f84a5609cc65c8bf9ecb2381d217f1460dffe3f2` — this is also the exact merge-base, i.e. a pure fast-forward, zero divergence
- **Diff:** 7 files, +452/-6 (`git diff --stat origin/main...HEAD`, re-verified fresh at gate time; matches hc-8fo's own recorded `tdd_diff_files/insertions/deletions`)
  `cmd/hold-court/main.go, internal/group/group.go, internal/group/group_test.go, internal/server/group_test.go, internal/server/server.go, internal/server/static/app.js, internal/server/static/style.css`
- **Commits in range:** `6e4dcff` (test, red) → `6f98a5f` (feat, green), both `(refs hc-8fo)`
- **Ancestry/citation scope:** `hc-p5q`, `hc-8fo` — hc-8fo is hc-p5q's own `build_bead`; hc-dsy's `deploy_bead`/`deploy_commit` metadata cross-reference this exact bead and commit. Confirmed legitimate sibling, not scope creep.

All evidence below was independently reproduced against the current source and a fresh command run on this branch — not copied from the reviewer's or builder's notes.

## Criteria

**1. Review PASS present — PASS**
hc-dsy closed `verdict: pass`, naming this exact commit (`deploy_commit: 6f98a5fe46a8072f13453ac6ace08e843ddb3aa6`, `deploy_bead: hc-p5q`).

**2. Acceptance criteria met — PASS**
Checked hc-8fo's exit_contract directly against current source, not just against the review or build notes:
- `internal/group/group.go` — new package. `Proposal{Group, Action, Note}` (`Action` deliberately a plain string, not `ruling.Action`, with a doc comment explaining why: an entry with an unrecognized action still carries Group/Note for display, and it's on the caller to exclude it from bulk actions). `Read(path)` returns an empty map and nil error for a missing file (same posture as unset `OnRuling`), a wrapped error for malformed JSON, and never writes. `Key(repo, pr)` builds the `"repo#pr"` index string. All read myself in full — real implementations, not stubs.
- `internal/server/server.go:41-45` — `Config.GroupsFile string`, documented as optional/inert-when-empty.
- `internal/server/server.go:185-190` — `holdViews()` calls `group.Read` exactly once per call (outside the per-hold loop), confirmed by reading the function body directly, not just trusting the exit_contract's claim.
- `internal/server/server.go:283,357` — `buildHoldView` takes the proposals map and sets `Group`/`ProposedAction`/`ProposedNote` from `proposals[group.Key(h.Repo, h.PR)]` — a zero-value `Proposal{}` for an unmatched hold, which is exactly empty-string fields, so ungrouped holds are provably unaffected (also directly asserted by `TestHandleHolds_SetsGroupFieldsOnlyForGroupedHold`, body read in full).
- `internal/server/server.go:407-457` (`buildFolders`) — gains a `groupCounts` map and, when non-empty, a `"----"` divider heading followed by one sorted `group:<name>` folder per group — read the full function body, matches exit_contract exactly.
- `internal/server/server.go:465-487` (`filterByFolder`) — gains a `group:` prefix case parallel to the existing `class:` case, read directly.
- `cmd/hold-court/main.go` — new `--groups` flag (default `groups.json`) and matching `fileConfig.Groups`/TOML wiring, same shape as the existing `--feed`/`--rulings` flags; threaded into `server.Config.GroupsFile`.
- `internal/server/static/app.js` — `VALID_ACTIONS` allowlist (`proceed/changes/close/discuss`); `groupEligibleHolds` (folder-state `inbox`, valid action, not already pending) backs both the banner's live count and `acceptGroup`'s staging set; `renderGroupBanner` (shows the proposing group's action/note, `Accept group (`n`)`/`Reject group` buttons, disabled accept when nothing is eligible); `acceptGroup` stages each hold's *own* `proposed_action`/`proposed_note` into the existing `state.pending` map and calls the existing `persistDrafts()`/render path — same save mechanism as a manual per-item decision, confirmed by reading the surrounding code, including the pre-existing manual-override assignment at the p/c/x/d keypress handler (`state.pending[hold.id] = {...}`, unconditional overwrite) that proves a later manual choice always wins over a group-staged one; `rejectGroup` clears pending entries for the group's members without writing anything.
- `internal/server/static/style.css` — `.group-tag` and `#group-banner`/button styling, purely cosmetic.
- TDD order honored: `6e4dcff` (red) is the direct parent of `6f98a5f` (green).

Carried-over, non-blocking gap (same class as hc-1qi's LOG-ONLY-label gap, and true here for the same reason): this repo's `make check`/CI has no browser/JS test lane, so the banner rendering, `escapeHTML()` coverage, and manual-override-always-wins behavior in `app.js` were verified by direct code read, not by an automated JS test. Not treated as a coverage failure — verified true, just not test-enforced. The Go-level behavioral guarantees (group-field population, folder construction, filtering) *are* covered by real tests (see criterion 3).

**3. Tests pass — PASS**
- **3a (non-diff-owned failures):** n/a — 0 FAIL anywhere in either suite this run; nothing to attribute.
- **Go:**
  `test_cmd: make test-race` (→ `go test -race ./...`)
  `test_cmd_scope: full-suite`
  `test_counts: 60 PASS, 0 FAIL, 0 SKIP` (top-level `--- PASS:` lines, independently grepped from a fresh verbose run) — matches hc-dsy's independently-recorded count exactly.
  Re-ran clean via direct redirect (no pipe, unambiguous exit code): `REAL_EXIT_CODE:0`.
  `diff_tests_executed`: all 8 new/changed tests resolved individually by exact name against the verbose log, each `--- PASS`: `TestRead_ValidFileParses`, `TestRead_MissingFileReturnsEmptyMapNilError`, `TestRead_MalformedJSONReturnsError`, `TestKey_RoundTrips` (`internal/group/group_test.go`); `TestHandleHolds_SetsGroupFieldsOnlyForGroupedHold`, `TestHandleHolds_NoGroupsFileLeavesGroupFieldsUnset`, `TestBuildFolders_GroupFolderHasCorrectCountAndLabel`, `TestFilterByFolder_GroupPrefixReturnsOnlyMatchingHolds` (`internal/server/group_test.go`). Read every one of these test bodies in full (not just their names) — all are substantive, asserting real field values/counts/membership, not vacuous.
- **Python:**
  `test_cmd: make test-adapters` (→ `python3 -m unittest discover -s adapters/mpr -p 'test_*.py'`)
  `test_cmd_scope: full-suite`
  `test_counts: 22 PASS, 0 FAIL, 0 SKIP` (lower than hc-1qi's 26 because this diff doesn't touch `adapters/mpr` at all — expected, not a regression; confirmed by diffstat).
  `diff_tests_executed`: n/a — this diff has no Python changes; the suite is run in full regardless, per `test_cmd_scope: full-suite`, and stays green.
  Side investigation: `test_install.InstallTests.test_discovers_selected_city_prefix` prints `"Connected to reviewer. Start/restart with make run..."` to stdout during the run. Traced this to `adapters/mpr/install_local.py`, a CLI script with no `if __name__` guard, exercised here via `runpy.run_path(..., run_name='__main__')`. Read `test_install.py` in full: `subprocess.run`/`subprocess.check_output`/`shutil.which` are all mocked, `XDG_DATA_HOME`/`XDG_CONFIG_HOME` are redirected into a `tempfile.TemporaryDirectory()`, and every path the script touches (`--city`, the checkout, the config) is inside that temp tree. No real host state (no real `systemctl`, no real git-exclude or `holdcourt.toml` write) is touched. Pre-existing, untouched by this diff, and confirmed harmless — noted here only because a surprising line in test output gets checked, not waved through.
- **3b (lint/policy lane):**
  `golangci-lint version`: `v2.13.2` — already matches the Makefile pin (installed during the hc-1qi gate on this same host), no drift this time.
  `make lint`: **0 issues**, clean on the first run — no recurrence of the stale-cache false positive seen on hc-1qi (this deployer worktree's cache was already cleaned during that gate). See Notes for the cross-reference to hc-kht/gm-17g0ym, the tracked version of that phenomenon.
- **3c (CI-config diff):** `ci_lane_run: n/a (no CI-config change in this diff)` — `git diff --name-only origin/main...HEAD -- .github/workflows/` is empty.

**4. No open HIGH-severity findings — PASS**
hc-dsy recorded a full OWASP Top 10 walk, all clear. Independently re-checked the diff's actual security-relevant surfaces myself rather than accepting that at face value:
- `internal/group/group.go`'s `os.ReadFile(path)` — `path` is `Config.GroupsFile`, an operator-supplied CLI flag/TOML value, the same trust level as the pre-existing `--feed`/`--rulings` paths. Justified `//nolint:gosec` matches that reasoning. Read-only; no write path exists in the package at all.
- `app.js` rendering of curator-authored `group`/`proposed_action`/`proposed_note` text: the hold-list group-tag span, the group banner's name/action/note, and the folder-label list (`f.label`, shared with the pre-existing `class:` folders) all route through the real DOM-based `escapeHTML()` — confirmed at each of the three call sites by reading the surrounding render functions, not just the definition. `notice()` (used by `acceptGroup`/`rejectGroup`) sets `textContent`, not `innerHTML`, so no injection surface there either even though it isn't escaped (doesn't need to be).
- Checked one non-obvious question directly: does staging a group's `proposed_action` values into `state.pending` create a new way to submit an unvalidated ruling action to the server, bypassing the client-side `VALID_ACTIONS` allowlist? Read `handleSaveRulings` (`internal/server/server.go:552`) — it does `ruling.Action(item.Action)` with no server-side enum validation, and `ruling.Action` is `type Action string` (no `Validate`/`IsValid` anywhere in `internal/ruling`). This is a real characteristic of the server's existing API surface, but confirmed it is **not diff-owned**: `git diff` shows `handleSaveRulings` is untouched by this change (the diff's only server.go edits are in `holdViews`/`buildHoldView`/`buildFolders`/`filterByFolder`/`Config`/`holdJSON`). Anyone who could already POST an arbitrary `action` string directly to `/api/rulings` could do so before this diff, with or without the group-groupings feature; the new client-side allowlist is strictly additive defense-in-depth for the UI's own bulk-staging convenience, not a narrowing of an existing boundary. Not a gate-blocking finding for this diff.

**5. Final branch is clean — PASS**
`git status --porcelain` on `deploy/hc-p5q-gate` at `6f98a5f`: only `?? .gitkeep` — untracked, pre-existing in this worktree (same file noted on the hc-1qi gate), not a tracked-file modification, not part of this diff, and won't appear in the PR. No uncommitted changes to any tracked file.

**6. Branch diverges cleanly from main — PASS**
`origin/main` tip (`f84a5609`) equals the merge-base with the reviewed commit exactly — a pure fast-forward. No rebase was needed.

**7. Single feature theme — PASS**
All 7 changed files belong to the one curator-groupings/tags feature (hc-8fo): the new `internal/group` package and its tests, the server wiring and its new tests, the `app.js` banner UI, its stylesheet, and the CLI flag. No unrelated files. Ancestry/citation-scope check passed citing `hc-p5q, hc-8fo` (confirmed legitimate sibling per header).

## Notes

- **Cross-reference on the golangci-lint shared-cache issue (informational, not a gate finding):** hc-1qi's gate (this same deployer worktree, prior bead) hit 7 stale-cache gosec false positives; hc-8fo's own builder session independently hit the same phenomenon and filed it as `hc-kht` (hold-court store, label `gate-tracker`, still open); hc-dsy's review independently corroborated it with a cache-cleared re-run. This gate's own lint run was clean on the first try (cache already cleaned from the hc-1qi gate), so there was nothing to diagnose here — noted only for continuity. The structural fix is tracked at the fleet level as `gm-17g0ym` (gc-management/HQ store), now cross-linked with `hc-kht` in both directions.
- No self-granted waiver was used or needed — there were no diff-owned or non-diff-owned failures of any kind to waive.
