# -*- coding: utf-8 -*-
"""Automatic tests for frozen-set tooling (B4, E94).

These use a fictional four-row sample. They do not constitute official
admission, a math-lead sign-off, or the teacher-authored 240-case set.
"""

import json
import os
import shutil
import tempfile
import unittest
from copy import deepcopy
from pathlib import Path

from mathsvc import catalog, frozen_set as fs
from mathsvc.errors import MathError
from mathsvc.families import REGISTRY

HERE = Path(__file__).resolve().parent
FIX = HERE / "fixtures" / "frozen_set"
CSV = FIX / "frozen_set_sample-2026-11-12.csv"
META = FIX / "frozen_set_sample-2026-11-12.meta.json"
SHA = FIX / "frozen_set_sample-2026-11-12.csv.sha256"
ANSWERS = FIX / "frozen_set_sample-2026-11-12_blind_answers.csv"
BLIND_ANSWER = "6*sqrt(77)/77"


class FrozenSetToolTests(unittest.TestCase):
    def setUp(self):
        self._env = {k: os.environ.get(k) for k in ("MATH_CATALOG_ADMISSION_OVERLAY", "MATH_FROZEN_PARAM_RANGES")}
        os.environ.pop("MATH_CATALOG_ADMISSION_OVERLAY", None)
        os.environ.pop("MATH_FROZEN_PARAM_RANGES", None)
        self.tmp = tempfile.mkdtemp(prefix="frozen-set-")

    def tearDown(self):
        for k, v in self._env.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v
        shutil.rmtree(self.tmp, ignore_errors=True)

    def test_sample_validates_and_imports_idempotently(self):
        meta = fs.load_meta(str(META))
        result = fs.validate(str(CSV), meta, answers=fs.load_answers(str(ANSWERS)), sha_path=str(SHA))
        self.assertEqual(result["kind"], fs.SAMPLE_KIND)
        self.assertFalse(result["official"])
        self.assertFalse(result["counts_toward_admission"])
        self.assertEqual(result["n"], 4)
        first = fs.import_set(str(CSV), str(META), self.tmp, answers_path=str(ANSWERS), sha_path=str(SHA))
        self.assertFalse(first["idempotent"])
        again = fs.import_set(str(CSV), str(META), self.tmp, answers_path=str(ANSWERS), sha_path=str(SHA))
        self.assertTrue(again["idempotent"])
        other = Path(self.tmp) / "tampered.csv"
        text = CSV.read_text(encoding="utf-8").replace("底边 2", "底边 2 ")
        other.write_text(text, encoding="utf-8")
        with self.assertRaises(fs.FrozenSetError) as ctx:
            fs.import_set(str(other), str(META), self.tmp, answers_path=str(ANSWERS))
        self.assertIn("different checksum", str(ctx.exception))

    def test_blind_answers_and_source_groups_are_isolated(self):
        meta = fs.load_meta(str(META))
        leaked = Path(self.tmp) / "leaked.csv"
        leaked.write_text(CSV.read_text(encoding="utf-8").replace(
            "FQ-S003,supported,SOLID-ANGLE-02,blind,G-03,geometric,",
            "FQ-S003,supported,SOLID-ANGLE-02,blind,G-03,geometric,",
        ).replace(
            '"answer","","","",blocker,教师甲,,,原创,教师乙,2026-11-12 21:00（UTC+9）',
            '"answer","%s","","",blocker,教师甲,,,原创,教师乙,2026-11-12 21:00（UTC+9）' % BLIND_ANSWER,
        ), encoding="utf-8")
        # The fixture quoting may not match the replace above; fall back to rewriting the table.
        rows = fs.load_table(str(CSV))
        for r in rows:
            if r["case_id"] == "FQ-S003":
                r["expected_answer"] = BLIND_ANSWER
        import csv
        with open(leaked, "w", encoding="utf-8", newline="") as fh:
            w = csv.DictWriter(fh, fieldnames=list(rows[0].keys()), lineterminator="\n")
            w.writeheader()
            w.writerows(rows)
        with self.assertRaises(fs.FrozenSetError) as ctx:
            fs.validate(str(leaked), meta, answers=fs.load_answers(str(ANSWERS)))
        self.assertIn("leaks", str(ctx.exception))

        crossed = Path(self.tmp) / "crossed.csv"
        rows = fs.load_table(str(CSV))
        for r in rows:
            if r["case_id"] == "FQ-S003":
                r["source_group"] = "G-01"
        with open(crossed, "w", encoding="utf-8", newline="") as fh:
            w = csv.DictWriter(fh, fieldnames=list(rows[0].keys()), lineterminator="\n")
            w.writeheader()
            w.writerows(rows)
        with self.assertRaises(fs.FrozenSetError) as ctx:
            fs.validate(str(crossed), meta, answers=fs.load_answers(str(ANSWERS)))
        self.assertIn("more than one split", str(ctx.exception))

    def test_official_counts_and_pii_are_rejected(self):
        meta = dict(fs.load_meta(str(META)))
        meta["kind"] = fs.OFFICIAL_KIND
        with self.assertRaises(fs.FrozenSetError) as ctx:
            fs.validate(str(CSV), meta, answers=fs.load_answers(str(ANSWERS)), sha_path=str(SHA))
        self.assertIn("official set must have 240", str(ctx.exception))

        rows = fs.load_table(str(CSV))
        rows[0]["problem_text"] += " contact student1@example.com"
        bad = Path(self.tmp) / "pii.csv"
        import csv
        with open(bad, "w", encoding="utf-8", newline="") as fh:
            w = csv.DictWriter(fh, fieldnames=list(rows[0].keys()), lineterminator="\n")
            w.writeheader()
            w.writerows(rows)
        with self.assertRaises(fs.FrozenSetError) as ctx:
            fs.validate(str(bad), fs.load_meta(str(META)), answers=fs.load_answers(str(ANSWERS)))
        self.assertIn("personal data", str(ctx.exception))

    def test_developer_projection_hides_blind_answers(self):
        fs.import_set(str(CSV), str(META), self.tmp, answers_path=str(ANSWERS), sha_path=str(SHA))
        dest, manifest, cases, answers, _ = fs.load_store(self.tmp)
        dev = fs.cases_for_role(cases, answers, "developer")
        ev = fs.cases_for_role(cases, answers, "evaluator")
        secrets = [BLIND_ANSWER]
        self.assertFalse(fs.contains_secret(dev, secrets))
        self.assertFalse(fs.contains_secret(json.loads((Path(dest) / "cases.json").read_text()), secrets))
        blind = next(c for c in ev if c["case_id"] == "FQ-S003")
        self.assertEqual(blind["expected_answer"], BLIND_ANSWER)
        self.assertIsNone(next(c for c in dev if c["case_id"] == "FQ-S003")["expected_answer"])

        dev_report = fs.report(self.tmp, role="developer")
        self.assertFalse(dev_report["counts_toward_admission"])
        self.assertFalse(dev_report["official"])
        self.assertFalse(fs.contains_secret(dev_report, secrets))
        ev_report = fs.report(self.tmp, role="evaluator")
        self.assertEqual(ev_report["failures"], 0)
        self.assertEqual(ev_report["blocker_failures"], 0)
        self.assertFalse(ev_report["counts_toward_admission"], "sample must not count as official admission")
        self.assertFalse(ev_report["engineering_set_confused"])
        self.assertEqual(ev_report["kind"], "frozen_admission_report")
        self.assertNotEqual(ev_report["kind"], "engineering_admission_set_not_frozen")

    def test_blocker_dispute_needs_two_reviewers(self):
        fs.import_set(str(CSV), str(META), self.tmp, answers_path=str(ANSWERS), sha_path=str(SHA))
        first = fs.record_dispute(self.tmp, "FQ-S003", "教师甲", "uphold", "几何复核")
        self.assertEqual(first["status"], "needs_second_reviewer")
        second = fs.record_dispute(self.tmp, "FQ-S003", "教师丙", "uphold", "独立复核")
        self.assertEqual(second["status"], "dual_reviewed")

    def test_sample_overlay_does_not_change_catalog(self):
        fs.import_set(str(CSV), str(META), self.tmp, answers_path=str(ANSWERS), sha_path=str(SHA))
        dest, manifest, cases, answers, _ = fs.load_store(self.tmp)
        overlay = fs.build_overlay(manifest, {"SOLID-ANGLE-02": "admitted"}, catalog.CATALOG_VERSION)
        self.assertFalse(overlay["signed"])
        self.assertEqual(overlay["source_kind"], fs.SAMPLE_KIND)
        path = Path(self.tmp) / "overlay.json"
        fs.write_json(str(path), overlay)
        os.environ["MATH_CATALOG_ADMISSION_OVERLAY"] = str(path)
        self.assertEqual(catalog.family("SOLID-ANGLE-02")["admission"], "dev")
        self.assertEqual(catalog.catalog()["admission_overlay"]["error"], "not_official_or_unsigned")

    def test_signed_official_overlay_can_mark_admitted(self):
        path = Path(self.tmp) / "official-overlay.json"
        fs.write_json(str(path), {
            "kind": "catalog_admission_overlay",
            "catalog_version": catalog.CATALOG_VERSION,
            "frozen_set_version": "FR-Q01-test-only",
            "signed": True,
            "source_kind": fs.OFFICIAL_KIND,
            "families": {"SOLID-ANGLE-02": "admitted"},
        })
        os.environ["MATH_CATALOG_ADMISSION_OVERLAY"] = str(path)
        self.assertEqual(catalog.family("SOLID-ANGLE-02")["admission"], "admitted")
        self.assertEqual(catalog.family("SOLID-VOL-01")["admission"], "dev")
        # Default without the env (already tested in test_server) stays dev when unset —
        # here we only prove the switch works for a synthetic official artifact in tmp.

    def test_signed_official_ranges_tighten_limits(self):
        path = Path(self.tmp) / "ranges.json"
        fs.write_json(str(path), {
            "kind": "frozen_param_ranges",
            "frozen_set_version": "FR-Q01-test-only",
            "signed": True,
            "source_kind": fs.OFFICIAL_KIND,
            "families": {"SOLID-ANGLE-02": {"a": {"max": "50"}, "h": {"max": "50"}}},
        })
        mod = REGISTRY["SOLID-ANGLE-02"]
        def norm(a):
            return mod.normalize({"family_id": "SOLID-ANGLE-02", "input_state": "sufficient", "unit": "", "params": {"a": a, "h": "1"}})

        norm("80")  # default max 100, allowed
        os.environ["MATH_FROZEN_PARAM_RANGES"] = str(path)
        with self.assertRaises(MathError) as ctx:
            norm("80")
        self.assertEqual(ctx.exception.code, "PARAMETER_LIMIT")
        norm("2")  # still inside the frozen cap

        # Unsigned / sample ranges are ignored.
        unsigned = Path(self.tmp) / "unsigned-ranges.json"
        data = json.loads(path.read_text())
        data["signed"] = False
        data["source_kind"] = fs.SAMPLE_KIND
        unsigned.write_text(json.dumps(data), encoding="utf-8")
        os.environ["MATH_FROZEN_PARAM_RANGES"] = str(unsigned)
        norm("80")

    def test_engineering_kind_cannot_be_imported_as_official_meta(self):
        bad = Path(self.tmp) / "bad.meta.json"
        bad.write_text(json.dumps({"kind": "engineering_admission_set_not_frozen", "version": "x"}), encoding="utf-8")
        with self.assertRaises(fs.FrozenSetError):
            fs.load_meta(str(bad))


if __name__ == "__main__":
    unittest.main()
