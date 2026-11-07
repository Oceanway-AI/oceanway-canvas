"""Prepare the authorized W7 registration; never touches production Console."""
import hashlib
import json
import os
import pathlib
import secrets

from studio_release import TEST_ORIGIN, register_test_studio


def private_write(path, text):
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w") as output:
        output.write(text)
        output.flush()
        os.fsync(output.fileno())
    os.chmod(path, 0o600)


def main():
    source = pathlib.Path("/opt/oceanway-w7-test/private/identity.json")
    private = pathlib.Path("/opt/oceanway-studio-preview/private")
    private.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(private, 0o700)
    raw = source.read_bytes()
    before = hashlib.sha256(raw).hexdigest()
    config = json.loads(raw)
    key_file = private / "platform-product-key"
    key = key_file.read_text().strip() if key_file.exists() else secrets.token_hex(32)
    updated = register_test_studio(config, key)
    payload = json.dumps(updated, indent=2) + "\n"
    if not key_file.exists():
        private_write(key_file, key + "\n")
    backup = private / ("w7-identity-before-" + before + ".json")
    if not backup.exists():
        private_write(backup, raw.decode())
    # Detect concurrent registration changes immediately before writing.
    if hashlib.sha256(source.read_bytes()).hexdigest() != before:
        raise SystemExit("W7 identity changed concurrently; no configuration applied")
    changed = config != updated
    if changed:
        # Keep the bind-mounted inode. The W7 app reloads this file on restart.
        private_write(source, payload)
    private_write(private / "studio-platform.env", "\n".join([
        "STUDIO_PLATFORM_ORIGIN=https://test.oceanway.tech:18447",
        "STUDIO_PLATFORM_PRODUCT_KEY=studio-preview-20261108." + key,
        "STUDIO_PUBLIC_ORIGIN=" + TEST_ORIGIN,
        "STUDIO_PLATFORM_SESSION_HOSTS=test.oceanway.tech:18448",
        "STUDIO_PLATFORM_ALLOWED_ORIGINS=" + TEST_ORIGIN,
    ]) + "\n")
    print(json.dumps({"changed": changed, "origin": TEST_ORIGIN, "identity_before_sha256": before,
                      "identity_after_sha256": hashlib.sha256(source.read_bytes()).hexdigest(),
                      "production_console_modified": False}))


if __name__ == "__main__":
    main()
