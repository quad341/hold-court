#!/usr/bin/env python3
"""Read MPR artifacts and GitHub state into a local Hold Court feed.

This adapter never writes to MPR, posts to GitHub, or executes rulings.
"""
import argparse
import collections
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

SOURCE = "maintainer-pr-review-local"
FYI_CODES = {"arch-topic-flag-fyi", "arch-hold-skipped-nonship"}


def read_json(path):
    with path.open() as stream:
        value = json.load(stream)
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def optional_json(path):
    return read_json(path) if path.exists() else {}


def atomic_json(path, value):
    data = json.dumps(value, indent=2, ensure_ascii=False) + "\n"
    if path.exists() and path.read_text() == data:
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=".export-", suffix=".tmp", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            stream.write(data)
        os.replace(name, path)
    finally:
        Path(name).unlink(missing_ok=True)


def open_prs(repo):
    result = subprocess.run(
        ["gh", "api", "--paginate", "--slurp",
         f"repos/{repo}/pulls?state=open&per_page=100"],
        capture_output=True, text=True, timeout=180, check=True,
    )
    pages = json.loads(result.stdout)
    if not isinstance(pages, list) or any(not isinstance(p, list) for p in pages):
        raise ValueError(f"invalid GitHub pagination response for {repo}")
    return {pr["number"]: pr for page in pages for pr in page}


def matched_run(pr_dir, head):
    runs = pr_dir / "runs"
    if not runs.is_dir():
        return None
    # MPR's latest symlink can move while a new run is being prepared. Only
    # use an immutable timestamp directory, with metadata for the held head.
    for run in sorted(runs.iterdir(), reverse=True):
        if not re.fullmatch(r"\d{8}T\d{6}Z", run.name) or not run.is_dir():
            continue
        metadata = optional_json(run / "metadata.json")
        if metadata.get("headRefOid") == head:
            result = optional_json(run / "publish-result.json")
            if result:  # An incomplete rerun must not erase a standing hold.
                return run, metadata, result
    return None


def markdown_sections(text):
    """Keep source wording; headings are the MPR review output contract."""
    sections = {}
    heading = None
    lines = []
    fenced = False
    for line in text.splitlines():
        if line.lstrip().startswith(('```', '~~~')):
            fenced = not fenced
        match = None if fenced else re.match(r'^#{1,3}\s+(.+?)\s*#*$', line)
        if match:
            if heading:
                sections[heading] = '\n'.join(lines).strip()
            heading, lines = match[1].strip().lower(), []
        else:
            lines.append(line)
    if heading:
        sections[heading] = '\n'.join(lines).strip()
    return sections


def artifact_text(run, name):
    path = run / name
    return path.read_text() if path.is_file() else ''


