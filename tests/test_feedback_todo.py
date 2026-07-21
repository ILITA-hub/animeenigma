from __future__ import annotations

import argparse
import contextlib
import importlib.util
import io
import json
import tempfile
import unittest
from importlib.machinery import SourceFileLoader
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "bin" / "feedback-todo"
SPEC = importlib.util.spec_from_loader(
    "feedback_todo", SourceFileLoader("feedback_todo", str(SCRIPT))
)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("cannot load feedback-todo")
feedback_todo = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(feedback_todo)


class FeedbackTodoReadTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        feedback_todo.REPORTS_DIR = self.root
        feedback_todo.STATUS_FILE = self.root / "_status.json"
        feedback_todo.STATUS_HISTORY_FILE = self.root / "_status_history.json"
        feedback_todo.ATTACHMENTS_DIR = self.root / "_attachments"
        self.report_id = "2026-07-21T01-42-59_owner_manual"
        self.report = {
            "source": "manual",
            "player_type": "feedback",
            "username": "owner",
            "user_id": "creator-123",
            "description": "Build the requested feature",
            "category": "feature",
            "attachments": ["design.png", "notes.txt"],
            "custom_field": {"preserved": True},
        }
        (self.root / f"{self.report_id}.json").write_text(
            json.dumps(self.report), encoding="utf-8"
        )
        (self.root / "_status.json").write_text(
            json.dumps(
                {
                    self.report_id: {
                        "status": "in_progress",
                        "updated_at": "2026-07-21T01:45:00Z",
                        "updated_by": "codex",
                    }
                }
            ),
            encoding="utf-8",
        )
        (self.root / "_status_history.json").write_text(
            json.dumps(
                {
                    self.report_id: [
                        {
                            "from": "new",
                            "to": "in_progress",
                            "at": "2026-07-21T01:45:00Z",
                            "by": "codex",
                        }
                    ]
                }
            ),
            encoding="utf-8",
        )
        attachment_dir = self.root / "_attachments" / self.report_id
        attachment_dir.mkdir(parents=True)
        (attachment_dir / "design.png").write_bytes(b"fake-png")
        (attachment_dir / "notes.txt").write_text("details", encoding="utf-8")

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_show_returns_complete_record_status_history_and_attachment_metadata(self) -> None:
        output = io.StringIO()
        args = argparse.Namespace(report_id=self.report_id, attachments_dir=None)

        with contextlib.redirect_stdout(output):
            feedback_todo.cmd_show(args)

        result = json.loads(output.getvalue())
        self.assertEqual(result["username"], "owner")
        self.assertEqual(result["user_id"], "creator-123")
        self.assertEqual(result["description"], "Build the requested feature")
        self.assertEqual(result["custom_field"], {"preserved": True})
        self.assertEqual(result["status"], "in_progress")
        self.assertEqual(result["status_updated_by"], "codex")
        self.assertEqual(result["status_history"][0]["from"], "new")
        self.assertEqual(
            [item["name"] for item in result["attachment_metadata"]],
            ["design.png", "notes.txt"],
        )
        self.assertTrue(all(item["available"] for item in result["attachment_metadata"]))
        self.assertTrue(all(len(item["sha256"]) == 64 for item in result["attachment_metadata"]))

    def test_show_can_copy_all_attachments_to_new_private_directory(self) -> None:
        output_dir = self.root / "export"
        output = io.StringIO()
        args = argparse.Namespace(report_id=self.report_id, attachments_dir=output_dir)

        with contextlib.redirect_stdout(output):
            feedback_todo.cmd_show(args)

        result = json.loads(output.getvalue())
        self.assertEqual((output_dir / "design.png").read_bytes(), b"fake-png")
        self.assertEqual((output_dir / "notes.txt").read_text(encoding="utf-8"), "details")
        self.assertEqual(output_dir.stat().st_mode & 0o777, 0o700)
        self.assertTrue(all("saved_to" in item for item in result["attachment_metadata"]))

    def test_attachment_copies_only_a_listed_file_without_overwrite(self) -> None:
        output_path = self.root / "copied" / "notes.txt"
        args = argparse.Namespace(
            report_id=self.report_id,
            name="notes.txt",
            output=output_path,
        )
        with contextlib.redirect_stdout(io.StringIO()):
            feedback_todo.cmd_attachment(args)
        self.assertEqual(output_path.read_text(encoding="utf-8"), "details")
        self.assertEqual(output_path.stat().st_mode & 0o777, 0o600)

        with contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit):
                feedback_todo.cmd_attachment(args)

    def test_show_refuses_non_manual_reports(self) -> None:
        self.report["source"] = "feedback_form"
        (self.root / f"{self.report_id}.json").write_text(
            json.dumps(self.report), encoding="utf-8"
        )
        args = argparse.Namespace(report_id=self.report_id, attachments_dir=None)

        with contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit):
                feedback_todo.cmd_show(args)

    def test_missing_or_symlinked_attachment_is_not_read(self) -> None:
        attachment_dir = self.root / "_attachments" / self.report_id
        (attachment_dir / "notes.txt").unlink()
        (attachment_dir / "notes.txt").symlink_to(self.root / f"{self.report_id}.json")
        args = argparse.Namespace(report_id=self.report_id, attachments_dir=None)
        output = io.StringIO()

        with contextlib.redirect_stdout(output):
            feedback_todo.cmd_show(args)

        result = json.loads(output.getvalue())
        notes = next(
            item for item in result["attachment_metadata"] if item["name"] == "notes.txt"
        )
        self.assertFalse(notes["available"])
        self.assertNotIn("sha256", notes)


if __name__ == "__main__":
    unittest.main()
