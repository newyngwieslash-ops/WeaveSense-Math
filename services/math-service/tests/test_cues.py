# -*- coding: utf-8 -*-
"""DEV-071: narration anchors and board/figure actions are part of the lesson
schema (edu.lesson/2); edu.lesson/1 documents stay readable and upgrade on edit."""
import copy
import unittest

from mathsvc import edits as E
from mathsvc import lesson as L
from mathsvc import manual as M
from tests.test_families import GOLD, run_all


def strip_to_v1(les):
    old = copy.deepcopy(les)
    old["schema_version"] = "edu.lesson/1"
    for k in ("cue_schema", "cue_timing"):
        old.pop(k, None)
    for b in old["beats"]:
        b.pop("cues", None)
    return old


class Cues(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.runs = {fid: run_all(cases[0][0]) for fid, cases in GOLD.items()}

    def test_every_family_has_consistent_cues(self):
        for fid, (prior, lp) in self.runs.items():
            les = lp["compose_lesson"]["lesson"]
            self.assertEqual((les["schema_version"], les["cue_schema"], les["cue_timing"]), ("edu.lesson/2", "edu.cues/1", "text_order"), fid)
            self.assertEqual(L.cue_problems(les), [], fid)
            anchors = [c["anchor"] for b in les["beats"] for c in b["cues"]]
            self.assertEqual(len(anchors), len(set(anchors)), fid)
            for b in les["beats"]:
                self.assertEqual("".join(c["text"] for c in b["cues"]), b["say"])
                self.assertEqual(b["cues"][-1]["chars"][1], len(b["say"]))
                lines = sorted(a["line"] for c in b["cues"] for a in c["actions"] if a["kind"] == "show_board_line")
                self.assertEqual(lines, list(range(len(b["board"]))))
                if b["figure_focus"]:
                    self.assertEqual(b["cues"][0]["actions"][0], {"kind": "focus_figure", "refs": b["figure_focus"]})
                for c in b["cues"]:
                    self.assertTrue(c["anchor"].startswith(b["id"] + ".n"))
                    self.assertGreaterEqual(c["est_ms"], 800)
                    for a in c["actions"]:
                        self.assertLessEqual(set(a), {"kind", "line", "refs"})  # data only, never code

    def test_student_projection_carries_no_cues_before_release(self):
        for fid, (prior, lp) in self.runs.items():
            les = lp["compose_lesson"]["lesson"]
            stu = L.student_projection(les, "none")
            self.assertTrue(all(set(b) == {"id", "title", "locked"} for b in stu["beats"]), fid)
            rel = L.student_projection(les, "released")
            self.assertTrue(all(b["cues"] for b in rel["beats"]), fid)

    def test_v1_lessons_stay_readable_and_upgrade_on_edit(self):
        fid = "SOLID-ANGLE-01"
        prior, lp = self.runs[fid]
        old = strip_to_v1(lp["compose_lesson"]["lesson"])
        sol = prior["solve"]["solution"]
        self.assertEqual(L.presentation_check(old, sol), [])
        stu = L.student_projection(old, "none")
        self.assertTrue(all(b.get("locked") for b in stu["beats"]))
        self.assertTrue(all("cues" not in b for b in L.student_projection(old, "released")["beats"]))
        b0 = old["beats"][0]
        new = E.apply(old, [{"target": "beat", "id": b0["id"], "field": "say", "value": "先看图。再找垂足；最后连线"}], sol)
        self.assertEqual(new["schema_version"], "edu.lesson/2")
        self.assertEqual(L.cue_problems(new), [])
        self.assertEqual([c["text"] for c in new["beats"][0]["cues"]], ["先看图。", "再找垂足；", "最后连线"])
        self.assertEqual(new["meta"]["parent_lesson_hash"], E.artifact_sha(old))

    def test_board_edit_rebuilds_actions(self):
        prior, lp = self.runs["CONIC-PARABOLA-01"]
        les, sol = lp["compose_lesson"]["lesson"], prior["solve"]["solution"]
        b = les["beats"][0]
        new = E.apply(les, [{"target": "beat", "id": b["id"], "field": "board", "value": ["第一行", "第二行", "第三行", "第四行"]}], sol)
        nb = new["beats"][0]
        self.assertEqual(sorted(a["line"] for c in nb["cues"] for a in c["actions"] if a["kind"] == "show_board_line"), [0, 1, 2, 3])
        self.assertEqual(L.cue_problems(new), [])

    def test_tampered_cues_are_refused(self):
        prior, lp = self.runs["CONIC-CHORD-01"]
        les, sol = lp["compose_lesson"]["lesson"], prior["solve"]["solution"]
        cases = []
        t = copy.deepcopy(les); t["beats"][0]["cues"][0]["text"] += "多"; cases.append(t)
        t = copy.deepcopy(les); t["beats"][0]["cues"][0]["actions"].append({"kind": "run_script", "refs": []}); cases.append(t)
        t = copy.deepcopy(les); t["beats"][0]["cues"][0]["actions"].append({"kind": "focus_figure", "refs": ["Z"]}); cases.append(t)
        t = copy.deepcopy(les); t["beats"][1]["cues"][0]["anchor"] = t["beats"][0]["cues"][0]["anchor"]; cases.append(t)
        t = copy.deepcopy(les); t["beats"][0]["cues"][0]["actions"] = [a for a in t["beats"][0]["cues"][0]["actions"] if a["kind"] != "show_board_line"]; cases.append(t)
        t = copy.deepcopy(les); del t["beats"][0]["cues"]; cases.append(t)
        t = copy.deepcopy(les); t["schema_version"] = "edu.lesson/9"; cases.append(t)
        for i, t in enumerate(cases):
            self.assertNotEqual(L.cue_problems(t), [], i)
            self.assertNotEqual(L.presentation_check(t, sol), [], i)

    def test_manual_lessons_get_cues(self):
        out = M.compose({"family_id": ""}, {"title": "教师讲解", "problem_text": "已知正方体棱长为 2，求体积。",
                                            "beats": [{"title": "公式", "lines": ["体积等于棱长的立方。", "V = 2³"]},
                                                      {"title": "结果", "lines": ["V = 8"], "reveals_answer": True}],
                                            "answer": {"plain": "8"}}, {})
        les = out["lesson"]
        self.assertEqual(L.cue_problems(les), [])
        self.assertEqual(out["lesson_hash"], L.canonical_hash(les))
        self.assertEqual(les["beats"][0]["cues"][0]["anchor"], "m1.n0")


if __name__ == "__main__":
    unittest.main()
