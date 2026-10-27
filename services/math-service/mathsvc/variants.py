# -*- coding: utf-8 -*-
"""Finite variant sets (FR-M07, FR-T11, AC-22, AC-29).

A variant keeps the family, target, naming, units and method profile of the parent
problem and changes only the numbers (change_type numbers_only, transfer near: it
is never claimed as far transfer). Every item is re-normalized (conditions and
limits), solved and checked by the family's independent verifier exactly like a
parent problem. An item is dropped, with the reason recorded, when its conditions
fail, it is not verified, or its answer equals the parent's answer or another
item's answer (a student who saw the parent answer could reuse it).
Answer keys are teacher-audience data; the student projection carries prompts only.

A teacher may replace an item with their own numbers (DEV-078, "teacher_own"): the
platform sends the full ordered item list as payload["items"] ([{"params", "source"}]).
Explicit items are never skipped or padded; a teacher item gets the same
normalization, solving and independent verification as a generated one, keeps the
family, target and method (still near transfer) and is marked change_type
"teacher_own". Its prompt is rendered from the family template, never taken from
free text, so nothing the teacher types reaches students unchecked.
"""

import copy

from . import catalog
from .errors import MathError
from .families import REGISTRY as FAMILIES

MAX_ITEMS = 5


def _entry_params(mod, entry):
    vals = entry if isinstance(entry, (tuple, list)) else (entry,)
    return dict(zip(mod.PARAM_NAMES, vals))


def _public(d):
    return {k: v for k, v in d.items() if not k.startswith("_")}


def generate(payload):
    problem = payload["problem"]
    mod = FAMILIES.get(problem.get("family_id"))
    if mod is None or not hasattr(mod, "VARIANTS"):
        raise MathError("UNSUPPORTED_FAMILY", "no variant generator for this family")
    parent = mod.normalize(problem)
    explicit = payload.get("items")
    if explicit is not None:
        return _explicit(mod, problem, parent, explicit)
    count = int(payload.get("count") or 3)
    if count < 1 or count > MAX_ITEMS:
        raise MathError("PARAMETER_LIMIT", "count must be 1..%d" % MAX_ITEMS)
    candidates = []
    for entry in mod.VARIANTS:
        params = _entry_params(mod, entry)
        if params == parent["params"]:
            continue
        p = copy.deepcopy(problem)
        p["params"] = params
        candidates.append({"params": params, "problem": p})
    return {"family_id": mod.FAMILY_ID, "count": count, "parent_params": parent["params"], "candidates": candidates}


def _explicit(mod, problem, parent, explicit):
    if not isinstance(explicit, list) or not 1 <= len(explicit) <= MAX_ITEMS:
        raise MathError("PARAMETER_LIMIT", "items must be a list of 1..%d entries" % MAX_ITEMS)
    candidates = []
    for e in explicit:
        if not isinstance(e, dict) or not isinstance(e.get("params"), dict):
            raise MathError("INPUT_AMBIGUOUS", "every item needs params")
        src = e.get("source", "generated")
        if src not in ("generated", "teacher_own"):
            raise MathError("INPUT_AMBIGUOUS", "item source must be generated or teacher_own")
        params = {str(k): str(v) for k, v in e["params"].items()}
        p = copy.deepcopy(problem)
        p["params"] = params
        candidates.append({"params": params, "problem": p, "source": src})
    return {"family_id": mod.FAMILY_ID, "count": len(candidates), "parent_params": parent["params"], "candidates": candidates, "explicit": True}


def verify(payload, prior):
    gen = prior["generate_variants"]
    problem = payload["problem"]
    mod = FAMILIES[gen["family_id"]]
    parent = mod.solve(mod.normalize(problem))
    seen = {parent["answer"]["exact"]}
    items, dropped = [], []
    for c in gen["candidates"]:
        if len(items) >= gen["count"]:
            break
        try:
            n = mod.normalize(c["problem"])
            sol = mod.solve(n)
            rep = mod.verify(n, sol)
        except MathError as e:
            dropped.append({"params": c["params"], "reason": e.code, "source": c.get("source", "generated")})
            continue
        ans = sol["answer"]["exact"]
        if rep["math_state"] != "verified":
            dropped.append({"params": c["params"], "reason": "not_verified:" + rep["math_state"], "source": c.get("source", "generated")})
            continue
        if ans in seen:
            dropped.append({"params": c["params"], "reason": "answer_reused", "source": c.get("source", "generated")})
            continue
        seen.add(ans)
        vid = "v%d" % (len(items) + 1)
        items.append({
            "id": vid, "family_id": mod.FAMILY_ID, "params": n["params"], "unit": n.get("unit", ""),
            "prompt": mod.problem_text(n), "answer_format": "exact",
            "change_type": "teacher_own" if c.get("source") == "teacher_own" else "numbers_only",
            "source": c.get("source", "generated"), "transfer": "near", "method": sol.get("method", n.get("method", "")),
            "math_state": rep["math_state"], "evidence_levels": rep.get("evidence_levels", []),
            "verifier_version": rep.get("verifier_version", ""), "solver_version": sol.get("solver_version", ""),
            "answer_key": {"id": vid, "exact": ans, "plain": sol["answer"]["plain"], "latex": sol["answer"]["latex"]},
        })
    if not items:
        raise MathError("VALIDATION_FAILED", "no independent variant: every candidate failed a check or reuses the parent answer")
    return {"variant_set": {"family_id": mod.FAMILY_ID, "catalog_version": catalog.CATALOG_VERSION,
                            "parent": {"params": gen["parent_params"], "problem_hash": payload.get("problem_hash", ""),
                                       "answer_exact": parent["answer"]["exact"]},
                            "requested": gen["count"], "items": items, "dropped": dropped}}


def student_items(vs):
    keep = ("id", "family_id", "params", "unit", "prompt", "answer_format")
    return [{k: it[k] for k in keep if k in it} for it in vs["items"]]
