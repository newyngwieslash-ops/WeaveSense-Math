# -*- coding: utf-8 -*-
"""Lesson schema edu.lesson/1 and the student projection.

Structure inspired by edulab skills/edu-math-video/lesson (meta/scenes/beats/board),
rewritten as a versioned data contract; no executable expressions are allowed
anywhere in a lesson (figures are declared points/segments/faces only).
"""

import hashlib
import json

from .errors import MathError

SCHEMA = "edu.lesson/1"
COMPOSER_VERSION = "lesson-composer/1.0.0"


def canonical_hash(obj):
    raw = json.dumps(obj, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return "sha256:" + hashlib.sha256(raw).hexdigest()


GOALS = ("understand_method", "practice", "fix_mistake")
STYLES = ("concise", "standard", "detailed")


def teaching_params(t):
    """Teacher chosen goal, style and target length (DEV-070). Absent means the
    defaults and leaves the lesson document unchanged. They describe the
    lesson and its pacing; they never change the verified mathematics."""
    if t is None:
        return None
    if not isinstance(t, dict) or set(t) - {"goal", "style", "target_minutes"}:
        raise MathError("INPUT_AMBIGUOUS", "teaching must be {goal, style, target_minutes}")
    goal, style, minutes = t.get("goal", ""), t.get("style", "standard"), t.get("target_minutes", 5)
    if goal not in GOALS + ("",) or style not in STYLES or isinstance(minutes, bool) or not isinstance(minutes, int) or not 3 <= minutes <= 8:
        raise MathError("INPUT_AMBIGUOUS", "teaching goal, style or target_minutes (3 to 8) is not supported")
    return {"goal": goal, "style": style, "target_minutes": minutes}


def compose(fam, norm, sol, report, cq, refs, teaching=None):
    teaching = teaching_params(teaching)
    if report["math_state"] != "verified":
        raise MathError("VALIDATION_FAILED", "lesson requires a verified solution", {"failed": report["failed_checks"]})
    beats = []
    for st in sol["steps"]:
        beats.append({"id": st["id"], "title": st["title"], "board": st.get("plain", []), "board_latex": st.get("latex", []),
                      "say": "".join(st.get("plain", [])), "figure_focus": st.get("uses", []),
                      "reveals_answer": bool(st.get("reveals_answer")), "solution_step_ref": st["id"]})
    mod = _module(fam)
    hints = mod.hints(sol)
    lesson = {
        "schema_version": SCHEMA, "composer_version": COMPOSER_VERSION,
        "meta": {"family_id": fam, "title": mod.TITLE, "language": "zh-CN", "style": "standard",
                 "ai_generated": False, "generator": "deterministic-template", "refs": refs},
        "problem": {"text": mod.problem_text(norm), "params": norm["params"], "unit": norm["unit"]},
        "figure": _figure(mod, norm, sol),
        "beats": beats,
        "answer": {"plain": sol["answer"]["plain"], "latex": sol["answer"]["latex"], "exact": sol["answer"]["exact"],
                   "release": "after_teacher_release"},
        "hints": hints,
        "check_question": cq["question"],
        "check_answer_key": cq["answer_key"],
        "quality": {"math": report["math_state"], "pedagogy": "pending_teacher_review", "presentation": "auto_checked"},
    }
    if teaching:
        lesson["meta"]["style"] = teaching["style"]
        lesson["meta"]["target_minutes"] = teaching["target_minutes"]
        if teaching["goal"]:
            lesson["meta"]["goal"] = teaching["goal"]
    problems = presentation_check(lesson, sol)
    if problems:
        raise MathError("VALIDATION_FAILED", "presentation consistency failed", {"problems": problems})
    return lesson


def _figure(mod, norm, sol):
    fig = mod.figure(norm, sol)
    if hasattr(mod, "explore"):
        fig["explore"] = mod.explore(norm, sol)
    return fig


def _module(fam):
    from .families import REGISTRY
    mod = REGISTRY.get(fam)
    if mod is None:
        raise MathError("UNSUPPORTED_FAMILY", "unknown problem family")
    return mod


def presentation_check(lesson, sol):
    """Automated presentation checks: labels exist, every beat has board + narration, answer consistent."""
    problems = []
    pts = lesson["figure"]["points"]
    for e in lesson["figure"]["edges"] + [a["seg"] for a in lesson["figure"]["aux"]]:
        for p in e:
            if p not in pts:
                problems.append("figure references unknown point %s" % p)
    for b in lesson["beats"]:
        if not b["board"] or not b["say"]:
            problems.append("beat %s missing board or narration" % b["id"])
    if lesson["answer"]["exact"] != sol["answer"]["exact"]:
        problems.append("answer differs from solution")
    if not any(b["reveals_answer"] for b in lesson["beats"]):
        problems.append("no beat is marked as revealing the answer")
    return problems


def answer_markers(lesson):
    a = lesson["answer"]
    k = lesson.get("check_answer_key") or {}
    out = {a["plain"], a["latex"], a["exact"], k.get("plain", ""), k.get("latex", ""), k.get("exact", "")}
    public = public_givens(lesson)
    return sorted(m for m in out if m and len(m) >= 2 and not any(m in g for g in public))


def public_givens(lesson):
    """Strings every student sees anyway (problem statement, given parameters,
    the self-test prompt). A marker that already occurs there carries no
    answer information, so it cannot be used to detect a leak."""
    cq = lesson.get("check_question") or {}
    out = [lesson["problem"]["text"], cq.get("prompt", "")]
    out += [str(v) for v in lesson["problem"].get("params", {}).values()]
    out += [str(v) for v in (cq.get("params") or {}).values()]
    return [g for g in out if g]


def visible_text(page_html):
    """Human-visible text of a rendered page: style blocks and SVG geometry are
    dropped (only SVG <text> labels are kept), tags removed, entities decoded."""
    import html as _html
    import re
    s = re.sub(r"<style.*?</style>", " ", page_html, flags=re.S)
    s = re.sub(r"<svg.*?</svg>", lambda m: " ".join(re.findall(r"<text[^>]*>([^<]*)</text>", m.group(0))), s, flags=re.S)
    s = re.sub(r"<[^>]+>", " ", s)
    return _html.unescape(s)


PROVENANCE_LABEL = {
    "deterministic-template": "由程序按题族模板自动生成，未使用生成式人工智能",
    "teacher-manual": "老师人工编写，未使用生成式人工智能",
}


def provenance(meta):
    """DEV-101: how the content was produced. It is derived from the lesson's own
    metadata (never from an edit), so review and teacher edits cannot remove it;
    every format (student JSON, preview HTML, export) carries the same object."""
    ai = bool(meta.get("ai_generated"))
    gen = str(meta.get("generator") or "unknown")
    edited = bool(meta.get("edit_log"))
    label = "含人工智能生成内容" if ai else PROVENANCE_LABEL.get(gen, "生成方式未记录")
    if edited:
        label += "；含老师修改"
    return {"ai_generated": ai, "generator": gen, "teacher_edited": edited, "label": label}


def student_projection(lesson, reveal="none"):
    """Server-side projection for students (PRD constraint 2). reveal=none keeps
    only the problem, the unlabeled figure, and ungated hints; nothing that
    reveals the answer or the check-question key is included."""
    if reveal not in ("none", "released"):
        raise MathError("POLICY_REVOKED", "unknown reveal policy")
    out = {
        "schema_version": SCHEMA, "projection": "student", "reveal": reveal,
        "meta": dict({k: lesson["meta"][k] for k in ("family_id", "title", "language", "style")},
                     **({"layout": lesson["meta"]["layout"]} if lesson["meta"].get("layout") else {}),
                     **({"verification": lesson["meta"]["verification"]} if lesson["meta"].get("verification") else {}),
                     provenance=provenance(lesson["meta"])),
        "problem": lesson["problem"], "figure": lesson["figure"],
        "hints": [{"level": h["level"], "kind": h["kind"], "text": h["text"]} for h in lesson["hints"] if not h.get("gated")],
        "gated_hint_levels": [h["level"] for h in lesson["hints"] if h.get("gated")],
        "check_question": lesson["check_question"],
    }
    if reveal == "released":
        out["beats"] = lesson["beats"]
        out["answer"] = lesson["answer"]
    else:
        out["beats"] = [{"id": b["id"], "title": b["title"], "locked": True} for b in lesson["beats"]]
    if reveal == "none":
        guard(out, answer_markers(lesson))
    else:
        k = lesson.get("check_answer_key") or {}
        pub = public_givens(lesson)
        guard(out, [m for m in (k.get("plain", ""), k.get("exact", "")) if m and not any(m in g for g in pub)])
    return out


def _strings(obj):
    if isinstance(obj, str):
        yield obj
    elif isinstance(obj, dict):
        for k, v in obj.items():
            yield str(k)
            yield from _strings(v)
    elif isinstance(obj, (list, tuple)):
        for v in obj:
            yield from _strings(v)


def guard(obj, markers):
    texts = list(_strings(obj))
    leaked = [m for m in markers if m and any(m in t for t in texts)]
    if leaked:
        raise MathError("VALIDATION_FAILED", "student projection would leak answer content", {"markers": len(leaked)})
