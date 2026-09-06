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
    body = [
        "## Decision workflow",
        ("Saving records your decision locally. It does **not** clear the MPR hold, "
         "post a review, close a PR, or enable merging.") if record_only else
        "Saving and confirming sends your decision to the configured agent. Discussion requests analysis only; other actions authorize the specified PR operation on this reviewed head. Progress and replies appear in History & discussion.",
        "## Hold reason (verbatim from MPR)", reason,
        f"Held commit: `{head}`  \nMPR verdict: `{category or 'not available'}`",
        f"Source run: `{run}`",
    ]
    if resolved:
        body += ["## No longer actionable", resolved]
    prepared = run / "gh-review-body.md"
    summary = run / "review-summary.md"
    review = prepared if prepared.exists() else summary
    body += ["## Prepared review", review.read_text() if review.exists() else
             "MPR did not produce a prepared review for this hold (for example, the diff exceeded its review limit)."]
    return {
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
