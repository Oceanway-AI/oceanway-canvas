# Agent 历史视频等待结算

Studio `v2.0.11` 的诊断和 Platform 的执行、归档及资金记录显示，视频成功后，旧任务仍可能停留在 `awaiting_settlement`。

- 在同一原单上核对 `execution_state=succeeded`、原件 `saved`、`funds_state=reserved` 和最终账单。
- 当实耗超过旧授权上限时，由原付款人在任务中心确认平台返回的精确总额。重复提交应复用原确认。
- 生成时的画布节点可能已经删除。通过任务中心的原任务核对结算与原件交付。

Studio 使用 `platformclient v1.0.0-rc.6` 的原单确认接口。专项测试覆盖任务详情、付款人归属、金额校验、重复确认和回读。
