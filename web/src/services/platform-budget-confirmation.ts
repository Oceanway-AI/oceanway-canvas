export type PlatformBudgetConfirmation = (estimate: string, title: string) => Promise<string | null>;
let confirmBudget: PlatformBudgetConfirmation | undefined;

// A single app-level dialog host serves native generation and Agent quotation.
// No request, task or authorization is created by this presentation bridge.
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

export function validPlatformBudget(maximum: string, estimate: string) {
    const decimal = /^(0|[1-9]\d*)(\.\d+)?$/;
    if (maximum.length > 128 || estimate.length > 128 || !decimal.test(maximum) || !decimal.test(estimate)) return false;
    const [a, af = ""] = maximum.split(".");
    const [b, bf = ""] = estimate.split(".");
    const digits = Math.max(af.length, bf.length);
    return BigInt(a + af.padEnd(digits, "0")) >= BigInt(b + bf.padEnd(digits, "0"));
}
