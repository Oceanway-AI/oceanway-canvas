"""Render and validate the isolated Studio deployment without printing secrets."""
import copy
import json
import os
import pathlib
import subprocess
import hmac

TEST_ORIGIN = "https://test.oceanway.tech:18448"
STUDIO_ENV = ("STUDIO_PLATFORM_ORIGIN", "STUDIO_PLATFORM_PRODUCT_KEY", "STUDIO_PUBLIC_ORIGIN",
              "STUDIO_PLATFORM_SESSION_HOSTS", "STUDIO_PLATFORM_ALLOWED_ORIGINS")


def register_test_studio(config, secret):
    if (config.get("cookie_host") != "test.oceanway.tech" or
            config.get("cookie_name") != "__Host-oceanway-w7-session"):
        raise ValueError("only the isolated W7 identity configuration is supported")
    if len(secret) < 48 or not secret.isascii() or not secret.isalnum():
        raise ValueError("a private random alphanumeric product credential is required")
    updated = copy.deepcopy(config)
    credential = {"key_id": "studio-preview-20261108", "product": "studio", "secret": secret,
                  "actions": ["model:invoke", "operation:read"], "origins": [TEST_ORIGIN]}
    existing = [p for p in updated["products"] if p.get("key_id") == credential["key_id"]]
    if existing:
        value = existing[0]
        if len(existing) != 1 or value.get("revoked") or not hmac.compare_digest(value.get("secret", ""), secret) or value != credential:
            raise ValueError("existing credential differs or was revoked; no automatic replacement")
    else:
        updated["products"].append(credential)
    for field in ["allowed_origins", "return_origins"]:
        if TEST_ORIGIN not in updated[field]:
            updated[field].append(TEST_ORIGIN)
    return updated


def fixture_env():
    return {
        "POSTGRES_PASSWORD": "fixture-password-only",
        "DATABASE_URL": "postgres://studio:fixture-password-only@postgres:5432/studio?sslmode=disable",
        "CANVAS_IMAGE_TAG": "fixture",
        "CANVAS_BACKEND_IMAGE": "ghcr.io/oceanway-ai/oceanway-canvas-backend@sha256:" + "1" * 64,
        "CANVAS_YINGCE_AGENT_IMAGE": "ghcr.io/oceanway-ai/oceanway-canvas-agent@sha256:" + "2" * 64,
        "CANVAS_WEB_IMAGE": "ghcr.io/oceanway-ai/oceanway-canvas-web@sha256:" + "3" * 64,
        "YINGCE_AGENT_TOKEN": "fixture-agent-only",
        "STUDIO_PLATFORM_ORIGIN": "https://test.oceanway.tech:18447",
        "STUDIO_PLATFORM_PRODUCT_KEY": "fixture-product-only",
        "STUDIO_PUBLIC_ORIGIN": TEST_ORIGIN,
        "STUDIO_PLATFORM_SESSION_HOSTS": "test.oceanway.tech:18448",
        "STUDIO_PLATFORM_ALLOWED_ORIGINS": TEST_ORIGIN,
    }


def compose_model(root):
    args = ["docker", "compose", "--env-file", "/dev/null", "-f", str(root / "docker-compose.deploy.yml")]
    overlay = root / "deploy/studio/compose.override.yaml"
    if overlay.exists():
        args += ["-f", str(overlay)]
    result = subprocess.run(args + ["config", "--format", "json"], capture_output=True, text=True,
                            env={"PATH": os.environ["PATH"], "HOME": os.environ["HOME"], **fixture_env()}, check=True)
    return json.loads(result.stdout)
