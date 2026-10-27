# -*- coding: utf-8 -*-
"""Text intake (DEV-031): turn a typed problem statement into a structured draft.

The whole statement must match one family's fixed grammar (object, conditions,
target, optional unit and method). Nothing is matched by text similarity and no
part of the text is evaluated: numbers go through the restricted parser of the
family's own normalize step. Any text left over, a renamed figure, the left
focus, a translation or rotation, or an unsupported method is refused with a
reason, so the teacher uses the structured form or writes the lesson by hand.
The draft is always needs_confirmation; only a teacher's confirmation admits it.
"""

import re
import unicodedata

from .errors import MathError
from .families import REGISTRY as FAMILIES

MAX_TEXT = 600
NUM = r"(\d+\*?√\d+(?:/\d+)?|√\d+(?:/\d+)?|\d+(?:/\d+)?|\d+\.\d+)"
UNIT = r"(mm|cm|dm|m)?"
UNIT_WORDS = (("毫米", "mm"), ("厘米", "cm"), ("分米", "dm"), ("米", "m"))
IS = r"(?:为|是|=)"
METHOD_WORDS = {
    "向量法": "vector_coordinate", "空间向量法": "vector_coordinate", "空间向量": "vector_coordinate", "坐标法": "vector_coordinate",
    "建系法": "vector_coordinate", "综合法": "synthetic", "几何法": "synthetic", "韦达定理": "vieta", "韦达": "vieta",
    "联立方程": "vieta", "代入法": "point_substitution", "点代入": "point_substitution", "点差法": "point_difference",
    "参数方程": "parametric", "极坐标": "polar",
}
METHOD_LABEL = {"formula": "公式法", "vector_coordinate": "向量坐标法", "synthetic": "综合法", "vieta": "联立与韦达定理", "point_substitution": "点代入"}
_METHOD = re.compile(r",?(?:请|要求)?(?:用|利用|使用|运用)(" + "|".join(sorted(METHOD_WORDS, key=len, reverse=True)) +
                     r")(?:求解|解答|来解)?")

# Recognised but not independently admitted (constraint: no admission by resemblance).
_REFUSE = (
    (re.compile(r"左焦点"), "UNSUPPORTED_FAMILY", "左焦点不在题族内（题族只覆盖右焦点）"),
    (re.compile(r"左顶点为P|P为(?:椭圆的)?左顶点"), "UNSUPPORTED_FAMILY", "左顶点不在题族内（题族只覆盖右顶点）"),
    (re.compile(r"平移|旋转|对称变换"), "UNSUPPORTED_FAMILY", "含平移或旋转的题目尚未单独准入"),
    (re.compile(r"斜棱柱|斜四棱柱|平行六面体"), "UNSUPPORTED_FAMILY", "斜棱柱与平行六面体不在题族内"),
    (re.compile(r"x\^2=\d|x²=|焦点在y轴"), "UNSUPPORTED_FAMILY", "开口向上或向下的抛物线不在题族内（题族约定 y²=2px）"),
)

# Object keyword -> (family, expected figure naming); used only to explain a refusal.
_OBJECTS = (
    ("正方体", "SOLID-ANGLE-01", "ABCD-A1B1C1D1"), ("正四棱锥", "SOLID-ANGLE-02", "P-ABCD"),
    ("长方体", "SOLID-VOL-01", ""), ("椭圆", "CONIC", ""), ("抛物线", "CONIC-PARABOLA-01", ""),
)

