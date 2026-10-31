# -*- coding: utf-8 -*-
"""DEV-043 per-family admission sets (engineering side).

For every family: at least 100 legal parameter combinations, each solved and
verified by the service and then cross-checked against an INDEPENDENT numeric
reference written here from the geometry (own coordinates, a random rigid
rotation, high precision floats; no call into the solver, the verifier or the
geometry kernel), plus at least 20 illegal combinations with the expected
refusal code. Boundary values (smallest and largest allowed), near-degenerate
shapes and every refusal class are included and counted per family.

This does NOT replace the math lead's independent review of reference answers
and the frozen FR-Q01 admission set (status "pending_math_lead" in the
report); numeric agreement at sampled points is evidence, not a proof.

`python -m tests.test_admission report` prints the per-family report;
tests/admission_report.json must equal it (checked below)."""

import itertools
import json
import os
import sys
import unittest

import mpmath as mp

from mathsvc.errors import MathError
from mathsvc.families import REGISTRY
from mathsvc.params import parse_exact

mp.mp.dps = 40
TOL = mp.mpf("1e-25")
REPORT_PATH = os.path.join(os.path.dirname(__file__), "admission_report.json")
REPORT_KIND = "engineering_admission_set_not_frozen"
TO_MM = {"mm": 1, "cm": 10, "dm": 100, "m": 1000}


def num(t):
    """Exact parameter text to a high precision float, through the same
    whitelist parser the service uses for input (no eval)."""
    return mp.mpf(str(parse_exact(t).evalf(60)))


def rotation(seed):
    """Deterministic rigid rotation (unit quaternion from the seed)."""
    rnd = mp.mpf(seed * 7919 % 1000 + 1) / 1001
    q = [mp.cos(rnd * 3), mp.sin(rnd * 5), mp.cos(rnd * 11) / 2, mp.sin(rnd * 13) / 3]
    n = mp.sqrt(sum(x * x for x in q))
    w, x, y, z = [v / n for v in q]
    m = [[1 - 2 * (y * y + z * z), 2 * (x * y - w * z), 2 * (x * z + w * y)],
         [2 * (x * y + w * z), 1 - 2 * (x * x + z * z), 2 * (y * z - w * x)],
         [2 * (x * z - w * y), 2 * (y * z + w * x), 1 - 2 * (x * x + y * y)]]
    return lambda p: [sum(m[i][j] * p[j] for j in range(3)) + (i + 1) * mp.mpf(seed % 5) for i in range(3)]


def sub(a, b):
    return [x - y for x, y in zip(a, b)]


def dot(a, b):
    return sum(x * y for x, y in zip(a, b))


def cross(a, b):
    return [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]]


def norm(a):
    return mp.sqrt(dot(a, a))


def sin_line_plane(d, n):
    return abs(dot(d, n)) / (norm(d) * norm(n))


# Independent numeric references, one per family -----------------------------

def ref_vol(p, seed):
    R = rotation(seed)
    u = p["units"]
    l, w, h = (num(p["params"][k]) * TO_MM[u[k]] / TO_MM[p["target_unit"]] for k in "lwh")
    o = R([0, 0, 0])
    ex, ey, ez = (sub(R(v), o) for v in ([l, 0, 0], [0, w, 0], [0, 0, h]))
    return abs(dot(ex, cross(ey, ez)))  # triple product, not l*w*h of the solver


def ref_angle1(p, seed):
    R = rotation(seed)
    e = num(p["params"]["e"])
    A, B, C, D = (R(v) for v in ([0, 0, 0], [e, 0, 0], [e, e, 0], [0, e, 0]))
    A1 = R([0, 0, e])
    return sin_line_plane(sub(C, A1), cross(sub(B, A), sub(D, A)))


def ref_angle2(p, seed):
    R = rotation(seed)
    a, h = num(p["params"]["a"]), num(p["params"]["h"])
    s = a / 2
    A, B, C = R([-s, -s, 0]), R([s, -s, 0]), R([s, s, 0])
    P = R([0, 0, h])
    E = [(x + y) / 2 for x, y in zip(P, C)]
    return sin_line_plane(sub(E, B), cross(sub(A, P), sub(C, P)))


