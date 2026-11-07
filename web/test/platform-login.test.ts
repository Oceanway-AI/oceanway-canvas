import { expect, test } from "bun:test";
import { platformLoginNext } from "../src/lib/platform-login";

test("Platform callback returns only to a local non-auth page", () => {
    expect(platformLoginNext("/canvas/123?tab=details")).toBe("/canvas/123?tab=details");
    for (const value of [undefined, "https://evil.test", "//evil.test", "/\\evil.test", "/\nevil.test", "/login", "/platform/login/complete?next=/canvas"]) {
        expect(platformLoginNext(value)).toBe("/");
    }
});
