# -*- coding: utf-8 -*-
import html as _html
import unittest

import sympy as sp

from mathsvc import lesson as L
from mathsvc.errors import MathError
from mathsvc.families import solid_angle_02 as F
from mathsvc.params import ParamError, parse_exact


def P(a, h, **kw):
    d = {"family_id": "SOLID-ANGLE-02", "params": {"a": a, "h": h}, "unit": "cm"}
    d.update(kw)
    return d


class ParamTests(unittest.TestCase):
    def test_accepts_exact_forms(self):
        self.assertEqual(parse_exact("3/2"), sp.Rational(3, 2))
        self.assertEqual(parse_exact("2*sqrt(3)/3"), 2 * sp.sqrt(3) / 3)
        self.assertEqual(parse_exact("√2"), sp.sqrt(2))
        self.assertEqual(parse_exact(5), 5)

    def test_rejects_code_and_unbounded_input(self):
        for bad in ["__import__('os').system('id')", "2**1000", "sqrt(sqrt(2))", "1.5", "x", "1" * 60,
                    "sqrt(-1)", "1/0", "9999999", "lambda: 1", True, None, 1.5]:
            with self.assertRaises(ParamError, msg=repr(bad)):
                parse_exact(bad)


class FamilyTests(unittest.TestCase):
    GOLD = [("2", "1", 2 * sp.sqrt(22) / 11), ("2", "2", 2 * sp.sqrt(7) / 7), ("1", "1", 2 * sp.sqrt(7) / 7),
            ("sqrt(2)", "1", sp.sqrt(6) / 3), ("3/2", "1/2", 6 * sp.sqrt(47) / 47)]

    def test_gold_answers_and_layers(self):
        for a, h, want in self.GOLD:
            n = F.normalize(P(a, h))
            s = F.solve(n)
            self.assertEqual(sp.simplify(sp.sympify(s["answer"]["exact"]) - want), 0, (a, h))
            r = F.verify(n, s)
            self.assertEqual(r["math_state"], "verified")
            kinds = {c["id"]: c["kind"] for c in r["checks"]}
            self.assertEqual(kinds["numeric_sampling"], "sampling")
            self.assertIn("symbolic_domain_identity", r["evidence_levels"])

    def test_synthetic_method(self):
        n = F.normalize(P("2", "1", method_profile={"allowed": ["synthetic"]}))
        s = F.solve(n)
        self.assertEqual(s["method"], "synthetic")
        self.assertEqual(s["answer"]["plain"], "2√22/11")

    def test_rejections(self):
        cases = [(P("-1", "1"), "CONDITION_CONFLICT"), (P("101", "1"), "PARAMETER_LIMIT"),
                 (P("2", "1", unit="inch"), "PARAMETER_LIMIT"), (P("2", "1", method_profile={"allowed": ["calculus"]}), "METHOD_MISMATCH"),
                 (P("2", "1", target="volume"), "UNSUPPORTED_FAMILY"), (P("2", "1", input_state="pending_confirmation"), "INPUT_AMBIGUOUS"),
                 ({"family_id": "SOLID-ANGLE-02", "params": {"a": "2"}}, "INPUT_AMBIGUOUS"),
                 (P("2", "1", naming={"apex": "S"}), "UNSUPPORTED_FAMILY")]
        for inp, code in cases:
            with self.assertRaises(MathError) as cm:
                F.normalize(inp)
            self.assertEqual(cm.exception.code, code, inp)

    def test_lesson_projection_has_no_answer(self):
        n = F.normalize(P("2", "1"))
        s = F.solve(n)
        r = F.verify(n, s)
        cq = F.check_question(n)
        self.assertNotEqual(cq["question"]["params"], n["params"])
        les = L.compose(F.FAMILY_ID, n, F._public(s) if hasattr(F, "_public") else s, r, cq, {})
        stu = L.student_projection(les, "none")
        texts = " ".join(L._strings(stu))
        for m in L.answer_markers(les):
            self.assertNotIn(m, texts)
        self.assertNotIn("check_answer_key", stu)
        self.assertEqual([h["level"] for h in stu["hints"]], [1, 2, 3])

    def test_teaching_params(self):
        """DEV-070: goal, style and target length are recorded in meta only;
        absent params leave the document (and its hash) as before; the math,
        beats and student projection are the same."""
        n = F.normalize(P("2", "1"))
        s = F.solve(n)
        r = F.verify(n, s)
        cq = F.check_question(n)
        base = L.compose(F.FAMILY_ID, n, s, r, cq, {})
        self.assertNotIn("target_minutes", base["meta"])
        self.assertEqual(L.canonical_hash(base), L.canonical_hash(L.compose(F.FAMILY_ID, n, s, r, cq, {}, None)))
        les = L.compose(F.FAMILY_ID, n, s, r, cq, {}, {"goal": "fix_mistake", "style": "detailed", "target_minutes": 6})
        self.assertEqual((les["meta"]["goal"], les["meta"]["style"], les["meta"]["target_minutes"]), ("fix_mistake", "detailed", 6))
        self.assertEqual([(b["id"], b["board"], b["reveals_answer"]) for b in les["beats"]],
                         [(b["id"], b["board"], b["reveals_answer"]) for b in base["beats"]])
        self.assertEqual(les["answer"], base["answer"])
        self.assertNotEqual(L.canonical_hash(les), L.canonical_hash(base))
        stu = L.student_projection(les, "none")
        self.assertNotIn("goal", stu["meta"])
        self.assertNotIn("target_minutes", stu["meta"])
        for bad in ({"goal": "x"}, {"style": "funny"}, {"target_minutes": 2}, {"target_minutes": 9}, {"target_minutes": True},
                    {"target_minutes": "5"}, {"other": 1}, "fast"):
            with self.assertRaises(MathError) as cm:
                L.compose(F.FAMILY_ID, n, s, r, cq, {}, bad)
            self.assertEqual(cm.exception.code, "INPUT_AMBIGUOUS", bad)

    def test_style_and_goal_change_wording_only(self):
        """DEV-070: concise narrates each step's result line, detailed adds the
        step number and figure focus, goals add fixed framing; boards, answer
        and the student projection's locked beats stay the same, and standard
        style without a goal keeps the narration unchanged."""
        n = F.normalize(P("2", "1"))
        s = F.solve(n)
        r = F.verify(n, s)
        cq = F.check_question(n)
        mk = lambda **t: L.compose(F.FAMILY_ID, n, s, r, cq, {}, dict({"target_minutes": 5}, **t))
        base = L.compose(F.FAMILY_ID, n, s, r, cq, {})
        std = mk(style="standard")
        self.assertEqual([b["say"] for b in std["beats"]], [b["say"] for b in base["beats"]])
        con, det = mk(style="concise"), mk(style="detailed")
        for b0, bc, bd in zip(base["beats"], con["beats"], det["beats"]):
            self.assertEqual(bc["say"], b0["board"][-1])
            self.assertTrue(bd["say"].startswith("第") and b0["title"] in bd["say"] and "".join(b0["board"]) in bd["say"])
            self.assertEqual((bc["board"], bd["board"]), (b0["board"], b0["board"]))
        self.assertLess(sum(len(b["say"]) for b in con["beats"]), sum(len(b["say"]) for b in base["beats"]))
        self.assertGreater(sum(len(b["say"]) for b in det["beats"]), sum(len(b["say"]) for b in base["beats"]))
        um = mk(goal="understand_method")
        self.assertTrue(um["beats"][0]["say"].startswith("整体思路：" + base["beats"][0]["title"]))
        pr = mk(goal="practice")
        for b0, b in zip(base["beats"], pr["beats"]):
            self.assertEqual(b["say"].endswith(L.GOAL_SUFFIX["practice"]), not b0["reveals_answer"])
        for les in (con, det, um, pr, mk(goal="fix_mistake", style="concise")):
            self.assertEqual(les["answer"], base["answer"])
            stu = L.student_projection(les, "none")
            self.assertTrue(all(b.get("locked") and "say" not in b for b in stu["beats"]))

    def test_provenance_in_every_format(self):
        """DEV-101: the student JSON and both preview pages carry the same
        provenance, explicitly (visible text) and implicitly (a meta tag); the
        label follows what the lesson records, a teacher edit adds to it, and
        the "no generative AI" wording needs an explicit ai_generated=False."""
        import json as _json
        import re as _re
        from mathsvc import render as R
        n = F.normalize(P("2", "1"))
        s = F.solve(n)
        les = L.compose(F.FAMILY_ID, n, s, F.verify(n, s), F.check_question(n), {})
        stu = L.student_projection(les, "none")
        want = {"kind": "template_only", "ai_generated": False, "generator": "deterministic-template", "ai_parts": [],
                "teacher_edited": False, "label": "讲解由程序按题族模板生成，平台未调用生成式人工智能撰写内容"}
        self.assertEqual(stu["meta"]["provenance"], want)
        for page in (R.page(les, "teacher"), R.page(stu, "student")):
            self.assertIn('<p class="prov" data-provenance>' + want["label"] + "</p>", page)
            m = _re.search(r'<meta name="content-provenance" content="([^"]*)">', page)
            self.assertEqual(_json.loads(_html.unescape(m.group(1))), want)
        edited = dict(les["meta"], edit_log=[{"class": "wording"}])
        self.assertEqual(L.provenance(edited)["label"], want["label"] + "；含老师修改")
        manual = L.provenance({"generator": "teacher-manual", "ai_generated": False})
        self.assertEqual((manual["kind"], manual["label"]), ("teacher_manual", "讲解由老师人工编写，平台未调用生成式人工智能撰写内容"))
        # Any recorded model part overrides the generator; nothing recorded is not "no AI".
        tts = L.provenance({"generator": "deterministic-template", "ai_generated": False, "ai_parts": ["narration", "audio"]})
        self.assertEqual((tts["kind"], tts["ai_generated"], tts["label"]), ("model_assisted", True, "含人工智能生成内容（旁白、配音）"))
        self.assertEqual(L.provenance({"ai_generated": True, "generator": "x"})["label"], "含人工智能生成内容")
        for meta in ({}, {"generator": "deterministic-template"}, {"generator": "other", "ai_generated": False}):
            p = L.provenance(meta)
            self.assertEqual((p["kind"], p["ai_generated"], p["label"]), ("unknown", None, "生成方式未记录"), meta)
            self.assertNotIn("未调用生成式人工智能", p["label"])

    def test_guard_catches_leak(self):
        with self.assertRaises(MathError):
            L.guard({"x": ["答案 2√22/11"]}, ["2√22/11"])

    def test_failed_verification_blocks_lesson(self):
        n = F.normalize(P("2", "1"))
        s = F.solve(n)
        r = F.verify(n, s)
        r["math_state"] = "failed"
        with self.assertRaises(MathError) as cm:
            L.compose(F.FAMILY_ID, n, s, r, F.check_question(n), {})
        self.assertEqual(cm.exception.code, "VALIDATION_FAILED")


if __name__ == "__main__":
    unittest.main()
