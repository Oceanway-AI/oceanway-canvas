import copy
import importlib.util
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("studio_release", ROOT / "scripts/studio_release.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class IdentityRegistrationTests(unittest.TestCase):
    def setUp(self):
        self.config = {
            "cookie_domain": "oceanway.tech", "cookie_host": "test.oceanway.tech",
            "cookie_name": "__Host-oceanway-w7-session",
            "allowed_origins": ["https://test.oceanway.tech:18447"],
            "return_origins": ["https://test.oceanway.tech:18447"],
            "products": [{"key_id": "existing", "product": "studio", "secret": "existing-secret",
                          "actions": ["operation:read"], "origins": ["https://test.oceanway.tech:18447"]}],
        }

    def test_adds_only_test_registration_without_mutating_input(self):
        before = copy.deepcopy(self.config)
        updated = release.register_test_studio(self.config, "a" * 64)
        self.assertEqual(self.config, before)
        self.assertEqual(updated["products"][0], before["products"][0])
        self.assertEqual(updated["products"][1]["origins"], [release.TEST_ORIGIN])
        self.assertEqual(updated["products"][1]["actions"], ["model:invoke", "operation:read"])
        self.assertIn(release.TEST_ORIGIN, updated["return_origins"])

    def test_idempotent_without_rotating_key(self):
        updated = release.register_test_studio(self.config, "a" * 64)
        self.assertEqual(release.register_test_studio(updated, "a" * 64), updated)
        with self.assertRaisesRegex(ValueError, "existing"):
            release.register_test_studio(updated, "b" * 64)

    def test_rejects_production_cookie_and_revoked_credential(self):
        for field, value in [("cookie_host", "console.oceanway.tech"), ("cookie_name", "session")]:
            config = copy.deepcopy(self.config)
            config[field] = value
            with self.assertRaises(ValueError):
                release.register_test_studio(config, "a" * 64)
        updated = release.register_test_studio(self.config, "a" * 64)
        updated["products"][-1]["revoked"] = True
        with self.assertRaises(ValueError):
            release.register_test_studio(updated, "a" * 64)

    def test_rejects_short_or_multiline_secret(self):
        for secret in ["", "abc", "a" * 64 + "\n"]:
            with self.assertRaises(ValueError):
                release.register_test_studio(self.config, secret)


class DeploymentTests(unittest.TestCase):
    def test_private_sdk_uses_buildkit_secret_not_build_argument(self):
        dockerfile = (ROOT / "backend/Dockerfile").read_text()
        self.assertIn("--mount=type=secret,id=github_netrc", dockerfile)
        self.assertNotIn("ARG GITHUB_TOKEN", dockerfile)
        self.assertNotIn("COPY .netrc", dockerfile)

    def test_uses_upstream_topology_and_injects_server_only_configuration(self):
        model = release.compose_model(ROOT)
        services = model["services"]
        self.assertEqual(set(services), {"postgres", "redis", "migrate", "yingce-agent", "backend", "web"})
        for key in release.STUDIO_ENV:
            self.assertEqual(services["backend"]["environment"][key], release.fixture_env()[key])
            self.assertNotIn(key, services["web"].get("environment", {}))
        self.assertEqual(services["backend"]["environment"]["CANVAS_DATABASE_DRIVER"], "postgres")
        self.assertEqual(services["backend"]["environment"]["YINGCE_AGENT_URL"], "http://yingce-agent:8081")
        self.assertEqual(services["backend"]["environment"]["CANVAS_UPDATER_TOKEN"], "")
        self.assertFalse(any("updater" in v.get("target", "") for v in services["backend"].get("volumes", [])))
        self.assertEqual(services["web"]["ports"][0]["host_ip"], "127.0.0.1")
        for name in ["backend", "migrate", "yingce-agent", "web"]:
            self.assertTrue(services[name]["image"].startswith("ghcr.io/oceanway-ai/oceanway-canvas-"))
            self.assertIn("@sha256:", services[name]["image"])

    def test_full_host_survives_every_nginx_proxy(self):
        for name in ["nginx.conf", "deploy/studio/nginx.test.conf"]:
            text = (ROOT / name).read_text()
            self.assertNotIn("proxy_set_header Host $host;", text)
            self.assertIn("proxy_set_header Host $http_host;", text)
        edge = (ROOT / "deploy/studio/nginx.test.conf").read_text()
        self.assertIn("listen 8443 ssl", edge)
        self.assertNotIn("console.oceanway.tech", edge)
        self.assertNotIn("$request_uri", edge)
        self.assertNotIn("$http_referer", edge)

    def test_legacy_gateway_bridge_exposes_no_admin_or_identity_routes(self):
        edge = (ROOT / "deploy/studio/nginx.test.conf").read_text()
        self.assertIn("listen 8444 ssl", edge)
        self.assertIn("proxy_pass http://45.58.184.2:18420;", edge)
        self.assertIn("location / { return 404; }", edge)
        self.assertNotIn("proxy_set_header Authorization", edge)
        self.assertNotIn("proxy_pass_request_headers off", edge)


if __name__ == "__main__":
    unittest.main()
