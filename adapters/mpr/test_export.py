import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("mpr_export", Path(__file__).with_name("export.py"))
adapter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(adapter)


class ExportTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.pr_dir = self.root / "artifacts/owner-repo/pr-42"
        self.run = self.pr_dir / "runs/20260905T100000Z"
        self.run.mkdir(parents=True)
        self.head = "a" * 40
        self.marker = self.pr_dir / "hold-notice.json"
        self.put(self.marker, {"head_sha": self.head, "reason": "Please decide scope",
                              "reason_code": "defer-human", "signature": "defer-human",
                              "first_flagged_ts": "2026-09-05T10:01:00Z"})
        self.put(self.run / "metadata.json", {"headRefOid": self.head, "title": "Held change"})
        self.put(self.run / "publish-result.json", {"status": "human_hold"})
        (self.run / "gh-review-body.md").write_text("Correct held-head review")
        self.live = {42: {"head": {"sha": self.head}}}
        self.config = {"artifact_root": str(self.root / "artifacts"), "repos": ["owner/repo"],
                       "feed": str(self.root / "feed"), "status": str(self.root / "status.json")}

    def put(self, path, value):
        path.write_text(json.dumps(value))

    def refresh(self, loader=None):
        with contextlib.redirect_stdout(io.StringIO()):
            return adapter.refresh(self.config, loader or (lambda repo: self.live))

    def docs(self):
        return [json.loads(p.read_text()) for p in (self.root / "feed").glob("*.json")]

    def test_current_head_and_exact_review(self):
        newer = self.pr_dir / "runs/20260905T110000Z"
        newer.mkdir()
        self.put(newer / "metadata.json", {"headRefOid": "b" * 40})
        self.put(newer / "publish-result.json", {"status": "human_hold"})
        (newer / "gh-review-body.md").write_text("WRONG HEAD REVIEW")
        self.refresh()
        hold = self.docs()[0]
        self.assertFalse(hold["resolved"])
        self.assertIn("Correct held-head review", hold["review_body_md"])
        self.assertNotIn("WRONG HEAD", hold["review_body_md"])
        self.assertEqual(hold["question"], "Please decide scope")

    def test_closed_and_changed_heads_stand_down(self):
        for live, reason in [({}, "no longer open"), ({42: {"head": {"sha": "b" * 40}}}, "head changed")]:
            self.live = live
            self.refresh()
            hold = self.docs()[0]
            self.assertTrue(hold["resolved"])
            self.assertIn(reason, hold["resolved_reason"])

    def test_fyi_not_actionable(self):
        notice = json.loads(self.marker.read_text())
        notice["reason_code"] = "arch-topic-flag-fyi"
        self.put(self.marker, notice)
        result = self.refresh()
        self.assertEqual(result["counts"]["informational"], 1)
        self.assertEqual(self.docs(), [])

    def test_published_outcome_and_removed_marker_resolve(self):
        self.refresh()
        self.put(self.run / "publish-result.json", {"status": "published"})
        self.refresh()
        self.assertTrue(self.docs()[0]["resolved"])
        self.put(self.run / "publish-result.json", {"status": "human_hold"})
        self.refresh()
        self.marker.unlink()
        self.refresh()
        self.assertIn("cleared or superseded", self.docs()[0]["resolved_reason"])

    def test_network_failure_preserves_snapshot(self):
        self.refresh()
        before = self.docs()
        def failure(repo):
            raise OSError("offline")
        with self.assertRaises(OSError):
            self.refresh(failure)
        self.assertEqual(before, self.docs())

    def test_malformed_source_preserves_previous_hold(self):
        self.refresh()
        before = self.docs()
        self.marker.write_text("{")
        report = self.refresh()
        self.assertEqual(len(report["warnings"]), 1)
        self.assertEqual(before, self.docs())

    def test_too_large_is_excluded(self):
        self.put(self.run / "publish-result.json", {"status": "skipped", "skipped_reason": "too-large"})
        (self.run / "gh-review-body.md").unlink()
        self.refresh()
        self.assertEqual(self.docs(), [])


if __name__ == "__main__":
    unittest.main()
