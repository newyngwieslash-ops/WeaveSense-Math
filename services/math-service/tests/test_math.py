# -*- coding: utf-8 -*-
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
        self.assertEqual(les["beats"], base["beats"])
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
