#!/usr/bin/env python3
"""Checks for WAL archive + PITR scripts (E95). The full cluster drill runs in CI
on Postgres 17; this file covers the archive helper without a server."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
BACKUP = os.path.join(ROOT, "deploy", "flow", "backup")


class WalArchiveHelper(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="edu-wal-")

    def tearDown(self):
        shutil.rmtree(self.tmp, ignore_errors=True)

    def run_archive(self, env=None, args=None):
        cmd = [os.path.join(BACKUP, "wal-archive.sh")] + (args or [])
        e = os.environ.copy()
        if env:
            e.update(env)
        return subprocess.run(cmd, env=e, capture_output=True, text=True)

    def test_copies_and_is_idempotent(self):
        src = os.path.join(self.tmp, "src")
        arch = os.path.join(self.tmp, "arch")
        with open(src, "wb") as fh:
            fh.write(b"WALSEG-A")
        r = self.run_archive({"ARCHIVE_DIR": arch}, [src, "0001"])
        self.assertEqual(r.returncode, 0, r.stderr)
        dest = os.path.join(arch, "0001")
        self.assertEqual(Path(dest).read_bytes(), b"WALSEG-A")
        r2 = self.run_archive({"ARCHIVE_DIR": arch}, [src, "0001"])
        self.assertEqual(r2.returncode, 0, r2.stderr)

    def test_refuses_overwrite_and_dangerous_dir(self):
        src = os.path.join(self.tmp, "src")
        other = os.path.join(self.tmp, "other")
        arch = os.path.join(self.tmp, "arch")
        Path(src).write_bytes(b"ONE")
        Path(other).write_bytes(b"TWO")
        self.run_archive({"ARCHIVE_DIR": arch}, [src, "0001"])
        r = self.run_archive({"ARCHIVE_DIR": arch}, [other, "0001"])
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("overwrite", r.stderr)
        r2 = self.run_archive({"ARCHIVE_DIR": "/"}, [src, "0001"])
        self.assertNotEqual(r2.returncode, 0)
        self.assertIn("dangerous", r2.stderr)

    def test_pitr_restore_refuses_nonempty_target(self):
        base = os.path.join(self.tmp, "base")
        wal = os.path.join(self.tmp, "wal")
        dest = os.path.join(self.tmp, "dest")
        os.makedirs(base)
        os.makedirs(wal)
        os.makedirs(dest)
        Path(dest, "x").write_text("no")
        r = subprocess.run(
            [os.path.join(BACKUP, "pitr-restore.sh"), base, wal, dest, "2026-11-12 12:00:00"],
            capture_output=True, text=True,
        )
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("not empty", r.stderr)

    def test_pitr_restore_sets_data_dir_mode(self):
        base = os.path.join(self.tmp, "base")
        wal = os.path.join(self.tmp, "wal")
        dest = os.path.join(self.tmp, "dest")
        os.makedirs(base)
        os.makedirs(wal)
        Path(base, "PG_VERSION").write_text("17")
        r = subprocess.run(
            [os.path.join(BACKUP, "pitr-restore.sh"), base, wal, dest, "2026-11-12 12:00:00"],
            capture_output=True, text=True,
        )
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        mode = os.stat(dest).st_mode & 0o777
        self.assertIn(mode, (0o700, 0o750), oct(mode))


if __name__ == "__main__":
    unittest.main(verbosity=2)
