# -*- coding: utf-8 -*-
"""Vendor-neutral OCR stand-in (DEV-069, E97).

A fake recogniser returns a structured draft that still needs a teacher's
confirmation. It never admits a problem on its own. Real models and credentials
stay with U7; whether a platform model may serve an education tenant is still
an open decision.
"""
from __future__ import annotations

from .errors import MathError

PROVIDER = "fake-standin"

# Fictional SOLID-ANGLE-01 wording already used in intake tests (not a real pupil's work).
SAMPLE_TEXT = "正方体ABCD-A1B1C1D1棱长为2，求直线A1C与底面ABCD所成角的正弦值"
SAMPLE_DRAFT = {
    "state": "needs_confirmation",
    "family_id": "SOLID-ANGLE-01",
    "structured": {
        "family_id": "SOLID-ANGLE-01",
        "input_state": "sufficient",
        "unit": "",
        "params": {"a": "2"},
    },
    "summary": {
        "object": "正方体 ABCD-A1B1C1D1",
        "conditions": ["棱长为 2"],
        "target": "直线 A1C 与底面 ABCD 所成角的正弦值",
        "unit": "",
        "method": "待老师确认",
        "method_stated": False,
    },
    "problem_text": SAMPLE_TEXT,
}


def recognize(image, mime="image/png"):
    """Return a needs_confirmation draft for a stand-in image.

    Any non-empty image is accepted. The recogniser does not try to read
    pixels: this is a contract stand-in, not a model. Empty or huge bodies
    are refused.
    """
    from .standin_media import standin_enabled
    if not standin_enabled():
        raise MathError("PROVIDER_UNAVAILABLE", "no OCR provider is configured for this service")
    if not image:
        raise MathError("VALIDATION_FAILED", "image is empty")
    if mime not in ("image/png", "image/jpeg", "image/jpg"):
        raise MathError("VALIDATION_FAILED", "unsupported image type")
    if len(image) > 10 * 1024 * 1024:
        raise MathError("VALIDATION_FAILED", "image exceeds 10 MB")
    out = dict(SAMPLE_DRAFT)
    out["provider"] = PROVIDER
    out["ai_parts"] = ["intake"]
    # Always needs_confirmation — never skip the teacher.
    if out["state"] != "needs_confirmation":
        raise MathError("INTERNAL", "stand-in OCR tried to auto-admit a draft")
    return out