def ref_chord(p, seed):
    a, b = num(p["params"]["a"]), num(p["params"]["b"])
    c = mp.sqrt(a * a - b * b)
    lens = []
    for k in range(0, 721):  # directions 0..pi in quarter degrees, both axes included exactly
        th = mp.pi * k / 720
        ct, st = (mp.mpf(1), mp.mpf(0)) if k == 0 else ((mp.mpf(0), mp.mpf(1)) if k == 360 else (mp.cos(th), mp.sin(th)))
        if k == 720:
            ct, st = mp.mpf(-1), mp.mpf(0)
        qa = ct * ct / (a * a) + st * st / (b * b)
        qb = 2 * c * ct / (a * a)
        qc = c * c / (a * a) - 1
        disc = qb * qb - 4 * qa * qc
        lens.append(mp.sqrt(disc) / qa)  # |t1 - t2| on a unit direction
    return (min(lens), max(lens))


def ref_slope(p, seed):
    a, b = num(p["params"]["a"]), num(p["params"]["b"])
    vals = []
    for k in (1, 2, 5, 7, 11):
        phi = mp.pi * k / 13 + seed
        A = (a * mp.cos(phi), b * mp.sin(phi))
        if abs(A[1]) < mp.mpf("1e-10"):
            continue
        B = (-A[0], -A[1])
        vals.append((A[1] / (A[0] - a)) * (B[1] / (B[0] - a)))
    assert max(vals) - min(vals) < TOL * max(1, abs(vals[0]))  # constant over the sampled points
    return vals[0]


def ref_parabola(p, seed):
    pp = num(p["params"]["p"])
    vals = []
    for k in (1, 3, 4, 6, 9):
        th = mp.pi * k / 10 + mp.mpf(seed % 7) / 100  # never the x axis
        ct, st = mp.cos(th), mp.sin(th)
        # (t st)^2 = 2p (p/2 + t ct)
        qa, qb, qc = st * st, -2 * pp * ct, -pp * pp
        r = mp.sqrt(qb * qb - 4 * qa * qc)
        pts = [(pp / 2 + t * ct, t * st) for t in ((-qb + r) / (2 * qa), (-qb - r) / (2 * qa))]
        vals.append(pts[0][0] * pts[1][0] + pts[0][1] * pts[1][1])
    assert max(vals) - min(vals) < TOL * max(1, abs(vals[0]))
    return vals[0]


REF = {"SOLID-VOL-01": ref_vol, "SOLID-ANGLE-01": ref_angle1, "SOLID-ANGLE-02": ref_angle2,
       "CONIC-CHORD-01": ref_chord, "CONIC-SLOPE-01": ref_slope, "CONIC-PARABOLA-01": ref_parabola}

# Legal combinations ----------------------------------------------------------

POOL = ["1/100", "1/3", "1/2", "1", "3/2", "2", "sqrt(3)", "3", "2*sqrt(2)", "4", "5", "sqrt(30)", "7", "10", "25", "50", "100"]
WIDE = ["1/1000", "1/7", "3/4", "1", "sqrt(2)", "2", "sqrt(5)/2", "7", "12", "2*sqrt(3)", "25", "99", "250", "1000"]
UNITS = ["mm", "cm", "dm", "m"]
SINGLE = ([str(i) for i in range(1, 41)] + ["%d/%d" % (k, 7) for k in range(1, 21)] + ["sqrt(%d)" % m for m in (2, 3, 5, 6, 7, 10, 11, 13, 14, 15)] +
          ["%d*sqrt(%d)" % (k, m) for k in (2, 3, 5) for m in (2, 3, 7)] + ["1/100", "99", "100", "199/2", "sqrt(9999)", "10*sqrt(99)", "50", "60", "75", "80", "90"] +
          [str(i) for i in range(41, 50)] + ["%d/%d" % (k, 11) for k in (1, 3, 5, 7, 9, 13)])


