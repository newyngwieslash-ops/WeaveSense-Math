# -*- coding: utf-8 -*-
"""Presentation checks on the rendered previews (DEV-082).

Runs as its own stage (check_render) in a fresh isolated subprocess, so the
previews are rebuilt from the lesson and the student projection in a clean
environment and compared byte for byte with what render_preview stored.

The report covers presentation only. It never says anything about the math:
"passed" here means the page is self-contained, reproducible and legible, not
that the solution is correct (constraint 4). Details carry codes and counts
only; no lesson text is copied into the report.
"""

import hashlib
import os
import re
import sys
import tempfile

from . import lesson as L
from . import render

CHECKER_VERSION = "render-check/1.0.0"
SCHEMA = "edu.render_check/1"

# The page is served with CSP default-src 'none'; the only URL allowed is the SVG namespace.
_ALLOWED_URLS = {"http://www.w3.org/2000/svg"}
_URL_RE = re.compile(r"(?i)\b(?:https?|ftp|data|blob|javascript)://[^\s\"'<>)]*|\bjavascript:")
_FETCH_RE = re.compile(r"(?i)<script|<link\b|<iframe|<object|<embed|<img\b|<audio|<video|<source\b|\bsrc\s*=|\bhref\s*=|url\s*\(|@import|@font-face")
_HOST_RE = re.compile(r"(?i)file:/|[a-z]:\\\\|\\\\\\\\|(?<![\w/])/(?:home|users|root|tmp|var|etc|opt|usr|srv|mnt|workspace|private)/")

# Code points covered by the declared CJK system fonts (Noto Sans CJK SC, PingFang SC, Microsoft YaHei).
_GLYPH_RANGES = [(0x20, 0x7E), (0xA0, 0xFF), (0x370, 0x3FF), (0x2000, 0x206F), (0x2070, 0x209F), (0x2100, 0x214F),
                 (0x2150, 0x218F), (0x2190, 0x21FF), (0x2200, 0x22FF), (0x2460, 0x24FF), (0x25A0, 0x25FF),
                 (0x3000, 0x303F), (0x3400, 0x4DBF), (0x4E00, 0x9FFF), (0xFF00, 0xFFEF)]
_SPACE = {"\n", "\t", "\r"}

_LABEL_W, _LABEL_H = 8.5, 14.0  # estimated glyph box of a 14px label (one CJK glyph is about 14px wide)


def _glyph_ok(ch):
    if ch in _SPACE:
        return True
    o = ord(ch)
    return any(a <= o <= b for a, b in _GLYPH_RANGES)


def _check(cid, problems, warn=False, **counts):
    status = "pass" if not problems else ("warn" if warn else "fail")
    out = {"id": cid, "status": status}
    if problems:
        out["count"] = len(problems)
        out["codes"] = sorted(set(problems))[:8]
    out.update(counts)
    return out


def _svgs(page_html):
    for m in re.finditer(r"<svg[^>]*viewBox=\"0 0 (\d+) (\d+)\"[^>]*>(.*?)</svg>", page_html, flags=re.S):
        yield int(m.group(1)), int(m.group(2)), m.group(3)


def _f(s):
    return float(s)


def _geometry(page_html):
    """Overflow and occlusion inside every SVG view: geometry outside the
    viewBox is an error, a label running past the edge or covering another
    label is a warning for the teacher."""
    outside, clipped, overlap, views, curves = [], [], [], 0, 0
    for w, h, body in _svgs(page_html):
        views += 1
        for m in re.finditer(r"<line x1=\"([-\d.]+)\" y1=\"([-\d.]+)\" x2=\"([-\d.]+)\" y2=\"([-\d.]+)\"", body):
            x1, y1, x2, y2 = map(_f, m.groups())
            if min(x1, x2) < -0.5 or min(y1, y2) < -0.5 or max(x1, x2) > w + 0.5 or max(y1, y2) > h + 0.5:
                outside.append("line_outside_view")
        for m in re.finditer(r"<circle cx=\"([-\d.]+)\" cy=\"([-\d.]+)\"", body):
            x, y = map(_f, m.groups())
            if not (0 <= x <= w and 0 <= y <= h):
                outside.append("point_outside_view")
        for m in re.finditer(r"<(polygon|polyline) points=\"([^\"]+)\"", body):
            for pair in m.group(2).split():
                x, y = map(_f, pair.split(","))
                if not (-0.5 <= x <= w + 0.5 and -0.5 <= y <= h + 0.5):
                    # an unbounded curve (parabola, hyperbola branch) is meant to run off the view
                    # and is cut at the edge; a face leaving the view is a layout error
                    if m.group(1) == "polyline":
                        curves += 1
                    else:
                        outside.append("face_outside_view")
                    break
        boxes = []
        for m in re.finditer(r"<text x=\"([-\d.]+)\" y=\"([-\d.]+)\"[^>]*>([^<]*)</text>", body):
            x, y, t = _f(m.group(1)), _f(m.group(2)), m.group(3)
            bw = _LABEL_W * max(1, len(t))
            box = (x, y - _LABEL_H + 3, x + bw, y + 3)
            if box[0] < 0 or box[1] < 0 or box[2] > w or box[3] > h:
                clipped.append("label_clipped")
            for o in boxes:
                if box[0] < o[2] and o[0] < box[2] and box[1] < o[3] and o[1] < box[3]:
                    overlap.append("label_overlap")
                    break
            boxes.append(box)
    return outside, clipped, overlap, views, curves


