// No create request has been sent when a user declines this confirmation.
export class PlatformBudgetDeclined extends Error {
    constructor() {
        super("已取消本次消费确认");
        this.name = "AbortError";
    }
}

export function validPlatformPoints(value: unknown): value is string {
    return typeof value === "string" && value.length <= 128 && /^(0|[1-9]\d*)(\.\d+)?$/.test(value);
}

export function validPlatformBudget(maximum: string, estimate: string) {
    if (!validPlatformPoints(maximum) || !validPlatformPoints(estimate)) return false;
    const [a, af = ""] = maximum.split(".");
    const [b, bf = ""] = estimate.split(".");
    const digits = Math.max(af.length, bf.length);
    return BigInt(a + af.padEnd(digits, "0")) >= BigInt(b + bf.padEnd(digits, "0"));
}
