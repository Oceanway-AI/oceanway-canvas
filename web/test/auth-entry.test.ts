import { expect, test } from "bun:test";
import { platformLoginPage } from "../src/lib/platform-login";

test("登录链接只携带站内工作区地址，并保留查询和锚点", () => {
    expect(platformLoginPage("/")).toBe("/login");
    expect(platformLoginPage("/canvas/project?tab=1#node")).toBe("/login?next=%2Fcanvas%2Fproject%3Ftab%3D1%23node");
    for (const unsafe of ["//other.test", "/settings/../LOGIN/", "/x/..//other.test", "/platform/login/complete/"]) {
        expect(platformLoginPage(unsafe)).toBe("/login");
    }
});
