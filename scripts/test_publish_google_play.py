"""Tests for transactional Google Play release publishing."""

from pathlib import Path
import tempfile
import unittest
from urllib.parse import parse_qs, urlparse

import publish_google_play as publisher


class FakePublisher:
    def __init__(self, *, releases=None, fail_at=None):
        self.releases = releases or []
        self.fail_at = fail_at
        self.calls = []

    def _call(self, name, *args):
        self.calls.append((name, args))
        if self.fail_at == name:
            raise publisher.GooglePlayError(f"failed at {name}")

    def insert_edit(self):
        self._call("insert_edit")
        return "edit-1"

    def get_track(self, edit_id, track):
        self._call("get_track", edit_id, track)
        return {"track": track, "releases": self.releases}

    def upload_bundle(self, edit_id, path):
        self._call("upload_bundle", edit_id, path)
        return 123

    def upload_mapping(self, edit_id, version_code, path):
        self._call("upload_mapping", edit_id, version_code, path)

    def update_track(self, edit_id, track, payload):
        self._call("update_track", edit_id, track, payload)

    def commit_edit(self, edit_id):
        self._call("commit_edit", edit_id)

    def delete_edit(self, edit_id):
        self._call("delete_edit", edit_id)


class GooglePlayPublishingTest(unittest.TestCase):
    def test_release_notes_are_loaded_and_validated(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "ko.txt"
            path.write_text("새 릴리스\n", encoding="utf-8")
            notes = publisher.load_release_notes([f"ko-KR={path}"])
        self.assertEqual(notes, [{"language": "ko-KR", "text": "새 릴리스"}])

    def test_release_notes_reject_duplicates_and_oversize_text(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "notes.txt"
            path.write_text("x" * 501, encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "at most 500"):
                publisher.load_release_notes([f"en-US={path}"])
            path.write_text("notes", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "duplicate"):
                publisher.load_release_notes([
                    f"en-US={path}",
                    f"en-US={path}",
                ])

    def test_successful_publish_is_committed_without_cleanup(self):
        fake = FakePublisher(releases=[{"status": "completed"}])
        version_code = publisher.publish_release(
            fake,
            aab_path=Path("app.aab"),
            mapping_path=Path("mapping.txt"),
            track="production",
            status="completed",
            release_name="v0.6.0",
            release_notes=[{"language": "en-US", "text": "notes"}],
        )
        self.assertEqual(version_code, 123)
        self.assertEqual(
            [name for name, _ in fake.calls],
            [
                "insert_edit",
                "get_track",
                "upload_bundle",
                "upload_mapping",
                "update_track",
                "commit_edit",
            ],
        )

    def test_track_payload_uses_string_version_code(self):
        payload = publisher.build_track_payload(
            "production",
            123,
            "v0.6.0",
            "completed",
            [{"language": "en-US", "text": "notes"}],
        )
        self.assertEqual(payload["track"], "production")
        self.assertEqual(payload["releases"][0]["versionCodes"], ["123"])
        self.assertEqual(payload["releases"][0]["status"], "completed")

    def test_pending_rollout_is_not_replaced(self):
        fake = FakePublisher(releases=[{"status": "inProgress"}])
        with self.assertRaisesRegex(publisher.GooglePlayError, "unfinished"):
            publisher.publish_release(
                fake,
                aab_path=Path("app.aab"),
                mapping_path=Path("mapping.txt"),
                track="production",
                status="completed",
                release_name="v0.6.0",
                release_notes=[{"language": "en-US", "text": "notes"}],
            )
        self.assertEqual(
            [name for name, _ in fake.calls],
            ["insert_edit", "get_track", "delete_edit"],
        )

    def test_failed_publish_deletes_the_edit(self):
        fake = FakePublisher(fail_at="commit_edit")
        with self.assertRaisesRegex(publisher.GooglePlayError, "commit_edit"):
            publisher.publish_release(
                fake,
                aab_path=Path("app.aab"),
                mapping_path=Path("mapping.txt"),
                track="production",
                status="completed",
                release_name="v0.6.0",
                release_notes=[{"language": "en-US", "text": "notes"}],
            )
        self.assertEqual(fake.calls[-1], ("delete_edit", ("edit-1",)))

    def test_commit_never_cancels_an_existing_review(self):
        captured = {}

        class Response:
            def __enter__(self):
                return self

            def __exit__(self, *_args):
                return False

            @staticmethod
            def read():
                return b"{}"

        def opener(request, *, timeout):
            captured["request"] = request
            captured["timeout"] = timeout
            return Response()

        client = publisher.GooglePlayPublisher(
            "io.github.example.app", "secret-token", opener=opener
        )
        client.commit_edit("edit 1")
        request = captured["request"]
        parsed = urlparse(request.full_url)
        self.assertEqual(request.method, "POST")
        self.assertTrue(parsed.path.endswith("/edits/edit%201:commit"))
        self.assertEqual(
            parse_qs(parsed.query),
            {"changesInReviewBehavior": ["ERROR_IF_IN_REVIEW"]},
        )
        self.assertEqual(request.headers["Authorization"], "Bearer secret-token")


if __name__ == "__main__":
    unittest.main()
