# -*- coding: utf-8 -*-
"""DEV-031 text intake: whole-statement grammar per family, never resemblance."""

import unittest

from mathsvc import pipeline
from mathsvc.errors import MathError
from mathsvc.intake import MAX_TEXT, normalize_text

CASES = {
    "SOLID-ANGLE-01": ("正方体 ABCD-A1B1C1D1 的棱长为 2cm。求直线 A1C 与底面 ABCD 所成角的正弦值。",
                       {"unit": "cm", "params": {"e": "2"}}),
    "SOLID-ANGLE-02": ("正四棱锥 P-ABCD 的底面边长为 2√2 cm，高为 3 cm，E 为 PC 的中点，求直线 BE 与平面 PAC 所成角的正弦值",
                       {"unit": "cm", "params": {"a": "2*sqrt(2)", "h": "3"}}),
    "SOLID-VOL-01": ("长方体的长为3厘米，宽为20毫米，高为5厘米，求它的体积（单位：cm³）",
                     {"params": {"l": "3", "w": "20", "h": "5"}, "units": {"l": "cm", "w": "mm", "h": "cm"}, "target_unit": "cm"}),
    "CONIC-CHORD-01": ("已知椭圆 x²/4 + y²/3 = 1，F 为其右焦点，过 F 的直线与椭圆交于 A、B 两点。求弦长 |AB| 的取值范围。",
                       {"params": {"a": "2", "b": "sqrt(3)"}}),
    "CONIC-SLOPE-01": ("已知椭圆 x²/4 + y²/3 = 1，P 为右顶点，A、B 是椭圆上关于原点对称的两点，且 A、B 都不是左右顶点。求直线 PA 与 PB 的斜率之积。",
                       {"params": {"a": "2", "b": "sqrt(3)"}}),
    "CONIC-PARABOLA-01": ("已知抛物线 y² = 3x，O 为坐标原点，过焦点 F 的直线与抛物线交于 A、B 两点。求 OA·OB",
                          {"params": {"p": "3/2"}}),
}


def err(text):
    with unittest.TestCase().assertRaises(MathError) as cm:
        normalize_text({"text": text})
    return cm.exception


