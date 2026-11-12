# -*- coding: utf-8 -*-
"""Frozen admission-set tooling (DEV-113/114/115, B4, E94).

Import and validate a U2 CSV/JSONL set, keep blind answers out of the
developer projection, record dual-reviewer disputes, and produce a layered
per-family report. Sample fixtures and the engineering admission set must
never be labelled officially admitted.

CLI (from services/math-service):
  python -m mathsvc.frozen_set validate FILE --meta META
  python -m mathsvc.frozen_set import FILE --meta META --store DIR [--answers FILE]
  python -m mathsvc.frozen_set report --store DIR [--role developer|evaluator] [--answers FILE]
  python -m mathsvc.frozen_set dispute --store DIR --case CASE --reviewer NAME --decision DECISION --note TEXT
  python -m mathsvc.frozen_set overlay --store DIR -o overlay.json
  python -m mathsvc.frozen_set ranges --store DIR -o ranges.json
"""

from __future__ import print_function

import csv
import hashlib
import io
import json
import os
import re
import sys
from collections import Counter, defaultdict
from copy import deepcopy
from datetime import datetime, timezone

ANSWER_FIELDS = ("expected_answer", "expected_unit", "expected_error_code")
REQUIRED = (
    "case_id", "category", "split", "source_group", "problem_text",
    "expected_outcome", "severity_if_wrong", "reviewer_1", "source_license",
    "author", "frozen_at",
)
CATEGORIES = {"supported", "image", "illegal", "teaching"}
SPLITS = {"dev", "blind"}
OUTCOMES = {"answer", "refuse", "needs_confirmation", "needs_teacher"}
SEVERITIES = {"blocker", "major", "minor"}
FAMILIES = {
    "SOLID-VOL-01", "SOLID-ANGLE-01", "SOLID-ANGLE-02",
    "CONIC-CHORD-01", "CONIC-SLOPE-01", "CONIC-PARABOLA-01",
}
OFFICIAL_COUNTS = {"supported": 150, "image": 30, "illegal": 30, "teaching": 30}
OFFICIAL_TOTAL = 240
OFFICIAL_MIN_PER_FAMILY = 25
SAMPLE_KIND = "sample_fixture_not_official"
OFFICIAL_KIND = "official_frozen_set"
ENGINEERING_KIND = "engineering_admission_set_not_frozen"
PII_PHONE = re.compile(r"1[3-9]\d{9}")
PII_ID = re.compile(r"\d{17}[\dXx]")
HIDDEN = object()


class FrozenSetError(ValueError):
    pass


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def sha256_text(text):
    if isinstance(text, str):
        text = text.encode("utf-8")
    return hashlib.sha256(text).hexdigest()


def _read(path):
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def load_sidecar_hash(path):
    raw = _read(path).strip().split()
    if not raw:
        raise FrozenSetError("empty sha256 sidecar")
    return raw[0].lower()


def load_meta(path):
    meta = json.loads(_read(path))
    if meta.get("kind") not in (SAMPLE_KIND, OFFICIAL_KIND):
        raise FrozenSetError("meta.kind must be %s or %s" % (SAMPLE_KIND, OFFICIAL_KIND))
    if not meta.get("version"):
        raise FrozenSetError("meta.version is required")
    return meta


def _pii(v):
    s = "" if v is None else str(v)
    return "@" in s or bool(PII_PHONE.search(s)) or bool(PII_ID.search(s))


def _parse_structured(raw):
    if raw is None or raw == "":
        return None
    if isinstance(raw, dict):
        return raw
    return json.loads(raw)


def load_table(path):
    if path.endswith(".jsonl"):
        rows = []
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                line = line.strip()
                if line:
                    rows.append(json.loads(line))
        return rows
    with open(path, encoding="utf-8-sig", newline="") as fh:
        return list(csv.DictReader(fh))


def load_answers(path):
    rows = load_table(path)
    out = {}
    for r in rows:
        cid = (r.get("case_id") or "").strip()
        if not cid:
            raise FrozenSetError("answers file row missing case_id")
        if cid in out:
            raise FrozenSetError("duplicate case_id in answers file: %s" % cid)
        out[cid] = {k: (r.get(k) or "").strip() for k in ANSWER_FIELDS}
    return out