def _resources(page_html):
    bad = []
    for u in _URL_RE.findall(page_html):
        if u not in _ALLOWED_URLS:
            bad.append("external_url")
    # xmlns="http://www.w3.org/2000/svg" is a namespace, not a fetch; everything else listed here fetches or runs.
    if _FETCH_RE.search(page_html):
        bad.append("fetching_element")
    return bad


def _host_paths(page_html):
    bad = []
    if _HOST_RE.search(page_html):
        bad.append("host_path")
    for p in {os.path.dirname(os.path.dirname(os.path.abspath(__file__))), sys.prefix, tempfile.gettempdir(), os.path.expanduser("~")}:
        if p and len(p) > 4 and p in page_html:
            bad.append("service_path")
    return bad


def _glyphs(page_html):
    return ["glyph_unsupported" for ch in L.visible_text(page_html) if not _glyph_ok(ch)]


def _text_figure(lesson, page_html):
    """Figure and text agree: the problem statement is rendered unchanged (its
    numbers included), every point named in it exists in the figure, and every
    figure label is a declared point."""
    bad = []
    import html as _html
    if _html.escape(lesson["problem"]["text"]) not in page_html:
        bad.append("problem_text_altered")
    pts = set(lesson["figure"]["points"])
    for name in set(re.findall(r"(?<![A-Za-z])[A-Z][0-9]?(?![a-z])", lesson["problem"]["text"])):
        if name not in pts:
            bad.append("text_point_missing_in_figure")
    for _, _, body in _svgs(page_html):
        for t in re.findall(r"<text[^>]*>([^<]*)</text>", body):
            if _html.unescape(t) not in pts:
                bad.append("figure_label_undeclared")
    return bad


def _narration(lesson):
    """Audio and picture correspondence. There is no audio track yet, so the
    check is limited to every unlocked beat having board text and narration
    text; synchronised audio is reported as not applicable, never as passed."""
    bad = []
    for b in lesson.get("beats", []):
        if b.get("locked"):
            continue
        if not b.get("board") or not b.get("say"):
            bad.append("beat_without_board_or_narration")
    return bad


def _sha(s):
    return "sha256:" + hashlib.sha256(s.encode("utf-8")).hexdigest()


def check(lesson, student, teacher_html, student_html):
    """Re-render both previews here (a fresh, empty-environment process) and
    check them. Returns the report; it never raises on a finding so the
    platform can show and record every blocking code."""
    t2, s2 = render.page(lesson, "teacher"), render.page(student, "student")
    checks = []
    repro = []
    if _sha(t2) != _sha(teacher_html):
        repro.append("teacher_preview_not_reproducible")
    if _sha(s2) != _sha(student_html):
        repro.append("student_preview_not_reproducible")
    checks.append(_check("clean_reproduction", repro))
    pages = (("teacher", teacher_html), ("student", student_html))
    checks.append(_check("no_external_resources", [c for _, p in pages for c in _resources(p)]))
    checks.append(_check("no_host_paths", [c for _, p in pages for c in _host_paths(p)]))
    checks.append(_check("glyph_coverage", [c for _, p in pages for c in _glyphs(p)]))
    outside, clipped, overlap, views, curves = [], [], [], 0, 0
    for _, p in pages:
        o, c, v, n, k = _geometry(p)
        outside, clipped, overlap, views, curves = outside + o, clipped + c, overlap + v, views + n, curves + k
    checks.append(_check("figure_in_view", outside, views=views, curves_cut_at_edge=curves))
    checks.append(_check("label_overflow", clipped, warn=True))
    checks.append(_check("label_occlusion", overlap, warn=True))
    checks.append(_check("figure_text_consistency", _text_figure(lesson, teacher_html) + _text_figure(student, student_html)))
    checks.append(_check("board_narration", _narration(lesson)))
    checks.append({"id": "audio_sync", "status": "not_applicable"})
    leak = []
    try:
        L.guard({"html": L.visible_text(student_html)}, L.answer_markers(lesson))
    except Exception:  # noqa: BLE001 - the guard raises MathError on a leak
        leak.append("student_page_shows_answer")
    checks.append(_check("student_page_hides_answer", leak))
    blocking = [c["id"] for c in checks if c["status"] == "fail"]
    warnings = [c["id"] for c in checks if c["status"] == "warn"]
    state = "failed" if blocking else ("warnings" if warnings else "passed")
    return {"schema_version": SCHEMA, "checker": CHECKER_VERSION, "renderer": render.RENDERER_VERSION,
            "scope": "presentation", "math_verification": False, "environment": "isolated_subprocess_empty_env",
            "state": state, "blocking": blocking, "warnings": warnings, "checks": checks,
            "teacher_html_sha256": _sha(teacher_html), "student_html_sha256": _sha(student_html)}
