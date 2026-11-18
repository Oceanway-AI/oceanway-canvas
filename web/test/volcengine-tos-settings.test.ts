import { describe, expect, test } from "bun:test";

import { normalizeVolcengineTOSLocation } from "../src/lib/oss-settings";

describe("Volcengine TOS storage settings", () => {
    test("matches the backend endpoint normalization", () => {
        const cases: Array<[{ region: string; endpoint: string; bucket: string }, { region: string; endpoint: string }]> = [
            [
                { region: "cn-beijing", endpoint: "", bucket: "" },
                { region: "cn-beijing", endpoint: "https://tos-s3-cn-beijing.volces.com" },
            ],
            [
                { region: "", endpoint: "tos-cn-shanghai.volces.com", bucket: "" },
                { region: "cn-shanghai", endpoint: "https://tos-s3-cn-shanghai.volces.com" },
            ],
            [
                { region: "", endpoint: "https://My-Bucket.tos-cn-guangzhou.volces.com", bucket: "my-bucket" },
                { region: "cn-guangzhou", endpoint: "https://tos-s3-cn-guangzhou.volces.com" },
            ],
            [
                { region: "AP-SOUTHEAST-1", endpoint: "https://tos-s3-ap-southeast-1.ivolces.com", bucket: "" },
                { region: "ap-southeast-1", endpoint: "https://tos-s3-ap-southeast-1.ivolces.com" },
            ],
            [
                { region: "", endpoint: "https://other.tos-cn-beijing.volces.com", bucket: "my-bucket" },
                { region: "", endpoint: "https://other.tos-cn-beijing.volces.com" },
            ],
            [
                { region: "cn-beijing", endpoint: "https://tos.example.com", bucket: "" },
                { region: "cn-beijing", endpoint: "https://tos.example.com" },
            ],
        ];
        for (const [input, expected] of cases) expect(normalizeVolcengineTOSLocation(input)).toEqual(expected);
    });
});