def decision_context(run, decision, category, reason):
    summary = markdown_sections(artifact_text(run, 'review-summary.md'))
    synthesis = markdown_sections(artifact_text(run, 'synthesis-output.md'))
    sections = {**synthesis, **summary}
    proposed = {
        'auto-merge': 'Merge the reviewed head through the existing merge checks.',
        'fix-merge': 'Apply and verify the proposed fixes, then merge through the existing checks.',
        'cherry-pick': 'Integrate the selected changes with attribution and verification.',
        'request-changes': 'Ask the author for the changes described in the review.',
        'close-superseded': 'Close as superseded with an explanation and attribution.',
    }.get(category, 'No supported disposition was supplied; ask the agent to clarify before execution.')
    body = ['## Decision requiring your response', reason,
            '### Proposed disposition', f'**{category or "not supplied"}** — {proposed}',
            'This is the MPR proposal, not a resolved human decision. Respond to the hold: '
            'which path should the agent take, why, and under what conditions? The prepared '
            'contributor message below is a draft for the agent to adapt to your guidance.']
    disagreement = sections.get('disagreement notes')
    body += ['### What the reviewers disagree about (synthesis)', disagreement or
             'MPR did not provide a disagreement explanation. The individual positions below '
             'are evidence, not an inferred reconciliation. Ask for the missing comparison if needed.']
    for key, label in [('top findings', 'Synthesis findings'), ('correctness risks', 'Unresolved risks')]:
        if sections.get(key):
            body += [f'### {label}', sections[key]]
    contract = decision.get('escalation_topic') or sections.get('escalation topic')
    change = decision.get('critical_path_change_raw') or sections.get('critical path change')
    body += ['### Contract / behavior change evidence']
    if contract and contract.strip().lower() != 'no':
        body += [str(contract)]
    if change and change.strip().lower() not in {'n/a', 'behavior-preserving'}:
        body += [str(change)]
    for key, label in [('existing contract', 'Existing contract'), ('proposed contract', 'Proposed contract')]:
        body += [f'**{label}:** ' + (sections.get(key) or
                 'Not separately specified by MPR. Use the cited evidence above or request an explicit before/after comparison.')]
    diagnostic_position = len(body)
    diagnostics = []
    body += ['### Individual reviewer positions']
    models = decision.get('models') or {}
    categories = (decision.get('ambiguity') or {}).get('reviewer_categories') or {}
    reviewer_names = sorted({'qwen', 'claude', 'codex'} | set(categories) |
                            {p.name.removesuffix('-review.md') for p in (run/'intake').glob('*-review.md')})
    for name in reviewer_names:
        if not re.fullmatch(r'[A-Za-z0-9_-]+', name):
            continue
        source = f'intake/{name}-review.md'
        review = artifact_text(run, source)
        parts = markdown_sections(review)
        model = models.get(name) or {}
        status = model.get('status', 'not recorded') if isinstance(model, dict) else str(model)
        verdict = categories.get(name) or parts.get('category') or 'not supplied'
        body += [f'#### {name} — {verdict}', f'Status: {status}. Source: `{source}`.']
        if review.strip():
            found = False
            for key in ['reasoning', 'fix plan', 'top findings', 'correctness risks', 'escalation topics']:
                if parts.get(key):
                    body += [f'**{key.title()}**', parts[key]]
                    found = True
            if not found:
                body += [review]
            elif not parts.get('reasoning'):
                body += ['No separate reasoning section was supplied; findings and plans above are the available explanation.']
        else:
            body += ['Reviewer output is missing. Its verdict alone does not explain a disagreement.']
        diagnostic = artifact_text(run, f'intake/{name}-review.stderr.txt')
        if diagnostic.strip() and (not review.strip() or status not in {'completed', 'not recorded'}):
            diagnostics += [f'### Reviewer error: {name}', diagnostic_block(diagnostic)]
    fix_plan = artifact_text(run, 'fix-plan.md')
    body += ['### Proposed fix plan', fix_plan.strip() or 'No fix plan was supplied. Do not infer what should be fixed from a verdict label.']
    # Preserve exact diagnostics when an infrastructure/precheck failure caused a hold.
    for name in ['runner-status.json', 'ensemble-status.json', 'ambiguity.json', 'out-of-diff.json', 'docs-render.json', 'codeql-alerts.json']:
        detail = optional_json(run/name)
        if has_failure(detail):
            diagnostics += [f'### Diagnostic evidence: {name}', diagnostic_block(json.dumps(detail, indent=2))]
    for name in ['synthesis.stderr.txt', 'pr-checkout.stderr.txt', 'diff.stderr.txt', 'metadata.stderr.txt']:
        diagnostic = artifact_text(run, name)
        if diagnostic.strip() and (has_failure(optional_json(run/'runner-status.json')) or any(word in reason.lower() for word in ['error', 'failed', 'timeout'])):
            diagnostics += [f'### Stage output: {name}', diagnostic_block(diagnostic)]
    body[diagnostic_position:diagnostic_position] = diagnostics
    return '\n\n'.join(body)


def has_failure(value):
    if isinstance(value, dict):
        if str(value.get('status', '')).lower() in {'error', 'failed', 'timeout'} or str(value.get('phase', '')).lower() in {'error', 'failed'}:
            return True
        if value.get('error') or value.get('failed_checks') or value.get('unresolved_reviews') or value.get('exit_code', 0):
            return True
        return any(has_failure(item) for item in value.values())
    if isinstance(value, list):
        return any(has_failure(item) for item in value)
    return isinstance(value, str) and value.lower() in {'error', 'failed', 'timeout'}


def diagnostic_block(text):
    # Choose a fence longer than any run in the original diagnostic.
    fence = '`' * max(3, 1 + max((len(m.group()) for m in re.finditer(r'`+', text)), default=0))
    return f'{fence}text\n{text.strip()}\n{fence}'


