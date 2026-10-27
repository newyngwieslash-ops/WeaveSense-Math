# -*- coding: utf-8 -*-
"""Teacher written lessons (DEV-039, FR-M05).

The fallback when a problem is outside the implemented families
(UNSUPPORTED_FAMILY, next action manual_authoring). The teacher writes the
statement, the steps and the answer; the service only checks the shape, builds
an edu.lesson/1 document and the same student projection and previews as an
automatic lesson.

Nothing here is a math check. The lesson is marked manual_unverified
everywhere, carries no exact answer and no self-test, so the platform cannot
grade it automatically or derive variants from it (constraint 4)."""

import re

from . import catalog
from .errors import MathError
from .lesson import COMPOSER_VERSION, SCHEMA, canonical_hash

GENERATOR = "teacher-manual"
VERIFICATION = "manual_unverified"
LIMITS = {"title": 60, "problem_text": 600, "beats": 12, "beat_title": 40, "lines": 8, "line": 200, "answer": 80, "hints": 5, "hint": 200}
_CTRL = re.compile(r"[\x00-\x08\x0b-\x1f\x7f<>]")


def _text(v, limit, what):
    if not isinstance(v, str) or not v.strip():
        raise MathError("INPUT_AMBIGUOUS", "%s is required" % what)
    v = v.strip()
    if len(v) > limit:
        raise MathError("PARAMETER_LIMIT", "%s is longer than %d characters" % (what, limit))
    if _CTRL.search(v):
        raise MathError("INPUT_AMBIGUOUS", "%s contains markup or control characters" % what)
    return v


def _list(v, lo, hi, what):
    if not isinstance(v, list) or len(v) < lo:
        raise MathError("INPUT_AMBIGUOUS", "%s needs at least %d entries" % (what, lo))
    if len(v) > hi:
        raise MathError("PARAMETER_LIMIT", "%s allows at most %d entries" % (what, hi))
    return v


def compose(problem, manual, refs):
    if not isinstance(manual, dict):
        raise MathError("INPUT_AMBIGUOUS", "manual lesson content is required")
    fam = problem.get("family_id") or ""
    beats = []
    for i, b in enumerate(_list(manual.get("beats"), 1, LIMITS["beats"], "beats")):
        if not isinstance(b, dict):
            raise MathError("INPUT_AMBIGUOUS", "beat %d is not an object" % (i + 1))
        lines = [_text(t, LIMITS["line"], "beat %d line" % (i + 1)) for t in _list(b.get("lines"), 1, LIMITS["lines"], "beat %d lines" % (i + 1))]
        beats.append({"id": "m%d" % (i + 1), "title": _text(b.get("title"), LIMITS["beat_title"], "beat %d title" % (i + 1)),
                      "board": lines, "board_latex": [], "say": "".join(lines), "figure_focus": [],
                      "reveals_answer": b.get("reveals_answer") is True, "solution_step_ref": ""})
    if not any(b["reveals_answer"] for b in beats):
        raise MathError("INPUT_AMBIGUOUS", "mark the beat that reveals the answer")
    hints = [{"level": i + 1, "kind": "teacher", "text": _text(h, LIMITS["hint"], "hint %d" % (i + 1)), "reveals_answer": False, "gated": False}
             for i, h in enumerate(_list(manual.get("hints", []), 0, LIMITS["hints"], "hints"))]
    answer = _text((manual.get("answer") or {}).get("plain") if isinstance(manual.get("answer"), dict) else None, LIMITS["answer"], "answer")
    lesson = {
        "schema_version": SCHEMA, "composer_version": COMPOSER_VERSION,
        "meta": {"family_id": fam, "title": _text(manual.get("title"), LIMITS["title"], "title"), "language": "zh-CN", "style": "standard",
                 "ai_generated": False, "generator": GENERATOR, "verification": VERIFICATION, "refs": dict(refs, catalog_version=catalog.CATALOG_VERSION)},
        "problem": {"text": _text(manual.get("problem_text"), LIMITS["problem_text"], "problem_text"), "params": {}, "unit": ""},
        "figure": {"kind": "none", "points": {}, "edges": [], "aux": [], "faces": []},
        "beats": beats,
        # No exact value: nothing downstream may treat the teacher's answer as a checked one.
        "answer": {"plain": answer, "latex": "", "exact": "", "release": "after_teacher_release"},
        "hints": hints,
        "check_question": None,
        "check_answer_key": {},
        "quality": {"math": VERIFICATION, "pedagogy": "pending_teacher_review", "presentation": "auto_checked"},
    }
    return {"lesson": lesson, "lesson_hash": canonical_hash(lesson)}
