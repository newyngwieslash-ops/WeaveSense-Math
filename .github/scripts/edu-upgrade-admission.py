"""Education upgrade admission (DEV-126).

A change to the solver or verifier, the render templates, the model/agent
wiring, course rules or the price and cost code may only merge when the
dependent Go suites for that area actually ran and passed in this run.

    python3 edu-upgrade-admission.py go-tests.jsonl changed-files.txt report.json

changed-files.txt lists the paths changed against the base; the word ALL
(or a missing file) means "unknown change set" and requires every suite.
A path under the math service, the edumath package or the education skill
pack that matches no rule also requires every suite (fail closed). Runtime
configuration (model choice or prices set in the admin console) is not a code
change and is not covered here; see docs/math-workbench/07. The report is written as JSON and,
when GITHUB_STEP_SUMMARY is set, as a Markdown table.
"""

import json
import os
import sys

EDU_ROOTS = ("services/math-service/", "backend/internal/edumath/", "industry-packs/education-math/")

SUITES = {
    "math": [
        "TestG1SolidAngle02EndToEnd", "TestSixFamiliesThroughPlatform", "TestPGG1Chain",
        "TestVariantSetsAndExtensions", "TestFigurePredictionsAndUse", "TestLessonEdits",
    ],
    "render": [
        "TestRenderCheckReportValidation", "TestRenderRollbackDrill", "TestPGRenderRollbackDrill",
        "TestStudentLeakGuard", "TestStudentLearningLoop", "TestPGStudentLearningLoop",
    ],
    "teaching": [
        "TestStudentLearningLoop", "TestHelpRequestsAndDashboard", "TestContentIncidentChain",
        "TestPGContentIncidentChain", "TestIncidentSOP", "TestPGIncidentSOP", "TestLessonPlan", "TestPGLessonPlan",
        "TestLessonDurationEstimate", "TestLessonCompare", "TestCompareLessonsFields",
    ],
    "agent": ["TestAgentMathTools", "TestMCPProtocolWithGoSDK"],
    "course": ["TestCourseConfig", "TestPGCourseConfig"],
    "cost": [
        "TestStageSettlementOutcomes", "TestLedgerReconcileAndExport", "TestPGLedgerReconcileAndExport",
        "TestBudgetsAndFairness", "TestPGBudgetsAndFairness", "TestDeliveryCostReport", "TestReceiptContract",
        "TestPriceChangeAudit", "TestPriceChangeDiff", "TestReadsAndRequestTrace",
    ],
    "permissions": [
        "TestClassesInvitesConsentAuthorization", "TestPGEduTablesForceRLS", "TestRoleBindingScopeExpiry",
        "TestPGRoleBindingScopeExpiry", "TestStudentLeakGuard", "TestPauses", "TestPGPauses",
        "TestPauseFailClosed", "TestPGPauseFailClosed", "TestProblemImages", "TestPGProblemImages", "TestEduResponsesNeverCached",
        "TestContentProvenance", "TestProvenanceOf", "TestPGReadsAndRequestTrace",
        "TestArtifactPurgeAndClassArchive", "TestPGArtifactPurgeAndClassArchive",
    ],
}

