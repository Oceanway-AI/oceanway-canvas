// No create request has been sent when a user declines this confirmation.
export class PlatformBudgetDeclined extends Error {
    constructor() {
        super("已取消本次消费确认");
        this.name = "AbortError";
    }
}

export type PlatformBudgetConfirmation = (estimate: string, title: string) => Promise<string | null>;
let confirmBudget: PlatformBudgetConfirmation | undefined;

// The Agent media approval flow uses this dialog to collect its spending limit.
export function installPlatformBudgetConfirmation(handler: PlatformBudgetConfirmation) {
    confirmBudget = handler;
    return () => {
        if (confirmBudget === handler) confirmBudget = undefined;
    };
}
export function confirmPlatformBudget(estimate: string, title: string) {
    if (!confirmBudget) return Promise.reject(new Error("消费确认暂不可用，请刷新页面"));
    return confirmBudget(estimate, title);
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
