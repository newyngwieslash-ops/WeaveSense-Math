# -*- coding: utf-8 -*-
"""Export the DEV-043 engineering admission sets for the math lead's review (U1).

Run from services/math-service:  python -m tests.admission_review_export > admission_review.csv

One row per case: legal cases carry the service's exact answer, its math state
and whether the independent numeric reference agrees; illegal cases carry the
expected and the actual refusal code. The CSV is review material only; the
sign-off itself is recorded with docs/math-workbench/templates/U1-数学审阅签字单.md."""
import csv
import json
import sys
from tests.test_admission import legal, illegal, run_legal, refusal
from mathsvc.families import REGISTRY


def main(out=sys.stdout):
    w = csv.writer(out)
    w.writerow(["family_id", "case_no", "kind", "input_json", "service_exact_answer", "math_state", "independent_numeric_agree", "expected_refusal", "actual_refusal"])
    for fid, cases in sorted(legal().items()):
        for i, p in enumerate(cases):
            mod = REGISTRY[fid]
            s = mod.solve(mod.normalize(p))
            ok, state, _ = run_legal(fid, p, i)
            w.writerow([fid, i + 1, "legal", json.dumps(p, ensure_ascii=False), json.dumps({k: str(v) for k, v in s["_exact"].items()}, ensure_ascii=False), state, ok, "", ""])
    for fid, cases in sorted(illegal().items()):
        for i, (p, code, cls) in enumerate(cases):
            w.writerow([fid, i + 1, "illegal:" + cls, json.dumps(p, ensure_ascii=False), "", "", "", code, refusal(p)])


if __name__ == "__main__":
    main()