def _check_row(row, answers, leaks):
    errs = []
    cid = (row.get("case_id") or "").strip()
    if not cid:
        errs.append("missing case_id")
        return cid, errs
    for k in REQUIRED:
        if not str(row.get(k) or "").strip():
            errs.append("missing %s" % k)
    cat = (row.get("category") or "").strip()
    if cat and cat not in CATEGORIES:
        errs.append("bad category")
    split = (row.get("split") or "").strip()
    if split and split not in SPLITS:
        errs.append("bad split")
    fid = (row.get("family_id") or "").strip()
    if cat == "supported":
        if fid not in FAMILIES:
            errs.append("supported case needs a known family_id")
    elif fid not in ("", "none"):
        if fid not in FAMILIES:
            errs.append("unknown family_id")
    outcome = (row.get("expected_outcome") or "").strip()
    if outcome and outcome not in OUTCOMES:
        errs.append("bad expected_outcome")
    sev = (row.get("severity_if_wrong") or "").strip()
    if sev and sev not in SEVERITIES:
        errs.append("bad severity_if_wrong")
    if cat == "image" and not str(row.get("image_ref") or "").strip():
        errs.append("image case needs image_ref")
    if cat == "supported" and not str(row.get("structured_input") or "").strip():
        errs.append("supported case needs structured_input")
    try:
        structured = _parse_structured(row.get("structured_input"))
        if structured and cat == "supported" and structured.get("family_id") not in (None, fid):
            errs.append("structured_input.family_id does not match family_id")
    except ValueError:
        errs.append("structured_input is not JSON")
        structured = None
    # Blind answers must live in the sealed answers file, never in the main table.
    if split == "blind":
        for k in ANSWER_FIELDS:
            if str(row.get(k) or "").strip():
                leaks.append(cid)
                errs.append("blind case leaks %s in the main file" % k)
    expected = {k: str(row.get(k) or "").strip() for k in ANSWER_FIELDS}
    if split == "blind":
        expected = dict(answers.get(cid) or {})
    if outcome == "answer" and not expected.get("expected_answer"):
        if split != "blind" or cid in answers:
            errs.append("answer case needs expected_answer")
        elif split == "blind" and cid not in answers:
            # allowed at validate-without-answers time; noted by caller
            pass
    if outcome == "refuse" and not expected.get("expected_error_code"):
        if split != "blind" or cid in answers:
            errs.append("refuse case needs expected_error_code")
    for k, v in list(row.items()) + list(expected.items()):
        if k in ANSWER_FIELDS and split == "blind":
            continue
        if _pii(v):
            errs.append("field %s looks like personal data" % k)
    return cid, errs


def validate(path, meta, answers=None, sha_path=None):
    """Validate a frozen-set file. answers may be omitted (blind answers then unchecked)."""
    answers = answers or {}
    meta = dict(meta)
    problems = []
    leaks = []
    rows = load_table(path)
    if sha_path:
        got = sha256_file(path)
        want = load_sidecar_hash(sha_path)
        if got != want:
            problems.append("sha256 mismatch: file %s sidecar %s" % (got, want))
    seen = set()
    groups = {}
    parsed = []
    missing_blind = []
    for row in rows:
        cid, errs = _check_row(row, answers, leaks)
        if not cid:
            problems.extend(errs)
            continue
        if cid in seen:
            problems.append("duplicate case_id %s" % cid)
        seen.add(cid)
        split = (row.get("split") or "").strip()
        grp = (row.get("source_group") or "").strip()
        if grp:
            if grp in groups and groups[grp] != split:
                problems.append("source_group %s appears in more than one split" % grp)
            groups[grp] = split
        if split == "blind" and answers and cid not in answers:
            if (row.get("expected_outcome") or "").strip() in ("answer", "refuse"):
                missing_blind.append(cid)
        problems.extend("%s: %s" % (cid, e) for e in errs)
        rec = {k: (row.get(k) or "").strip() if not isinstance(row.get(k), dict) else row.get(k) for k in
               list(row.keys())}
        rec["case_id"] = cid
        rec["structured_input"] = _parse_structured(row.get("structured_input")) if row.get("structured_input") else None
        if cid in answers:
            rec.update({k: answers[cid].get(k, "") for k in ANSWER_FIELDS})
        parsed.append(rec)
    if missing_blind:
        problems.append("blind answers missing for: %s" % ", ".join(missing_blind))
    counts = Counter((r.get("category") or "") for r in parsed)
    supported_fams = Counter((r.get("family_id") or "") for r in parsed if r.get("category") == "supported")
    official = meta.get("kind") == OFFICIAL_KIND
    if official:
        if len(parsed) != OFFICIAL_TOTAL:
            problems.append("official set must have %d cases, got %d" % (OFFICIAL_TOTAL, len(parsed)))
        for cat, n in OFFICIAL_COUNTS.items():
            if counts[cat] != n:
                problems.append("official %s count must be %d, got %d" % (cat, n, counts[cat]))
        for fid, n in supported_fams.items():
            if n < OFFICIAL_MIN_PER_FAMILY:
                problems.append("official family %s has %d supported cases, need >= %d" % (fid, n, OFFICIAL_MIN_PER_FAMILY))
    if leaks:
        problems.append("blind answers leaked in the main file: %s" % ", ".join(leaks))
    if problems:
        raise FrozenSetError("; ".join(problems))
    return {
        "version": meta["version"],
        "kind": meta["kind"],
        "counts": dict(counts),
        "supported_families": dict(supported_fams),
        "n": len(parsed),
        "sha256": sha256_file(path),
        "cases": parsed,
        "official": official,
        "counts_toward_admission": official,
    }


