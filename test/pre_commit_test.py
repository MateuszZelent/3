"""Exercise the real hook in disposable repos without compiling CUDA."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

HOOK = Path(__file__).resolve().parents[1] / "pre-commit"


class PreCommitTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="mumax-hook-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "--quiet")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "make.log"
        make = self.bin / "make"
        make.write_text('#!/bin/sh\nprintf "called\\n" >> "$HOOK_TEST_MAKE_LOG"\nexit "${HOOK_TEST_MAKE_EXIT:-0}"\n')
        make.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"], HOOK_TEST_MAKE_LOG=str(self.log))

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, check=True, capture_output=True)

    def stage(self, path, contents):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(contents)
        self.git("add", "--", path)
        return target

    def run_hook(self):
        index = (self.root / ".git/index").read_bytes()
        result = subprocess.run([str(HOOK)], cwd=self.root, env=self.env, capture_output=True, text=True)
        self.assertEqual(index, (self.root / ".git/index").read_bytes(), "hook modified the index")
        return result

    def test_non_go_commit_does_not_build_or_touch_unstaged_go(self):
        self.stage("notes.txt", "notes\n")
        dirty = self.root / "engine/dirty.go"
        dirty.parent.mkdir()
        dirty.write_text("package engine\nvar x=1\n")
        before = dirty.read_bytes()
        result = self.run_hook()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(self.log.exists(), "non-Go commit started a CUDA build")
        self.assertEqual(before, dirty.read_bytes(), "hook formatted unrelated work")

    def test_unformatted_staged_go_is_rejected_without_rewriting(self):
        target = self.stage("engine/value.go", "package engine\nvar x=1\n")
        before = target.read_bytes()
        self.assertNotEqual(self.run_hook().returncode, 0)
        self.assertEqual(before, target.read_bytes(), "hook rewrote working tree")
        self.assertFalse(self.log.exists())

    def test_staged_content_is_checked_even_if_worktree_is_formatted(self):
        target = self.stage("engine/value.go", "package engine\nvar x=1\n")
        target.write_text("package engine\n\nvar x = 1\n")
        self.assertNotEqual(self.run_hook().returncode, 0, "hook checked worktree instead of staged content")
        self.assertFalse(self.log.exists())

    def test_formatting_checks_index_without_rewriting_unstaged_changes(self):
        target = self.stage("engine/value.go", "package engine\n\nvar x = 1\n")
        target.write_text("uncommitted invalid syntax\n")
        result = self.run_hook()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(target.read_text(), "uncommitted invalid syntax\n")
        self.assertTrue(self.log.exists(), "Go commit skipped its build")

    def test_staged_syntax_error_is_rejected(self):
        self.stage("engine/value.go", "package engine\nfunc {\n")
        self.assertNotEqual(self.run_hook().returncode, 0)
        self.assertFalse(self.log.exists())

    def test_paths_with_spaces_are_checked(self):
        self.stage("engine/with space.go", "package engine\nvar x=1\n")
        self.assertNotEqual(self.run_hook().returncode, 0)
        self.assertFalse(self.log.exists())

    def test_archive_go_sources_do_not_build_or_get_reformatted(self):
        path = self.stage("doc/audits/example/snapshot/old.go", "package old\nvar x=1\n")
        result = self.run_hook()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(path.read_text(), "package old\nvar x=1\n")
        self.assertFalse(self.log.exists())

    def test_dependency_changes_keep_build_gate(self):
        self.stage("go.mod", "module example.test/test\n\ngo 1.26.10\n")
        result = self.run_hook()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.log.exists())

    def test_build_failure_rejects_commit(self):
        self.stage("engine/value.go", "package engine\n\nvar x = 1\n")
        self.env["HOOK_TEST_MAKE_EXIT"] = "7"
        self.assertNotEqual(self.run_hook().returncode, 0)
        self.assertTrue(self.log.exists())

    def commit_fixture(self):
        self.git("-c", "user.name=Hook Test", "-c", "user.email=hook@example.invalid",
                 "-c", "core.hooksPath=/dev/null", "commit", "--quiet", "-m", "fixture")

    def test_deleted_go_file_keeps_build_gate(self):
        self.stage("engine/value.go", "package engine\n\nvar x = 1\n")
        self.commit_fixture()
        self.git("rm", "engine/value.go")
        result = self.run_hook()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.log.exists())

    def test_move_from_production_to_archive_keeps_build_gate(self):
        self.stage("engine/value.go", "package engine\n\nvar x = 1\n")
        self.commit_fixture()
        (self.root / "doc/audits/archive").mkdir(parents=True)
        self.git("mv", "engine/value.go", "doc/audits/archive/value.go")
        result = self.run_hook()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.log.exists(), "moving production code out of the module skipped its build")


if __name__ == "__main__":
    unittest.main()
