import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location("adm", pathlib.Path(__file__).with_name("edu-upgrade-admission.py"))
adm = importlib.util.module_from_spec(spec)
spec.loader.exec_module(adm)


def passing(tests):
    ev = []
    for t in tests:
        ev.append({"Package": "yingce/backend/internal/edumath", "Test": t, "Action": "run"})
        ev.append({"Package": "yingce/backend/internal/edumath", "Test": t, "Action": "pass"})
    return ev


ALL = sorted({t for ts in adm.SUITES.values() for t in ts})


class Admission(unittest.TestCase):
    def test_docs_only_needs_nothing(self):
        r = adm.admit(["docs/x.md", "web/src/a.tsx"], [])
        self.assertTrue(r["admitted"])
        self.assertEqual(r["areas"], [])

    def test_render_change_needs_render_suites(self):
        r = adm.admit(["services/math-service/mathsvc/render.py"], passing(adm.SUITES["render"]))
        self.assertEqual(r["areas"], ["render"])
        self.assertTrue(r["admitted"])

    def test_missing_suite_refuses(self):
        r = adm.admit(["services/math-service/mathsvc/families/solid_angle_02.py"], passing(adm.SUITES["math"][1:]))
        self.assertFalse(r["admitted"])

    def test_skip_or_fail_refuses(self):
        ev = passing(adm.SUITES["cost"]) + [{"Package": "x/edumath", "Test": "TestReceiptContract", "Action": "skip"}]
        self.assertFalse(adm.admit(["backend/internal/edumath/ledger.go"], ev)["admitted"])

    def test_other_package_does_not_count(self):
        ev = [{"Package": "x/other", "Test": t, "Action": a} for t in adm.SUITES["course"] for a in ("run", "pass")]
        self.assertFalse(adm.admit(["backend/internal/edumath/courses.go"], ev)["admitted"])

    def test_unknown_edu_path_fails_closed(self):
        r = adm.admit(["backend/internal/edumath/newthing.go"], passing(adm.SUITES["math"]))
        self.assertEqual(r["areas"], sorted(adm.SUITES))
        self.assertFalse(r["admitted"])
        self.assertTrue(adm.admit(["backend/internal/edumath/newthing.go"], passing(ALL))["admitted"])

    def test_skill_pack_and_price_paths_are_mapped(self):
        self.assertEqual(adm.required(["backend/internal/flow/skillpacks/education-math.json"])[0], {"agent", "course"})
        self.assertEqual(adm.required(["backend/internal/flow/billing.go"])[0], {"cost"})
        self.assertEqual(adm.required(["industry-packs/education-math/new.md"])[0], {"agent", "course"})

    def test_unknown_change_set_requires_all(self):
        self.assertEqual(adm.required(None)[0], set(adm.SUITES))


if __name__ == "__main__":
    unittest.main()