S = ",?"
PATTERNS = {
    "SOLID-ANGLE-01": re.compile(
        r"(?:已知|在)?正方体ABCD-A1B1C1D1(?:中)?,?(?:的)?棱长" + IS + NUM + UNIT + S +
        r"求(?:直线)?A1C与(?:底面|平面)ABCD所成(?:的)?角的正弦(?:值)?"),
    "SOLID-ANGLE-02": re.compile(
        r"(?:已知|在)?正四棱锥P-ABCD(?:中)?,?(?:的)?底面边长" + IS + NUM + UNIT + ",?高" + IS + NUM + UNIT + S +
        r"E" + IS + r"PC的中点" + S + r"求(?:直线)?BE与平面PAC所成(?:的)?角的正弦(?:值)?"),
    "SOLID-VOL-01": re.compile(
        r"(?:已知|一个)?长方体(?:ABCD-A1B1C1D1)?(?:的)?,?长" + IS + NUM + UNIT + ",?宽" + IS + NUM + UNIT + ",?高" + IS + NUM + UNIT + S +
        r"求(?:它的|该长方体的|长方体的|这个长方体的)?体积(?:\((?:单位)?:?" + UNIT + r"(?:\^3)?\)|,?(?:用|以)" + UNIT + r"(?:\^3)?(?:为单位|表示))?"),
    # Same family in its own canonical wording (edges named AB, AD, AA1).
    "SOLID-VOL-01/edges": re.compile(
        r"长方体ABCD-A1B1C1D1(?:中)?,?AB" + IS + NUM + UNIT + ",?AD" + IS + NUM + UNIT + ",?AA1" + IS + NUM + UNIT + S +
        r"求(?:长方体的)?体积(?:\((?:结果)?(?:用|以)" + UNIT + r"(?:\^3)?(?:表示|为单位)\)|\((?:单位)?:?" + UNIT + r"(?:\^3)?\))?"),
    "CONIC-CHORD-01": re.compile(
        r"已知椭圆x\^2/" + NUM + r"\+y\^2/" + NUM + r"=1,?F" + IS + r"(?:其|椭圆的)?右焦点" + S +
        r"过F的直线与椭圆交于A、B两点" + S + r"求弦长\|AB\|的取值范围(?:,?并指出取到最大值和最小值时弦的位置)?"),
    "CONIC-SLOPE-01": re.compile(
        r"已知椭圆x\^2/" + NUM + r"\+y\^2/" + NUM + r"=1,?P" + IS + r"(?:其|椭圆的)?右顶点" + S +
        r"A、B" + IS + r"椭圆上关于原点对称的两点" + S + r"(?:且)?A、B都不是左右顶点" + S + r"求直线PA与PB的斜率之积"),
    "CONIC-PARABOLA-01": re.compile(
        r"已知抛物线y\^2=" + NUM + r"x,?O" + IS + r"坐标原点" + S + r"过(?:其)?焦点F的直线与抛物线交于A、B两点" + S +
        r"求(?:向量OA与OB的数量积|OA·OB|向量OA·向量OB)"),
}
TARGET_TEXT = {
    "SOLID-ANGLE-01": "直线 A1C 与底面 ABCD 所成角的正弦值",
    "SOLID-ANGLE-02": "直线 BE 与平面 PAC 所成角的正弦值",
    "SOLID-VOL-01": "长方体的体积",
    "CONIC-CHORD-01": "焦点弦长 |AB| 的取值范围",
    "CONIC-SLOPE-01": "直线 PA 与 PB 的斜率之积",
    "CONIC-PARABOLA-01": "向量 OA 与 OB 的数量积",
}


def _refuse(code, reason):
    """reason is shown to the teacher (Chinese); next_action follows the shared code table."""
    return MathError(code, reason, {"reason": reason})


def clean(text):
    if not isinstance(text, str) or not text.strip():
        raise _refuse("INPUT_AMBIGUOUS", "题目文字为空")
    if len(text) > MAX_TEXT:
        raise _refuse("PARAMETER_LIMIT", "题目文字超过 %d 字" % MAX_TEXT)
    if re.search(r"[<>\x00-\x08\x0b\x0c\x0e-\x1f\x7f]", text):
        raise _refuse("INPUT_AMBIGUOUS", "题目文字含尖括号或控制字符")
    t = text.replace("²", "^2").replace("³", "^3")
    t = unicodedata.normalize("NFKC", t)
    t = re.sub(r"\s+", "", t)
    t = t.replace("−", "-").replace("–", "-").replace("—", "-").replace("⋅", "·").replace("•", "·")
    t = t.replace("\\cdot", "·").replace("sqrt", "√").replace("√(", "√")
    t = re.sub(r"√(\d+)\)", r"√\1", t)
    for w, u in UNIT_WORDS:
        t = t.replace(w, u)
    t = re.sub(r"[;。.](?!\d)", ",", t)
    t = re.sub(r",+", ",", t).strip(",")
    return t


def _num(s):
    m = re.fullmatch(r"(?:(\d+)\*?)?√(\d+)(/\d+)?", s)
    if m:
        k, r, d = m.groups()
        return (k + "*" if k else "") + "sqrt(%s)" % r + (d or "")
    return s


def _root(den):
    """Semi-axis from a denominator a² (integer only)."""
    if not re.fullmatch(r"\d+", den):
        raise _refuse("INPUT_AMBIGUOUS", "椭圆方程的分母需为正整数，请用结构化表单填写")
    n = int(den)
    r = int(round(n ** 0.5))
    return str(r) if r * r == n else "sqrt(%d)" % n


