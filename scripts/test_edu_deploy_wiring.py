#!/usr/bin/env python3
"""Contracts for math-service image, compose wiring, and the app DB role."""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def read(rel: str) -> str:
    return (ROOT / rel).read_text(encoding="utf-8")


class MathImageTests(unittest.TestCase):
    def test_dockerfile_uses_slim_and_drops_privileges(self):
        text = read("services/math-service/Dockerfile")
        self.assertIn("FROM python:3.12-slim", text)
        self.assertIn("USER mathsvc", text)
        self.assertIn("MATH_LISTEN=0.0.0.0:8790", text)
        self.assertIn('CMD ["python", "-m", "mathsvc.server"]', text)
        self.assertNotIn("DATABASE_URL", text)
        self.assertNotIn("FLOW_MATH_SERVICE_KEY", text)
        self.assertIn("COPY services/math-service/requirements.txt", text)


class StandinMediaWiringTests(unittest.TestCase):
    def test_production_compose_defaults_standin_off(self):
        self.assertIn("MATH_MEDIA_STANDIN: ${MATH_MEDIA_STANDIN:-0}", read("docker-compose.deploy.yml"))
        self.assertIn("MATH_MEDIA_STANDIN: ${MATH_MEDIA_STANDIN:-0}", read("docker-compose.server.yml"))

    def test_dev_compose_defaults_standin_on(self):
        self.assertIn("MATH_MEDIA_STANDIN: ${MATH_MEDIA_STANDIN:-1}", read("docker-compose.dev.yml"))
        self.assertIn("MATH_MEDIA_STANDIN: ${MATH_MEDIA_STANDIN:-1}", read("docker-compose.local.yml"))
        self.assertIn("MATH_MEDIA_STANDIN: ${MATH_MEDIA_STANDIN:-1}", read("docker-compose.yml"))


class EnsureAppRoleTests(unittest.TestCase):
    def test_script_creates_no_bypass_role_and_verifies(self):
        text = read("deploy/postgres/ensure-app-role.sh")
        for token in (
            "NOSUPERUSER",
            "NOBYPASSRLS",
            "NOCREATEDB",
            "ALTER DEFAULT PRIVILEGES",
            "rolsuper",
            "rolbypassrls",
            'f|f',
        ):
            self.assertIn(token, text)
        self.assertNotIn("CREATEDB", text.replace("NOCREATEDB", ""))
        self.assertNotIn("BYPASSRLS", text.replace("NOBYPASSRLS", "").replace("rolbypassrls", ""))