def export_hold(repo, marker, live, record_only=True):
    notice = read_json(marker)
    if notice.get("signature") == "skip-too-large" or notice.get("reason_code") == "skip-too-large":
        return None, "excluded-too-large"
    if notice.get("reason_code") in FYI_CODES:
        return None, "informational"
    head = notice.get("head_sha", "")
    if not re.fullmatch(r"[0-9a-f]{40,64}", head):
        raise ValueError(f"missing/invalid held head: {marker}")
    number = int(marker.parent.name.removeprefix("pr-"))
    matched = matched_run(marker.parent, head)
    if matched is None:
        raise ValueError(f"no completed review matches held head: {marker}")
    run, metadata, outcome = matched
    if outcome.get("skipped_reason") == "too-large":
        return None, "excluded-too-large"
    reason = notice.get("reason", "").strip()
    if not reason:
        raise ValueError(f"missing hold reason: {marker}")
    held_at = notice.get("first_flagged_ts", "")
    parsed = datetime.datetime.fromisoformat(held_at.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError(f"hold timestamp lacks timezone: {marker}")
    held_at = parsed.isoformat().replace("+00:00", "Z")
    is_hold = outcome.get("status") == "human_hold" or (
        outcome.get("status") == "skipped" and outcome.get("skipped_reason") == "too-large"
    )
    current = live.get(number)
    if current is None:
        resolved = "PR is no longer open on GitHub"
    elif current["head"]["sha"] != head:
        resolved = "PR head changed; this review applies to the previous head"
    elif not is_hold:
        resolved = "MPR has a later non-hold outcome for this head"
    else:
        resolved = ""
    decision = optional_json(run / "review-decision.json")
    human_hold = optional_json(run / "human-hold.json")
    category = human_hold.get("synthesis_category") or decision.get("synthesis_category") or decision.get("category", "")
    hold_class = notice.get("reason_code") or (
        "skip-too-large" if outcome.get("skipped_reason") == "too-large" else "human-hold"
    )
    signature = notice.get("signature") or hold_class
    digest = hashlib.sha256(signature.encode()).hexdigest()[:10]
    hold_id = f"{repo.replace('/', '-')}-{number}-{head}-{digest}"
    body = [f"Held commit: `{head}`", f"Source run: `{run}`"]
    if resolved:
        body += ["## No longer actionable", resolved]
    prepared = run / "gh-review-body.md"
    draft = run / "contributor-draft.md"
    message = prepared if prepared.exists() else draft
    body += ["## Proposed contributor message", message.read_text() if message.exists() else
             "No contributor message draft was supplied. Ask the agent to prepare one from your response; a review summary is not an outgoing message."]
    author = ((current or {}).get("user") or {}).get("login")
    if not author:
        recorded_author = metadata.get("author") or {}
        author = recorded_author.get("login", "") if isinstance(recorded_author, dict) else recorded_author
    return {
        "author": author,
        "decision_context_md": decision_context(run, decision, category, reason),
        "id": hold_id, "source": SOURCE, "repo": repo, "pr": number,
        "url": f"https://github.com/{repo}/pull/{number}", "class": hold_class,
        "title": f"{repo} #{number}: {metadata.get('title', 'Held PR')}",
        "question": reason, "review_body_md": "\n\n".join(body),
        "verdict": category, "head_sha": head, "held_at": held_at,
        "resolved": bool(resolved), "resolved_reason": resolved,
    }, "stood-down" if resolved else "inbox"


def refresh(config, live_loader=open_prs):
    root = Path(config["artifact_root"]).resolve(strict=True)
    feed = Path(config["feed"])
    repos = config["repos"]
    for repo in repos:
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo):
            raise ValueError(f"invalid repository: {repo}")
    # Finish all network requests and parsing before replacing any feed files.
    # A failed refresh leaves the last successful snapshot available.
    live = {repo: live_loader(repo) for repo in repos}
    documents = {}
    counts = collections.Counter()
    warnings = []
    uncertain_prs = set()
    for repo in repos:
        repo_dir = root / repo.replace("/", "-")
        if not repo_dir.is_dir():
            raise ValueError(f"missing MPR repository directory: {repo_dir}")
        for marker in sorted(repo_dir.glob("pr-*/hold-notice.json")):
            counts["notices"] += 1
            try:
                hold, state = export_hold(repo, marker, live[repo], config.get("execution", "record-only") == "record-only")
            except (ValueError, KeyError, OSError) as exc:
                warnings.append(f"{marker}: {exc}")
                uncertain_prs.add((repo, int(marker.parent.name.removeprefix("pr-"))))
                continue
            counts[state] += 1
            if hold:
                documents[hold["id"]] = hold
    # Keep decisions addressable when their marker disappears or changes head.
    # Do not infer resolution when reading that PR's source failed.
    for path in feed.glob("*.json"):
        old = read_json(path)
        if old.get("source") == SOURCE and old.get("class") == "skip-too-large":
            archive = feed.parent / "excluded-too-large"
            archive.mkdir(exist_ok=True)
            path.replace(archive / path.name)
            continue
        if old.get("source") != SOURCE or old.get("id") in documents:
            continue
        if (old.get("repo"), old.get("pr")) in uncertain_prs:
            continue
        if not old.get("resolved"):
            old["resolved"] = True
            old["resolved_reason"] = "MPR standing hold was cleared or superseded"
            documents[old["id"]] = old
    for hold_id, hold in documents.items():
        if Path(hold_id).name != hold_id or hold_id in {".", ".."}:
            raise ValueError("invalid feed document identity")
        atomic_json(feed / (hold_id + ".json"), hold)
    report = {
        "last_success": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "counts": dict(counts), "warnings": warnings,
        "feed": str(feed), "execution": config.get("execution", "record-only"),
    }
    atomic_json(Path(config["status"]), report)
    print(json.dumps(report))
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=Path, default=Path(__file__).with_name("config.json"))
    args = parser.parse_args()
    try:
        refresh(read_json(args.config))
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as exc:
        print(f"MPR feed refresh failed; last successful feed retained: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