def legal():
    out = {f: [] for f in REF}
    for i, (l, w, h) in enumerate(itertools.islice(itertools.product(WIDE, WIDE, WIDE), 0, len(WIDE) ** 3, 23)):
        u = {k: UNITS[(i + j) % 4] for j, k in enumerate("lwh")}
        out["SOLID-VOL-01"].append({"family_id": "SOLID-VOL-01", "params": {"l": l, "w": w, "h": h}, "units": u, "target_unit": UNITS[(i * 3) % 4]})
    for e in SINGLE:
        out["SOLID-ANGLE-01"].append({"family_id": "SOLID-ANGLE-01", "params": {"e": e}})
        out["CONIC-PARABOLA-01"].append({"family_id": "CONIC-PARABOLA-01", "params": {"p": e}})
    for a, h in itertools.islice(itertools.product(POOL, POOL), 0, len(POOL) ** 2, 2):
        out["SOLID-ANGLE-02"].append({"family_id": "SOLID-ANGLE-02", "params": {"a": a, "h": h}})
    pairs = [(a, b) for a in POOL for b in POOL if parse_exact(a) > parse_exact(b)]
    near = [("101/100", "1"), ("100", "99"), ("100", "1/100"), ("sqrt(2)", "1"), ("2", "sqrt(3)")]  # near circle, very flat, classic
    for a, b in dict.fromkeys(near + pairs[: 110]):
        out["CONIC-CHORD-01"].append({"family_id": "CONIC-CHORD-01", "params": {"a": a, "b": b}})
    for a, b in dict.fromkeys(near + pairs[20: 130]):
        out["CONIC-SLOPE-01"].append({"family_id": "CONIC-SLOPE-01", "params": {"a": a, "b": b}})
    return out


# Illegal combinations: (problem, expected code, class) ----------------------

def illegal():
    out = {f: [] for f in REF}

    def add(fid, params, code, cls, **extra):
        out[fid].append((dict({"family_id": fid, "params": params}, **extra), code, cls))

    bad_text = [("x", "PARAMETER_LIMIT"), ("1.5", "PARAMETER_LIMIT"), ("2**10", "PARAMETER_LIMIT"), ("sqrt(sqrt(2))", "PARAMETER_LIMIT"),
                ("__import__('os')", "PARAMETER_LIMIT"), ("1" * 60, "PARAMETER_LIMIT"), ("", "PARAMETER_LIMIT")]
    one = {"SOLID-ANGLE-01": ("e", 100), "CONIC-PARABOLA-01": ("p", 100)}
    two = {"SOLID-ANGLE-02": ("a", "h", 100), "CONIC-CHORD-01": ("a", "b", 100), "CONIC-SLOPE-01": ("a", "b", 100)}
    for fid, (k, mx) in one.items():
        for v, code in bad_text:
            add(fid, {k: v}, code, "malformed")
        for v in ("0", "-1", "-1/2", "-sqrt(2)"):
            add(fid, {k: v}, "CONDITION_CONFLICT", "degenerate_or_negative")
        for v in (str(mx + 1), "1001/10", "1000", "100*sqrt(2)"):
            add(fid, {k: v}, "PARAMETER_LIMIT", "over_limit")
        add(fid, {}, "INPUT_AMBIGUOUS", "missing")
        add(fid, {k: "2"}, "UNSUPPORTED_FAMILY", "wrong_target", target="other_target")
        add(fid, {k: "2"}, "INPUT_AMBIGUOUS", "unconfirmed_input", input_state="pending_confirmation")
        add(fid, {k: "2"}, "METHOD_MISMATCH", "method", method_profile={"allowed": ["calculus"]})
    for fid, (k1, k2, mx) in two.items():
        for v, code in bad_text:
            add(fid, {k1: "3", k2: v}, code, "malformed")
        if fid == "SOLID-ANGLE-02":
            for a, h in (("0", "1"), ("2", "0"), ("-2", "1"), ("2", "-1")):
                add(fid, {k1: a, k2: h}, "CONDITION_CONFLICT", "degenerate_or_negative")
        else:
            for a, b in (("2", "2"), ("1", "2"), ("sqrt(3)", "sqrt(3)"), ("0", "1"), ("3", "-1")):
                add(fid, {k1: a, k2: b}, "CONDITION_CONFLICT", "degenerate_or_negative")
        for a, b in ((str(mx + 1), "1"), ("1000", "1"), ("101", "100")):
            add(fid, {k1: a, k2: b}, "PARAMETER_LIMIT", "over_limit")
        add(fid, {k1: "3"}, "INPUT_AMBIGUOUS", "missing")
        add(fid, {k1: "3", k2: "1"}, "UNSUPPORTED_FAMILY", "wrong_target", target="other_target")
        add(fid, {k1: "3", k2: "1"}, "INPUT_AMBIGUOUS", "unconfirmed_input", input_state="pending_confirmation")
    base = {"SOLID-ANGLE-01": {"e": "2"}, "CONIC-PARABOLA-01": {"p": "2"}, "SOLID-ANGLE-02": {"a": "2", "h": "1"},
            "CONIC-CHORD-01": {"a": "3", "b": "1"}, "CONIC-SLOPE-01": {"a": "3", "b": "1"}, "SOLID-VOL-01": {"l": "3", "w": "4", "h": "5"}}
    for fid, pr in base.items():
        add(fid, dict(pr, q="1"), "CONDITION_CONFLICT", "extra_parameter")
        add(fid, pr, "UNSUPPORTED_FAMILY", "nonstandard_naming", naming={"zz": "Q"})
        add(fid, ["2"], "CONDITION_CONFLICT", "params_not_object")
        add(fid, {k: True for k in pr}, "PARAMETER_LIMIT", "boolean_value")
        if fid != "SOLID-VOL-01":
            add(fid, pr, "PARAMETER_LIMIT", "unit", unit="inch")
    fid = "SOLID-VOL-01"
    for v, code in bad_text:
        add(fid, {"l": "3", "w": "4", "h": v}, code, "malformed")
    for v in ("0", "-1", "-1/2"):
        add(fid, {"l": "3", "w": v, "h": "5"}, "CONDITION_CONFLICT", "degenerate_or_negative")
    for v in ("1001", "2000", "1000*sqrt(2)"):
        add(fid, {"l": "3", "w": "4", "h": v}, "PARAMETER_LIMIT", "over_limit")
    add(fid, {"l": "3", "w": "4"}, "INPUT_AMBIGUOUS", "missing")
    add(fid, {"l": "3", "w": "4", "h": "5"}, "UNSUPPORTED_FAMILY", "wrong_target", target="surface_area")
    add(fid, {"l": "3", "w": "4", "h": "5"}, "UNSUPPORTED_FAMILY", "unsupported_shape", oblique=True)
    add(fid, {"l": "3", "w": "4", "h": "5"}, "PARAMETER_LIMIT", "unit", unit="inch")
    add(fid, {"l": "3", "w": "4", "h": "5"}, "INPUT_AMBIGUOUS", "mixed_units_without_target", units={"l": "cm", "w": "mm", "h": "cm"})
    add(fid, {"l": "3", "w": "4", "h": "5"}, "INPUT_AMBIGUOUS", "unconfirmed_input", input_state="pending_confirmation")
    return out