class SyncEnvTests(unittest.TestCase):
    def run_sync(self, path: Path) -> None:
        script = ROOT / "scripts" / "sync-edu-deploy-env.sh"
        subprocess.run(["bash", str(script), str(path)], check=True, cwd=ROOT)

    def parse_env(self, path: Path) -> dict[str, str]:
        data: dict[str, str] = {}
        for line in path.read_text(encoding="utf-8").splitlines():
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            data[key] = value
        return data

    def url_user(self, url: str) -> str:
        netloc = urlsplit(url).netloc
        if "@" not in netloc:
            return ""
        return unquote(netloc.rsplit("@", 1)[0].split(":", 1)[0])

    def test_new_env_splits_migrate_and_app_urls(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / ".env"
            self.run_sync(path)
            env = self.parse_env(path)
            self.assertEqual(env["APP_DB_USER"], "flow_app")
            self.assertEqual(env["POSTGRES_USER"], "open_ai_canvas")
            self.assertGreaterEqual(len(env["APP_DB_PASSWORD"]), 32)
            self.assertGreaterEqual(len(env["MATH_SERVICE_KEY"]), 32)
            self.assertEqual(env["MATH_SERVICE_KEY"], env["FLOW_MATH_SERVICE_KEY"])
            self.assertEqual(env["FLOW_MATH_SERVICE_URL"], "http://math:8790")
            self.assertEqual(env["EDU_MATH_REQUIRE_SERVICE"], "1")
            self.assertEqual(env["MATH_MEDIA_STANDIN"], "0")
            self.assertEqual(self.url_user(env["MIGRATE_DATABASE_URL"]), "open_ai_canvas")
            self.assertEqual(self.url_user(env["DATABASE_URL"]), "flow_app")
            self.assertNotEqual(env["APP_DB_PASSWORD"], env["POSTGRES_PASSWORD"])

    def test_existing_superuser_url_is_rewritten_once(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / ".env"
            path.write_text(
                "POSTGRES_DB=open_ai_canvas\n"
                "POSTGRES_USER=open_ai_canvas\n"
                "POSTGRES_PASSWORD=supersecret-supersecret-supersecret-12\n"
                "DATABASE_URL=postgresql://open_ai_canvas:supersecret-supersecret-supersecret-12@postgres:5432/open_ai_canvas?sslmode=disable\n",
                encoding="utf-8",
            )
            os.chmod(path, 0o600)
            self.run_sync(path)
            env = self.parse_env(path)
            self.assertEqual(
                self.url_user(env["MIGRATE_DATABASE_URL"]),
                "open_ai_canvas",
            )
            self.assertIn("supersecret-supersecret-supersecret-12", env["MIGRATE_DATABASE_URL"])
            self.assertEqual(self.url_user(env["DATABASE_URL"]), "flow_app")
            self.assertNotIn("supersecret-supersecret-supersecret-12", env["DATABASE_URL"])
            # second run is stable
            self.run_sync(path)
            env2 = self.parse_env(path)
            self.assertEqual(env2["DATABASE_URL"], env["DATABASE_URL"])
            self.assertEqual(env2["MIGRATE_DATABASE_URL"], env["MIGRATE_DATABASE_URL"])
            self.assertEqual(env2["APP_DB_PASSWORD"], env["APP_DB_PASSWORD"])
            self.assertEqual(env2["MATH_SERVICE_KEY"], env["MATH_SERVICE_KEY"])


class ComposeWiringTests(unittest.TestCase):
    def test_deploy_compose_wires_math_and_app_role(self):
        text = read("docker-compose.deploy.yml")
        self.assertIn("CANVAS_MATH_IMAGE", text)
        self.assertIn("FLOW_MATH_SERVICE_URL", text)
        self.assertIn("FLOW_MATH_SERVICE_KEY", text)
        self.assertIn("FLOW_PRICE_BOOK", text)
        self.assertIn("EDU_MATH_REQUIRE_SERVICE", text)
        self.assertIn("MIGRATE_DATABASE_URL", text)
        self.assertIn("postgres-app-role", text)
        self.assertIn("postgres-app-grants", text)
        self.assertIn("ensure-app-role.sh", text)
        self.assertIn("MATH_SERVICE_KEY", text)
        # math must not receive the platform database URL
        import re as _re
        match = _re.search(r"^  math:\n(?:    .*\n)+", text, _re.M)
        self.assertIsNotNone(match)
        math_block = match.group(0)
        self.assertIn("MATH_LISTEN", math_block)
        self.assertNotIn("DATABASE_URL", math_block)
        self.assertNotIn("POSTGRES_PASSWORD", math_block)
        self.assertNotIn("CANVAS_UPDATER_TOKEN", math_block)

    def test_build_compose_includes_math(self):
        text = read("docker-compose.build.yml")
        self.assertIn("services/math-service/Dockerfile", text)

    def test_dev_and_local_compose_pass_edu_env(self):
        for rel in (
            "docker-compose.dev.yml",
            "docker-compose.local.yml",
            "docker-compose.yml",
            "docker-compose.server.yml",
        ):
            text = read(rel)
            self.assertIn("FLOW_MATH_SERVICE_URL", text, rel)
            self.assertIn("FLOW_MATH_SERVICE_KEY", text, rel)
            self.assertIn("FLOW_PRICE_BOOK", text, rel)

    def test_install_scripts_call_sync_and_math_image(self):
        source = read("scripts/install-server.sh")
        image = read("scripts/install-server-image.sh")
        for text in (source, image):
            self.assertIn("sync-edu-deploy-env.sh", text)
        self.assertIn("CANVAS_MATH_IMAGE", image)
        self.assertIn("ensure-app-role.sh", image)
        self.assertIn("backend web math", source)


class HandoverDockerfileListTests(unittest.TestCase):
    def test_handover_checks_math_dockerfile(self):
        text = read(".github/scripts/check-edu-handover.py")
        self.assertIn('services/math-service/Dockerfile', text)

    def test_publish_and_quality_include_math_image(self):
        publish = read(".github/workflows/publish-images.yml")
        quality = read(".github/workflows/quality.yml")
        self.assertIn("weavesense-math-math-service", publish)
        self.assertIn("services/math-service/Dockerfile", publish)
        self.assertIn("services/math-service/Dockerfile", quality)


if __name__ == "__main__":
    unittest.main()
