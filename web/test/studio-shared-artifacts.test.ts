import { expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

test.each([
    ["oceanway-account-panel-v1", "ea3f23f6709efc3ac85ad06dc22e757ef25c3c66"],
    ["oceanway-context-switcher-v1", "289386183b4479b15be4f8fa56dbe64cf0df5218"],
])("Studio consumes the accepted %s artifact without editing its source", (directory, commit) => {
    const root = resolve(import.meta.dir, "../src/vendor", directory);
    const manifest = JSON.parse(readFileSync(resolve(root, "artifact-manifest.json"), "utf8"));
    expect(manifest.source_commit).toBe(commit);
    for (const [path, digest] of Object.entries(manifest.files))
        expect(
            createHash("sha256")
                .update(readFileSync(resolve(root, path)))
                .digest("hex"),
        ).toBe(digest);
});