def solver_value(fid, sol):
    ex = sol["_exact"]
    if fid == "CONIC-CHORD-01":
        return (mp.mpf(str(ex["lo"].evalf(60))), mp.mpf(str(ex["hi"].evalf(60))))
    return mp.mpf(str(list(ex.values())[0].evalf(60)))


def close(x, y):
    if isinstance(x, tuple):
        return all(close(a, b) for a, b in zip(x, y))
    return abs(x - y) <= TOL * max(1, abs(y))


def run_legal(fid, p, seed):
    mod = REGISTRY[fid]
    n = mod.normalize(p)
    s = mod.solve(n)
    r = mod.verify(n, s)
    return close(solver_value(fid, s), REF[fid](p, seed)), r["math_state"], [c.get("result") for c in r.get("checks", [])]


def refusal(p):
    try:
        REGISTRY[p["family_id"]].normalize(p)
    except MathError as e:
        return e.code
    return None


def report():
    leg, ill = legal(), illegal()
    fams = {}
    for fid in sorted(REF):
        agree = verified = 0
        for i, p in enumerate(leg[fid]):
            ok, state, _ = run_legal(fid, p, i)
            agree += int(ok)
            verified += int(state == "verified")
        classes = {}
        refused = 0
        for p, code, cls in ill[fid]:
            got = refusal(p)
            refused += int(got == code)
            classes[cls] = classes.get(cls, 0) + 1
        fams[fid] = {"legal_cases": len(leg[fid]), "numeric_cross_check_agree": agree, "verified": verified,
                     "illegal_cases": len(ill[fid]), "refused_with_expected_code": refused, "refusal_classes": dict(sorted(classes.items())),
                     "independent_reference": "numeric geometry in tests/test_admission.py (mpmath 40 digits, rotated coordinates)",
                     "math_lead_review": "pending_math_lead", "admission": "engineering_passed_pending_review" if (
                         agree == verified == len(leg[fid]) >= 100 and refused == len(ill[fid]) >= 20) else "failed"}
    return {"kind": REPORT_KIND, "tolerance": "1e-25 relative", "families": fams,
            "not_covered_here": ["math lead review of reference answers (DEV-043, (b))", "frozen FR-Q01 admission set written by independent teachers",
                                 "timeouts: covered by tests.test_hostile and the Go job tests, not by these sets"]}