MS = "services/math-service/mathsvc/"
ED = "backend/internal/edumath/"
# (prefix, areas). The first matching rule wins; tests directories count as their area.
RULES = [
    (MS + "families/", ["math"]),
    (MS + "kernel/", ["math"]),
    (MS + "render", ["render"]),
    (MS + "fmt.py", ["render"]),
    (MS + "lesson.py", ["render", "math"]),
    (MS + "edits.py", ["math", "render"]),
    (MS + "variants.py", ["math"]),
    (MS + "grade.py", ["math", "teaching"]),
    (MS + "pipeline.py", ["math"]),
    (MS + "runner.py", ["math"]),
    (MS + "params.py", ["math"]),
    (MS + "catalog.py", ["math", "course"]),
    (MS + "intake.py", ["math"]),
    (MS + "manual.py", ["math"]),
    ("services/math-service/tests/", ["math", "render"]),
    ("services/math-service/requirements.txt", ["math", "render"]),
    (ED + "render_check", ["render"]),
    (ED + "artifacts", ["render", "permissions"]),
    (ED + "agent", ["agent", "cost"]),
    (ED + "client", ["agent", "math"]),
    (ED + "courses", ["course"]),
    (ED + "catalog", ["course", "math"]),
    (ED + "cost", ["cost"]),
    (ED + "budget", ["cost"]),
    (ED + "ledger", ["cost"]),
    (ED + "worker", ["cost", "math"]),
    (ED + "learn", ["teaching", "permissions"]),
    (ED + "classes", ["teaching", "permissions"]),
    (ED + "help", ["teaching"]),
    (ED + "regrade", ["teaching", "math"]),
    (ED + "incidents", ["teaching", "permissions"]),
    (ED + "pauses", ["permissions", "teaching"]),
    (ED + "authz", ["permissions"]),
    (ED + "problemimages", ["permissions"]),
    (ED + "cache", ["permissions"]),
    (ED + "lessonplan", ["teaching", "cost"]),
    (ED + "lessoncompare", ["teaching"]),
    (ED + "provenance", ["permissions"]),
    (ED + "priceaudit", ["cost", "permissions"]),
    (ED + "views", ["cost", "permissions"]),
    (ED + "trace", ["permissions"]),
    (ED + "retention", ["permissions"]),
    (ED + "archive_purge", ["permissions"]),
    (ED + "roles", ["permissions"]),
    (ED + "opsaccess", ["permissions"]),
    (ED + "org", ["permissions"]),
    (ED + "figure", ["math"]),
    (ED + "variant", ["math"]),
    (ED + "edits", ["math", "render"]),
    (ED + "manual", ["math"]),
    (ED + "intake", ["math"]),
    (ED + "quality", ["math", "teaching"]),
    (ED + "service", ["cost", "math"]),
    ("backend/internal/flow/skillpacks/education-math", ["agent", "course"]),
    ("industry-packs/education-math/", ["agent", "course"]),
    ("backend/internal/flow/mathtools", ["agent", "math"]),
    ("backend/internal/flow/mcp_internal", ["agent", "permissions"]),
    ("backend/internal/flow/usage", ["cost"]),
    ("backend/internal/flow/billing", ["cost"]),
    ("backend/internal/database/migrations.go", ["permissions"]),
    ("backend/internal/database/flow_rls.go", ["permissions"]),
    ("backend/internal/model/models_edu.go", ["permissions"]),
]


def areas_for(path):
    for prefix, areas in RULES:
        if path.startswith(prefix):
            return set(areas)
    if path.startswith(EDU_ROOTS):
        return set(SUITES)  # unknown edu path: fail closed
    return set()


def required(changed):
    if changed is None:
        return set(SUITES), {"ALL": sorted(SUITES)}
    areas, why = set(), {}
    for p in changed:
        a = areas_for(p)
        if a:
            why[p] = sorted(a)
            areas |= a
    return areas, why


def results(events):
    ran, passed, bad = set(), set(), set()
    for e in events:
        if not e.get("Package", "").endswith("/edumath") or not e.get("Test") or "/" in e["Test"]:
            continue
        act = e.get("Action")
        if act == "run":
            ran.add(e["Test"])
        elif act == "pass":
            passed.add(e["Test"])
        elif act in ("fail", "skip"):
            bad.add(e["Test"])
    return (ran & passed) - bad


def admit(changed, events):
    areas, why = required(changed)
    ok = results(events)
    rows = []
    for area in sorted(areas):
        for t in SUITES[area]:
            rows.append({"area": area, "test": t, "passed": t in ok})
    return {"areas": sorted(areas), "because": why, "checks": rows, "admitted": all(r["passed"] for r in rows)}


def read_changed(path):
    try:
        with open(path, encoding="utf-8") as f:
            lines = [ln.strip() for ln in f if ln.strip()]
    except OSError:
        return None
    if not lines or lines == ["ALL"]:
        return None if lines == ["ALL"] else []
    return lines


def main(argv):
    with open(argv[1], encoding="utf-8") as f:
        events = [json.loads(ln) for ln in f if ln.strip().startswith("{")]
    report = admit(read_changed(argv[2]), events)
    with open(argv[3], "w", encoding="utf-8") as f:
        json.dump(report, f, ensure_ascii=False, indent=2)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as f:
            f.write("### Education upgrade admission\n\n")
            if not report["areas"]:
                f.write("No solver, render, agent, course, cost or permission path changed.\n")
            else:
                f.write("| area | test | result |\n|---|---|---|\n")
                for r in report["checks"]:
                    f.write("| %s | %s | %s |\n" % (r["area"], r["test"], "pass" if r["passed"] else "MISSING/FAILED"))
    if not report["admitted"]:
        missing = sorted({r["test"] for r in report["checks"] if not r["passed"]})
        sys.exit("Upgrade admission refused; dependent suites missing or failed: " + ", ".join(missing))
    print("Upgrade admission: areas %s, %d checks passed." % (report["areas"] or "none", len(report["checks"])))


if __name__ == "__main__":
    main(sys.argv)