def developer_view(cases):
    """Strip sealed fields so a developer projection cannot see blind answers."""
    out = []
    for c in cases:
        row = deepcopy(c)
        if row.get("split") == "blind":
            for k in ANSWER_FIELDS:
                row.pop(k, None)
                row[k] = None
            row["answers_sealed"] = True
        out.append(row)
    return out


def contains_secret(payload, secrets):
    blob = json.dumps(payload, ensure_ascii=False)
    for s in secrets:
        if s and str(s) in blob:
            return True
    return False


def import_set(path, meta_path, store_dir, answers_path=None, sha_path=None):
    meta = load_meta(meta_path)
    answers = load_answers(answers_path) if answers_path else {}
    if sha_path is None:
        candidate = path + ".sha256"
        if os.path.exists(candidate):
            sha_path = candidate
        elif os.path.exists(os.path.splitext(path)[0] + ".sha256"):
            sha_path = os.path.splitext(path)[0] + ".sha256"
    result = validate(path, meta, answers=answers, sha_path=sha_path)
    dest = os.path.join(store_dir, meta["version"])
    manifest_path = os.path.join(dest, "manifest.json")
    payload = {
        "version": meta["version"],
        "kind": meta["kind"],
        "source_sha256": result["sha256"],
        "answers_sha256": sha256_file(answers_path) if answers_path else None,
        "n": result["n"],
        "counts": result["counts"],
        "official": result["official"],
        "counts_toward_admission": result["counts_toward_admission"],
        "imported_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "source_file": os.path.basename(path),
    }
    if os.path.exists(manifest_path):
        prev = json.loads(_read(manifest_path))
        if prev.get("source_sha256") == payload["source_sha256"] and prev.get("answers_sha256") == payload["answers_sha256"]:
            payload["idempotent"] = True
            return payload
        raise FrozenSetError("version %s already imported with a different checksum" % meta["version"])
    os.makedirs(dest, exist_ok=True)
    visible = developer_view(result["cases"])
    with open(os.path.join(dest, "cases.json"), "w", encoding="utf-8") as fh:
        json.dump(visible, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    if answers:
        sealed = os.path.join(dest, "answers.json")
        with open(sealed, "w", encoding="utf-8") as fh:
            json.dump(answers, fh, ensure_ascii=False, indent=2)
            fh.write("\n")
        try:
            os.chmod(sealed, 0o600)
        except OSError:
            pass
        secrets = [a.get("expected_answer") for a in answers.values()]
        if contains_secret(visible, secrets):
            raise FrozenSetError("developer cases leaked a sealed answer")
    if os.path.exists(os.path.join(dest, "disputes.json")) is False:
        with open(os.path.join(dest, "disputes.json"), "w", encoding="utf-8") as fh:
            json.dump([], fh)
            fh.write("\n")
    with open(manifest_path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    payload["idempotent"] = False
    payload["store"] = dest
    return payload


def load_store(store_dir, version=None):
    if version is None:
        # allow passing the version directory itself
        if os.path.exists(os.path.join(store_dir, "manifest.json")):
            dest = store_dir
        else:
            vers = sorted(p for p in os.listdir(store_dir) if os.path.exists(os.path.join(store_dir, p, "manifest.json")))
            if not vers:
                raise FrozenSetError("no imported frozen set in %s" % store_dir)
            dest = os.path.join(store_dir, vers[-1])
    else:
        dest = os.path.join(store_dir, version)
    manifest = json.loads(_read(os.path.join(dest, "manifest.json")))
    cases = json.loads(_read(os.path.join(dest, "cases.json")))
    answers = {}
    ap = os.path.join(dest, "answers.json")
    if os.path.exists(ap):
        answers = json.loads(_read(ap))
    disputes = []
    dp = os.path.join(dest, "disputes.json")
    if os.path.exists(dp):
        disputes = json.loads(_read(dp))
    return dest, manifest, cases, answers, disputes


def cases_for_role(cases, answers, role):
    if role == "evaluator":
        out = []
        for c in cases:
            row = deepcopy(c)
            if row.get("split") == "blind" and row["case_id"] in answers:
                row.update(answers[row["case_id"]])
                row["answers_sealed"] = False
            out.append(row)
        return out
    if role != "developer":
        raise FrozenSetError("role must be developer or evaluator")
    return developer_view(cases)


def record_dispute(store_dir, case_id, reviewer, decision, note="", version=None):
    if not reviewer or not str(reviewer).strip():
        raise FrozenSetError("reviewer is required")
    if decision not in ("uphold", "revise", "open"):
        raise FrozenSetError("decision must be uphold, revise or open")
    dest, manifest, cases, answers, disputes = load_store(store_dir, version)
    found = next((c for c in cases if c["case_id"] == case_id), None)
    if not found:
        raise FrozenSetError("unknown case_id %s" % case_id)
    entry = {
        "case_id": case_id,
        "reviewer": reviewer.strip(),
        "decision": decision,
        "note": note or "",
        "severity": found.get("severity_if_wrong"),
        "at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    if _pii(entry["note"]) or _pii(entry["reviewer"]):
        raise FrozenSetError("dispute text looks like personal data")
    disputes.append(entry)
    if found.get("severity_if_wrong") == "blocker" and decision != "open":
        people = {d["reviewer"] for d in disputes if d["case_id"] == case_id and d["decision"] != "open"}
        if len(people) < 2:
            entry["status"] = "needs_second_reviewer"
        else:
            entry["status"] = "dual_reviewed"
    else:
        entry["status"] = "recorded"
    with open(os.path.join(dest, "disputes.json"), "w", encoding="utf-8") as fh:
        json.dump(disputes, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    return entry


def _answers_equal(got, expected):
    if got is None or expected is None or expected == "":
        return False
    a, b = str(got).strip().replace(" ", ""), str(expected).strip().replace(" ", "")
    if a == b:
        return True
    try:
        from .params import parse_exact
        import sympy as sp
        return sp.simplify(parse_exact(a) - parse_exact(b)) == 0
    except Exception:
        return False


def run_structured(structured):
    from .errors import MathError
    from .families import REGISTRY
    if not structured or not isinstance(structured, dict):
        return {"outcome": "refuse", "error_code": "INPUT_AMBIGUOUS", "answer": None}
    fid = structured.get("family_id")
    mod = REGISTRY.get(fid)
    if mod is None:
        return {"outcome": "refuse", "error_code": "UNSUPPORTED_FAMILY", "answer": None}
    try:
        norm = mod.normalize(structured)
        sol = mod.solve(norm)
        exact = (sol.get("answer") or {}).get("exact")
        if exact is None and sol.get("_exact"):
            exact = next(iter(sol["_exact"].values()), None)
            if exact is not None:
                from .params import exact_text
                exact = exact_text(exact)
        return {"outcome": "answer", "error_code": None, "answer": exact}
    except MathError as e:
        if e.code in ("INPUT_AMBIGUOUS",):
            # intake/family may also use this for needs_confirmation-like cases
            return {"outcome": "refuse", "error_code": e.code, "answer": None}
        return {"outcome": "refuse", "error_code": e.code, "answer": None}


def _eval_case(case):
    expected_outcome = case.get("expected_outcome")
    structured = case.get("structured_input")
    got = run_structured(structured) if structured else {"outcome": "refuse", "error_code": "UNSUPPORTED_FAMILY", "answer": None}
    ok = False
    if expected_outcome == "answer":
        ok = got["outcome"] == "answer" and _answers_equal(got["answer"], case.get("expected_answer"))
    elif expected_outcome == "refuse":
        ok = got["error_code"] == case.get("expected_error_code")
    elif expected_outcome in ("needs_confirmation", "needs_teacher"):
        ok = got["error_code"] in ("INPUT_AMBIGUOUS", "UNSUPPORTED_FAMILY") or got["outcome"] == expected_outcome
    return ok, got


def report(store_dir, role="evaluator", answers_path=None, version=None):
    dest, manifest, cases, answers, disputes = load_store(store_dir, version)
    if answers_path:
        answers = load_answers(answers_path)
    view = cases_for_role(cases, answers, role)
    layers = defaultdict(lambda: {"n": 0, "pass": 0, "fail": 0, "blocker_fail": 0, "skipped_sealed": 0})
    rows = []
    if role == "developer":
        # Developers may only run the open (dev) split; blind cases are counted as sealed.
        runnable = []
        for c in view:
            if c.get("split") == "blind":
                layers[c.get("family_id") or c.get("category")]["skipped_sealed"] += 1
                layers[c.get("family_id") or c.get("category")]["n"] += 1
            else:
                runnable.append(c)
        view_run = runnable
    else:
        view_run = view
    for c in view_run:
        key = c.get("family_id") if c.get("category") == "supported" else (c.get("category") or "other")
        layers[key]["n"] += 1
        if c.get("split") == "blind" and not c.get("expected_answer") and not c.get("expected_error_code"):
            layers[key]["skipped_sealed"] += 1
            continue
        ok, got = _eval_case(c)
        if ok:
            layers[key]["pass"] += 1
        else:
            layers[key]["fail"] += 1
            if c.get("severity_if_wrong") == "blocker":
                layers[key]["blocker_fail"] += 1
        rec = {
            "case_id": c["case_id"],
            "family_id": c.get("family_id"),
            "category": c.get("category"),
            "split": c.get("split"),
            "severity": c.get("severity_if_wrong"),
            "pass": ok,
            "got_outcome": got["outcome"],
            "got_error_code": got["error_code"],
        }
        # Never put expected answers or got answers into a developer report.
        if role == "evaluator":
            rec["got_answer"] = got["answer"]
        rows.append(rec)
    blocker_fails = sum(v["blocker_fail"] for v in layers.values())
    fails = sum(v["fail"] for v in layers.values())
    official = bool(manifest.get("official")) and manifest.get("kind") == OFFICIAL_KIND
    # Blocker failures cannot be offset; sample/engineering never counts as admission.
    admitted = official and blocker_fails == 0 and fails == 0 and role == "evaluator"
    out = {
        "kind": "frozen_admission_report",
        "frozen_set_version": manifest.get("version"),
        "source_kind": manifest.get("kind"),
        "official": official,
        "counts_toward_admission": bool(admitted),
        "role": role,
        "blocker_failures": blocker_fails,
        "failures": fails,
        "layers": {k: dict(v) for k, v in layers.items()},
        "engineering_set_confused": False,
        "cases": rows,
    }
    # Isolation guard: this report is never the engineering admission report.
    if out["kind"] == ENGINEERING_KIND:
        out["engineering_set_confused"] = True
    secrets = [a.get("expected_answer") for a in answers.values()]
    if role == "developer" and contains_secret({"layers": out["layers"], "cases": rows}, secrets):
        raise FrozenSetError("developer report leaked a sealed answer")
    return out


def build_overlay(manifest, families_status, catalog_version):
    official = manifest.get("kind") == OFFICIAL_KIND and manifest.get("official")
    return {
        "kind": "catalog_admission_overlay",
        "catalog_version": catalog_version,
        "frozen_set_version": manifest.get("version"),
        "frozen_set_sha256": manifest.get("source_sha256"),
        "signed": False,
        "source_kind": manifest.get("kind"),
        "official": official,
        "families": families_status,
        "note": "signed stays false until a math lead signs the official frozen set; the catalog ignores unsigned or sample overlays",
    }


def build_ranges(cases, manifest):
    from .params import parse_exact
    fams = defaultdict(dict)
    for c in cases:
        if c.get("category") != "supported" or not c.get("structured_input"):
            continue
        fid = c.get("family_id")
        params = (c.get("structured_input") or {}).get("params") or {}
        for name, raw in params.items():
            try:
                v = parse_exact(str(raw))
            except Exception:
                continue
            slot = fams[fid].setdefault(name, {"min": None, "max": None})
            if slot["min"] is None or (slot["min"] - v).is_positive:
                slot["min"] = v
            if slot["max"] is None or (v - slot["max"]).is_positive:
                slot["max"] = v
    families = {}
    for fid, params in fams.items():
        families[fid] = {k: {"min": str(v["min"]), "max": str(v["max"]), "max_inclusive": True} for k, v in params.items()}
    official = manifest.get("kind") == OFFICIAL_KIND and manifest.get("official")
    return {
        "kind": "frozen_param_ranges",
        "frozen_set_version": manifest.get("version"),
        "signed": False,
        "source_kind": manifest.get("kind"),
        "official": official,
        "families": families,
        "note": "unsigned sample or unsigned official ranges are ignored by the catalog",
    }


def write_json(path, obj):
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(obj, fh, ensure_ascii=False, indent=2, default=str)
        fh.write("\n")


def main(argv=None):
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv or argv[0] in ("-h", "--help"):
        print(__doc__)
        return 0
    cmd = argv[0]
    args = argv[1:]

    def opt(flag, default=None):
        if flag in args:
            i = args.index(flag)
            return args[i + 1]
        return default

    if cmd == "validate":
        path = args[0]
        meta = load_meta(opt("--meta"))
        answers = load_answers(opt("--answers")) if opt("--answers") else {}
        result = validate(path, meta, answers=answers, sha_path=opt("--sha256"))
        print(json.dumps({k: result[k] for k in ("version", "kind", "n", "counts", "sha256", "official")}, ensure_ascii=False, indent=2))
        return 0
    if cmd == "import":
        out = import_set(args[0], opt("--meta"), opt("--store"), answers_path=opt("--answers"), sha_path=opt("--sha256"))
        print(json.dumps({k: out[k] for k in out if k != "cases"}, ensure_ascii=False, indent=2))
        return 0
    if cmd == "report":
        out = report(opt("--store"), role=opt("--role", "evaluator"), answers_path=opt("--answers"))
        hide = dict(out)
        if hide["role"] != "evaluator":
            hide.pop("cases", None)
        print(json.dumps({k: hide[k] for k in hide if k != "cases"}, ensure_ascii=False, indent=2, default=str))
        if opt("--out"):
            write_json(opt("--out"), out if out["role"] == "evaluator" else hide)
        return 0 if out["failures"] == 0 or out["role"] == "developer" else 1
    if cmd == "dispute":
        rec = record_dispute(opt("--store"), opt("--case"), opt("--reviewer"), opt("--decision"), opt("--note", ""))
        print(json.dumps(rec, ensure_ascii=False, indent=2))
        return 0
    if cmd in ("overlay", "ranges"):
        dest, manifest, cases, answers, disputes = load_store(opt("--store"))
        from . import catalog
        if cmd == "overlay":
            # Only mark families admitted when an official evaluator report was clean — never here by default.
            status = {f: "dev" for f in FAMILIES}
            obj = build_overlay(manifest, status, catalog.CATALOG_VERSION)
        else:
            full = cases_for_role(cases, answers, "evaluator") if answers else cases
            obj = build_ranges(full, manifest)
        path = opt("-o") or opt("--out")
        if path:
            write_json(path, obj)
        print(json.dumps(obj, ensure_ascii=False, indent=2, default=str))
        return 0
    raise FrozenSetError("unknown command %s" % cmd)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except FrozenSetError as e:
        print("error: %s" % e, file=sys.stderr)
        sys.exit(2)