class IntakeTests(unittest.TestCase):
    def test_each_family_statement_becomes_a_draft_needing_confirmation(self):
        for fid, (text, want) in CASES.items():
            with self.subTest(fid):
                d = normalize_text({"text": text})
                self.assertEqual(d["state"], "needs_confirmation")
                self.assertEqual(d["family_id"], fid)
                for k, v in want.items():
                    self.assertEqual(d["structured"][k], v)
                s = d["summary"]
                self.assertTrue(s["object"] and s["conditions"] and s["target"] and s["method"])
                self.assertFalse(s["method_stated"])
                # The draft is admissible by the same validate stage the analyze job runs.
                pipeline.run_stage("validate_input", {"problem": d["structured"]}, {})

    def test_draft_round_trips_through_the_family_statement(self):
        for fid, (text, _) in CASES.items():
            d = normalize_text({"text": text})
            again = normalize_text({"text": d["problem_text"]})
            self.assertEqual(again["structured"], d["structured"], fid)

    def test_stated_method_is_kept_or_refused(self):
        d = normalize_text({"text": "正方体 ABCD-A1B1C1D1 的棱长为 2，用综合法求直线 A1C 与底面 ABCD 所成角的正弦值。"})
        self.assertEqual(d["structured"]["method_profile"], {"allowed": ["synthetic"], "preferred": "synthetic"})
        self.assertTrue(d["summary"]["method_stated"])
        e = err("已知椭圆 x²/4 + y²/3 = 1，P 为右顶点，A、B 是椭圆上关于原点对称的两点，且 A、B 都不是左右顶点。用点差法求直线 PA 与 PB 的斜率之积。")
        self.assertEqual(e.code, "METHOD_MISMATCH")

    def test_conventions_outside_the_family_are_refused_with_a_reason(self):
        cases = {
            "已知椭圆 x²/4 + y²/3 = 1，F 为其左焦点，过 F 的直线与椭圆交于 A、B 两点。求弦长 |AB| 的取值范围。": ("UNSUPPORTED_FAMILY", "左焦点"),
            "正方体 ABCD-EFGH 的棱长为 2。求直线 EC 与底面 ABCD 所成角的正弦值。": ("UNSUPPORTED_FAMILY", "图形字母"),
            "将正方体 ABCD-A1B1C1D1 平移后，棱长为 2，求直线 A1C 与底面 ABCD 所成角的正弦值。": ("UNSUPPORTED_FAMILY", "平移"),
            "已知抛物线 x² = 4y，过焦点 F 的直线与抛物线交于 A、B 两点。求向量 OA 与 OB 的数量积。": ("UNSUPPORTED_FAMILY", "y²=2px"),
            "斜四棱柱的长为 3，宽为 2，高为 5，求体积": ("UNSUPPORTED_FAMILY", "斜棱柱"),
            "求 1+1 的值": ("UNSUPPORTED_FAMILY", "不属于"),
        }
        for text, (code, word) in cases.items():
            with self.subTest(text):
                e = err(text)
                self.assertEqual(e.code, code)
                self.assertIn(word, e.details["reason"])

    def test_extra_conditions_are_not_dropped_to_force_a_match(self):
        # Similar statements with one more condition or another target must not be admitted.
        for text in ("正方体 ABCD-A1B1C1D1 的棱长为 2，M 为 AB 中点，求直线 A1C 与底面 ABCD 所成角的正弦值。",
                     "正方体 ABCD-A1B1C1D1 的棱长为 2，求直线 A1C 与底面 ABCD 所成角的余弦值。",
                     "正方体 ABCD-A1B1C1D1 的棱长为 2，求直线 A1C 与底面 ABCD 所成角的正弦值，并求体积。",
                     "已知椭圆 x²/4 + y²/3 = 1，F 为其右焦点，过 F 的直线与椭圆交于 A、B 两点。求弦长 |AB| 的最大值。"):
            with self.subTest(text):
                self.assertEqual(err(text).code, "INPUT_AMBIGUOUS")

    def test_numbers_go_through_family_limits_and_no_text_is_evaluated(self):
        self.assertEqual(err("正方体 ABCD-A1B1C1D1 的棱长为 0，求直线 A1C 与底面 ABCD 所成角的正弦值。").code, "CONDITION_CONFLICT")
        self.assertEqual(err("正方体 ABCD-A1B1C1D1 的棱长为 1000，求直线 A1C 与底面 ABCD 所成角的正弦值。").code, "PARAMETER_LIMIT")
        self.assertEqual(err("长方体的长为3厘米，宽为20毫米，高为5厘米，求它的体积").code, "INPUT_AMBIGUOUS")
        self.assertEqual(err("正四棱锥 P-ABCD 的底面边长为 2 cm，高为 3 mm，E 为 PC 的中点，求直线 BE 与平面 PAC 所成角的正弦值").code, "CONDITION_CONFLICT")
        for text in ("正方体 ABCD-A1B1C1D1 的棱长为 __import__('os').system('id')，求直线 A1C 与底面 ABCD 所成角的正弦值。",
                     "正方体 ABCD-A1B1C1D1 的棱长为 2**99，求直线 A1C 与底面 ABCD 所成角的正弦值。"):
            self.assertEqual(err(text).code, "INPUT_AMBIGUOUS")

    def test_input_limits(self):
        self.assertEqual(err("").code, "INPUT_AMBIGUOUS")
        self.assertEqual(err(None).code, "INPUT_AMBIGUOUS")
        self.assertEqual(err("正" * (MAX_TEXT + 1)).code, "PARAMETER_LIMIT")
        self.assertEqual(err("<script>正方体</script>").code, "INPUT_AMBIGUOUS")
        self.assertEqual(err("正方体\x07").code, "INPUT_AMBIGUOUS")


if __name__ == "__main__":
    unittest.main()
