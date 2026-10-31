"""DEV-110: distribution notices stay in step with the code.

Every source file that says it adapts or derives from edulab must be listed in
NOTICE with a per-file header, the Apache-2.0 text must ship next to it, and the
service must not pull fonts, CDN scripts or third-party packages beyond the
pinned, inventoried requirements.
"""

import os
import re
import unittest

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PKG = os.path.join(ROOT, "mathsvc")


def _read(*parts):
    with open(os.path.join(ROOT, *parts), encoding="utf-8") as f:
        return f.read()


def _sources():
    for base, _, files in os.walk(PKG):
        for name in files:
            if name.endswith(".py"):
                path = os.path.join(base, name)
                with open(path, encoding="utf-8") as fh:
                    yield os.path.relpath(path, ROOT).replace(os.sep, "/"), fh.read()


class NoticeTests(unittest.TestCase):
    def test_license_text_and_baseline(self):
        lic = _read("LICENSE-edulab")
        self.assertIn("Apache License", lic)
        self.assertIn("Version 2.0", lic)
        notice = _read("NOTICE")
        self.assertIn("805fdc5dd6bdf942290a7791924bf0ee08f26632", notice)
        self.assertIn("Original NOTICE of edulab", notice)

    def test_every_adapted_file_is_listed_and_marked(self):
        notice = _read("NOTICE")
        adapted = [p for p, text in _sources() if re.search(r"(Adapted from|inspired by) edulab", text, re.I)]
        self.assertTrue(adapted, "expected at least the geometry kernel to be adapted from edulab")
        for path in adapted:
            self.assertIn(path, notice, f"{path} adapts edulab but is not listed in NOTICE")

    def test_no_fonts_cdn_or_unpinned_dependencies(self):
        for path, text in _sources():
            if path == "mathsvc/render_check.py":
                continue  # the presentation checker names these patterns in order to reject them
            self.assertIsNone(re.search(r"https?://(cdn|unpkg|fonts\.googleapis|cdnjs)", text), path)
            self.assertIsNone(re.search(r"@font-face|\.woff2?\b|\.ttf\b", text), path)
        reqs = [l.strip() for l in _read("requirements.txt").splitlines() if l.strip() and not l.startswith("#")]
        self.assertEqual(reqs, ["sympy==1.13.3", "mpmath==1.3.0"], "new Python dependencies must be added to the 06 license inventory")
        inventory = os.path.join(ROOT, "..", "..", "docs", "math-workbench", "06-数学目录-限制-许可.md")
        if os.path.exists(inventory):  # the service image ships without docs
            with open(inventory, encoding="utf-8") as fh:
                rows = fh.read()
            for req in reqs:
                name, version = req.split("==")
                self.assertRegex(rows, rf"\| {re.escape(name)} \| {re.escape(version)} \|", f"{req} missing from the 06 inventory")


if __name__ == "__main__":
    unittest.main()
