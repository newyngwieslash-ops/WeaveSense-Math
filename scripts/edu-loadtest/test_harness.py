#!/usr/bin/env python3
# Automatic checks for the education load-test scaffold (E95). Does not issue
# live traffic and is not a formal capacity report.
import os
from pathlib import Path
import subprocess
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


class EduLoadtestScaffold(unittest.TestCase):
    def test_run_scaffold_only(self):
        out = subprocess.check_output([os.path.join(HERE, "run.sh"), "--scaffold-only"], text=True)
        self.assertIn("edu loadtest scaffold OK", out)
        self.assertIn("job_accept p95<800ms", out)
        self.assertIn("auth_rw p95<400ms", out)

    def test_scripts_mention_declared_scenarios(self):
        k6 = Path(HERE, "k6-edu.js").read_text(encoding="utf-8")
        self.assertIn("problem-revisions", k6)
        self.assertIn("observability", k6)
        veg = Path(HERE, "vegeta-targets.txt").read_text(encoding="utf-8")
        self.assertNotIn("password", veg.lower())
        self.assertIn("BASE/api/edu/me", veg)


if __name__ == "__main__":
    sys.exit(unittest.main(verbosity=2))
