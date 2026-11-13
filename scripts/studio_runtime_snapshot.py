"""Read-only fingerprint of existing services. No environment or credentials."""
import hashlib
import json
import pathlib
import subprocess


def main():
    ids = subprocess.check_output(["docker", "ps", "-aq"], text=True).split()
    containers = json.loads(subprocess.check_output(["docker", "inspect", *ids])) if ids else []
    result = {"containers": {}, "nginx": {}}
    for item in containers:
        name = item["Name"].lstrip("/")
        if name.startswith("oceanway-studio-preview-"):
            continue
        result["containers"][name] = {
            "id": item["Id"], "image": item["Image"],
            "started_at": item["State"]["StartedAt"], "status": item["State"]["Status"],
        }
    for path in pathlib.Path("/etc/nginx").rglob("*"):
        if path.is_file() and not path.is_symlink():
            result["nginx"][str(path)] = hashlib.sha256(path.read_bytes()).hexdigest()
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
