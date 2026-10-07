"""Exercise component promotion against a local Git remote, including races."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import promote_components


class ComponentPromotionTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.remote = self.root / "remote.git"
        self.seed = self.root / "seed"
        self.product = self.root / "product"
        self.git(self.root, "init", "--bare", str(self.remote))
        self.git(self.root, "init", "-b", "main", str(self.seed))
        self.configure(self.seed)
        self.manifest = {
            "core": "1" * 40,
            "frontend": "2" * 40,
            "authd": "3" * 40,
            "ebiten": "4" * 40,
            "test_runner": "5" * 40,
        }
        (self.seed / "product-components.json").write_text(
            json.dumps(self.manifest) + "\n", encoding="utf-8"
        )
        self.git(self.seed, "add", "product-components.json")
        self.git(self.seed, "commit", "-m", "initial product")
        self.baseline = self.git(self.seed, "rev-parse", "HEAD")
        self.git(self.seed, "remote", "add", "origin", str(self.remote))
        self.git(self.seed, "push", "origin", "HEAD:main")
        self.git(self.root, "clone", "--branch", "main", str(self.remote), str(self.product))
        self.configure(self.product)
        heads = patch.object(
            promote_components, "component_main_heads",
            return_value={"core": "a" * 40, "frontend": "b" * 40},
        )
        heads.start()
        self.addCleanup(heads.stop)

    @staticmethod
    def git(root, *arguments):
        result = subprocess.run(
            ["git", "-C", str(root), *arguments],
            check=True,
            capture_output=True,
            text=True,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
        )
        return result.stdout.strip()

    def configure(self, repository):
        self.git(repository, "config", "user.name", "component promotion test")
        self.git(repository, "config", "user.email", "test@example.invalid")
        self.git(repository, "config", "commit.gpgsign", "false")

    def advance_main(self):
        (self.seed / "concurrent.txt").write_text("concurrent work\n", encoding="utf-8")
        self.git(self.seed, "add", "concurrent.txt")
        self.git(self.seed, "commit", "-m", "concurrent product update")
        self.git(self.seed, "push", "origin", "HEAD:main")
        return self.git(self.seed, "rev-parse", "HEAD")

    def promote(self):
        return promote_components.promote(
            self.product, self.baseline, "a" * 40, "b" * 40
        )

    def test_promotes_only_candidate_pins_and_preserves_test_runner(self):
        promoted = self.promote()
        self.assertNotEqual(promoted, self.baseline)
        self.assertEqual(promoted, self.git(self.remote, "rev-parse", "refs/heads/main"))
        self.assertEqual(self.git(self.product, "rev-parse", "HEAD^"), self.baseline)
        expected = {**self.manifest, "core": "a" * 40, "frontend": "b" * 40}
        self.assertEqual(
            json.loads((self.product / "product-components.json").read_text(encoding="utf-8")),
            expected,
        )
        self.assertEqual(self.git(self.product, "status", "--porcelain"), "")

    def test_main_change_during_validation_requires_a_new_gate(self):
        concurrent = self.advance_main()
        self.assertIsNone(self.promote())
        self.assertEqual(self.git(self.remote, "rev-parse", "refs/heads/main"), concurrent)
        self.assertEqual(self.git(self.product, "rev-parse", "HEAD"), self.baseline)
        self.assertEqual(self.git(self.product, "status", "--porcelain"), "")

    def test_component_change_during_validation_requires_a_new_gate(self):
        with patch.object(promote_components, "component_main_heads", return_value={
            "core": "a" * 40, "frontend": "c" * 40,
        }):
            self.assertIsNone(self.promote())
        self.assertEqual(self.git(self.remote, "rev-parse", "refs/heads/main"), self.baseline)
        self.assertEqual(self.git(self.product, "status", "--porcelain"), "")

    def test_lagging_nightly_cannot_replace_current_component_pins(self):
        self.assertIsNone(promote_components.promote(
            self.product, self.baseline, "a" * 40, self.manifest["frontend"],
        ))
        self.assertEqual(self.git(self.remote, "rev-parse", "refs/heads/main"), self.baseline)
        self.assertEqual(
            json.loads((self.product / "product-components.json").read_text()),
            self.manifest,
        )

    def test_push_race_cannot_overwrite_a_new_main(self):
        original_git = promote_components.git
        concurrent = []

        def racing_git(repository, *arguments):
            if arguments[0] == "push":
                concurrent.append(self.advance_main())
            return original_git(repository, *arguments)

        with patch.object(promote_components, "git", side_effect=racing_git):
            with self.assertRaises(subprocess.CalledProcessError):
                self.promote()
        self.assertEqual(self.git(self.remote, "rev-parse", "refs/heads/main"), concurrent[0])

    def test_wrong_product_checkout_cannot_promote(self):
        self.advance_main()
        self.git(self.product, "pull", "--ff-only", "origin", "main")
        with self.assertRaises(ValueError):
            self.promote()

    def test_dirty_manifest_cannot_enter_a_verified_commit(self):
        manifest = self.product / "product-components.json"
        manifest.write_text("{}\n", encoding="utf-8")
        with self.assertRaises(ValueError):
            self.promote()
        self.assertEqual(manifest.read_text(encoding="utf-8"), "{}\n")

    def test_mutable_candidate_ref_is_rejected(self):
        with self.assertRaises(ValueError):
            promote_components.promote(self.product, self.baseline, "nightly", "b" * 40)


if __name__ == "__main__":
    unittest.main()