class Admission(unittest.TestCase):
    def test_legal_sets_agree_with_independent_reference(self):
        for fid, cases in legal().items():
            self.assertGreaterEqual(len(cases), 100, fid)
            self.assertEqual(len({json.dumps(c, sort_keys=True) for c in cases}), len(cases), fid)
            for i, p in enumerate(cases):
                ok, state, results = run_legal(fid, p, i)
                self.assertTrue(ok, (fid, p))
                self.assertEqual(state, "verified", (fid, p))
                self.assertNotIn("failed", results, (fid, p))

    def test_boundaries_are_in_the_sets(self):
        leg = legal()
        vals = lambda fid: {v for p in leg[fid] for v in p["params"].values()}
        for fid in ("SOLID-ANGLE-01", "CONIC-PARABOLA-01", "SOLID-ANGLE-02", "CONIC-CHORD-01", "CONIC-SLOPE-01"):
            self.assertIn("100", vals(fid), fid)
            self.assertIn("1/100", vals(fid), fid)
        self.assertTrue({"1000", "1/1000"} <= vals("SOLID-VOL-01"))
        self.assertIn({"a": "101/100", "b": "1"}, [p["params"] for p in leg["CONIC-CHORD-01"]])

    def test_illegal_sets_are_refused_with_the_expected_code(self):
        for fid, cases in illegal().items():
            self.assertGreaterEqual(len(cases), 20, fid)
            classes = {cls for _, _, cls in cases}
            self.assertTrue({"malformed", "degenerate_or_negative", "over_limit", "missing", "wrong_target"} <= classes, fid)
            for p, code, cls in cases:
                self.assertEqual(refusal(p), code, (fid, cls, p))

    def test_reference_detects_a_wrong_answer(self):
        """The cross-check is not vacuous: a perturbed solver value disagrees."""
        p = legal()["CONIC-PARABOLA-01"][5]
        ref = REF["CONIC-PARABOLA-01"](p, 5)
        self.assertFalse(close(ref * (1 + mp.mpf("1e-12")), ref))
        lo, hi = REF["CONIC-CHORD-01"]({"params": {"a": "2", "b": "sqrt(3)"}}, 0)
        self.assertTrue(close((lo, hi), (mp.mpf(3), mp.mpf(4))))

    def test_flat_ellipse_sampling_does_not_break_the_verifier(self):
        """Found by this set: with plain floats the sampled discriminant of a=100,
        b=1/100 cancelled to a negative number and verify raised TypeError."""
        mod = REGISTRY["CONIC-CHORD-01"]
        for a, b in (("100", "1/100"), ("101/100", "1"), ("100", "99")):
            n = mod.normalize({"family_id": "CONIC-CHORD-01", "params": {"a": a, "b": b}})
            r = mod.verify(n, mod.solve(n))
            self.assertEqual(r["math_state"], "verified", (a, b))
            samp = [c for c in r["checks"] if c["id"] == "numeric_sampling"][0]
            self.assertEqual((samp["result"], samp["evidence"]["out_of_range"]), ("passed", 0), (a, b))

    def test_report_file_is_current(self):
        with open(REPORT_PATH, encoding="utf-8") as f:
            self.assertEqual(json.load(f), report())


if __name__ == "__main__":
    if sys.argv[1:] == ["report"]:
        print(json.dumps(report(), ensure_ascii=False, indent=1, sort_keys=True))
    else:
        unittest.main()
