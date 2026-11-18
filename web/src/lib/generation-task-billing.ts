import type { GenerationTask } from "@/services/api/task-center";

type TaskBilling = NonNullable<GenerationTask["platformBilling"]>;

export type TaskBillingDisplay = {
    amount: string | null;
    note: string;
    tone: "settled" | "held" | "refunded" | "uncertain";
};

export function taskBillingDisplay(billing: TaskBilling): TaskBillingDisplay {
    if (!billing.stateAvailable) return { amount: null, note: "平台费用状态未知", tone: "uncertain" };
    switch (billing.fundsState) {
        case "settled": {
            const points = billing.settledPoints;
            if (typeof points !== "string" || points.length > 128 || !/^(0|[1-9]\d*)(\.\d+)?$/.test(points)) {
                return { amount: null, note: "平台已结算，金额待确认", tone: "uncertain" };
            }
            return { amount: points, note: "平台已结算", tone: "settled" };
        }
        case "reserved":
            return { amount: null, note: "平台资金已预留", tone: "held" };
        case "released":
            return { amount: null, note: "平台资金已释放", tone: "refunded" };
        default:
            return { amount: null, note: "平台费用状态待确认", tone: "uncertain" };
    }
}

export function taskBillingLabel(billing: TaskBilling) {
    const display = taskBillingDisplay(billing);
    return display.amount === null ? display.note : `${display.note} ${display.amount} 积分`;
}