def _structured(fid, g):
    base = {"family_id": fid, "input_state": "sufficient"}
    if fid == "SOLID-ANGLE-01":
        return dict(base, unit=g[1] or "", params={"e": _num(g[0])}), ["棱长 %s%s" % (g[0], g[1] or "")]
    if fid == "SOLID-ANGLE-02":
        if (g[1] or "") != (g[3] or ""):
            raise _refuse("CONDITION_CONFLICT", "底面边长与高的单位不同，请统一单位后再识别")
        return dict(base, unit=g[1] or "", params={"a": _num(g[0]), "h": _num(g[2])}), [
            "底面边长 %s%s" % (g[0], g[1] or ""), "高 %s%s" % (g[2], g[3] or ""), "E 为 PC 的中点"]
    if fid == "SOLID-VOL-01":
        units = {"l": g[1] or "", "w": g[3] or "", "h": g[5] or ""}
        s = dict(base, params={"l": _num(g[0]), "w": _num(g[2]), "h": _num(g[4])}, units=units)
        tu = g[6] or g[7]
        if tu:
            s["target_unit"] = tu
        return s, ["长 %s%s" % (g[0], units["l"]), "宽 %s%s" % (g[2], units["w"]), "高 %s%s" % (g[4], units["h"])]
    if fid in ("CONIC-CHORD-01", "CONIC-SLOPE-01"):
        a, b = _root(g[0]), _root(g[1])
        cond = ["椭圆 x²/%s + y²/%s = 1" % (g[0], g[1])]
        cond.append("F 为右焦点，过 F 的直线交椭圆于 A、B" if fid == "CONIC-CHORD-01" else "P 为右顶点，A、B 关于原点对称且都不是左右顶点")
        return dict(base, params={"a": a, "b": b}), cond
    if fid == "CONIC-PARABOLA-01":
        if not re.fullmatch(r"\d+", g[0]):
            raise _refuse("INPUT_AMBIGUOUS", "抛物线 y²=2px 中的系数需为正整数，请用结构化表单填写")
        c = int(g[0])
        p = str(c // 2) if c % 2 == 0 else "%d/2" % c
        return dict(base, params={"p": p}), ["抛物线 y² = %sx" % g[0], "O 为坐标原点，过焦点 F 的直线交抛物线于 A、B"]
    raise _refuse("UNSUPPORTED_FAMILY", "no grammar")


def _explain(t):
    for word, fid, naming in _OBJECTS:
        if word in t:
            if naming and naming not in t:
                return _refuse("UNSUPPORTED_FAMILY", "图形字母与题族约定不同（题族约定 %s），改名的题目尚未单独准入" % naming)
            return _refuse("INPUT_AMBIGUOUS", "识别到“%s”，但条件或所求与题族的固定表述不完全一致；请用结构化表单填写，或人工编写讲解" % word)
    return _refuse("UNSUPPORTED_FAMILY", "不属于已支持的题族")


def normalize_text(payload):
    t = clean(payload.get("text"))
    for rx, code, reason in _REFUSE:
        if rx.search(t):
            raise _refuse(code, reason)
    method = None
    m = _METHOD.search(t)
    if m:
        method = METHOD_WORDS[m.group(1)]
        t = (t[:m.start()] + "," + t[m.end():]).strip(",")
        t = re.sub(r",+", ",", t)
        if _METHOD.search(t):
            raise _refuse("INPUT_AMBIGUOUS", "题目中指定了多种方法")
    hits = [(fid, rx.fullmatch(t)) for fid, rx in PATTERNS.items()]
    hits = [(fid, mm) for fid, mm in hits if mm]
    if len(hits) != 1:
        raise _explain(t)
    fid, mm = hits[0]
    fid = fid.split("/")[0]
    structured, conditions = _structured(fid, mm.groups())
    if method:
        structured["method_profile"] = {"allowed": [method], "preferred": method}
    mod = FAMILIES[fid]
    norm = mod.normalize(structured)  # the family's own admission checks (limits, units, methods)
    unit = norm.get("target_unit", norm.get("unit", ""))
    return {
        "state": "needs_confirmation",
        "family_id": fid,
        "structured": structured,
        "summary": {
            "object": mod.TITLE,
            "conditions": conditions,
            "target": TARGET_TEXT[fid],
            "unit": unit,
            "method": METHOD_LABEL.get(norm["method"], norm["method"]),
            "method_stated": bool(method),
        },
        "problem_text": mod.problem_text(norm),
    }
