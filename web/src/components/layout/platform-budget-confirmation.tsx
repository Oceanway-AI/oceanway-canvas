import { useEffect } from "react";
import { App, Input } from "antd";
import { installPlatformBudgetConfirmation, validPlatformBudget } from "@/services/platform-budget-confirmation";

export function PlatformBudgetConfirmationHost() {
    const { modal, message } = App.useApp();
    useEffect(() => {
        const pending = new Set<() => void>();
        const uninstall = installPlatformBudgetConfirmation(
            (estimate, title) =>
                new Promise((resolve) => {
                    let maximum = estimate;
                    const finish = (value: string | null) => {
                        pending.delete(cancel);
                        resolve(value);
                    };
                    const dialog = modal.confirm({
                        title,
                        content: (
                            <div>
                                <p>预估 {estimate} 平台积分。最终按实际用量结算，可能高于预估。</p>
                                <label>
                                    本次最高消费（平台积分）
                                    <Input
                                        aria-label="本次最高消费（平台积分）"
                                        defaultValue={estimate}
                                        inputMode="decimal"
                                        onChange={(event) => {
                                            maximum = event.target.value.trim();
                                        }}
                                    />
                                </label>
                                <p>可调整上限；超出上限时暂停结算，不自动提高额度或重新生成。</p>
                            </div>
                        ),
                        okText: "确认额度",
                        cancelText: "取消",
                        onOk: () => {
                            if (!validPlatformBudget(maximum, estimate)) {
                                void message.error("最高消费须为不低于预估的有效积分数");
                                return Promise.reject(new Error("invalid maximum"));
                            }
                            finish(maximum);
                        },
                        onCancel: () => finish(null),
                    });
                    const cancel = () => {
                        dialog.destroy();
                        finish(null);
                    };
                    pending.add(cancel);
                }),
        );
        return () => {
            uninstall();
            for (const cancel of pending) cancel();
        };
    }, [modal, message]);
    return null;
}
