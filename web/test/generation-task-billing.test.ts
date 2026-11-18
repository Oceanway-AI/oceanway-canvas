import { describe, expect, test } from "bun:test";

import { taskBillingDisplay, taskBillingLabel } from "@/lib/generation-task-billing";
import type { GenerationTask } from "@/services/api/task-center";

type Billing = NonNullable<GenerationTask["platformBilling"]>;
const receipt: Billing = { fundsState: "settled", stateAvailable: true, summary: "" };

describe("Platform task funds", () => {
    test("preserves exact decimal settlement and an explicit zero", () => {
        for (const settledPoints of ["0", "0.000000000000000001", "9007199254740993.123456789012345678"]) {
            expect(taskBillingLabel({ ...receipt, settledPoints })).toBe(`平台已结算 ${settledPoints} 积分`);
        }
    });

    test("missing or malformed settlement cannot become a zero charge", () => {
        for (const settledPoints of [undefined, "", "NaN", "-1", "1e3", "01", "1".repeat(129)]) {
            expect(taskBillingDisplay({ ...receipt, settledPoints })).toEqual({ amount: null, note: "平台已结算，金额待确认", tone: "uncertain" });
        }
        expect(taskBillingDisplay({ ...receipt, settledPoints: "0", stateAvailable: false }).amount).toBeNull();
    });

    test("reservation and release have no invented money amount", () => {
        expect(taskBillingLabel({ ...receipt, fundsState: "reserved" })).toBe("平台资金已预留");
        expect(taskBillingLabel({ ...receipt, fundsState: "released" })).toBe("平台资金已释放");
        expect(taskBillingLabel({ ...receipt, fundsState: "new-platform-state" })).toBe("平台费用状态待确认");
    });
});
