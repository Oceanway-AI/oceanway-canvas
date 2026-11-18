# 任务准入与 Agent 本地账务退出

本批基于 `c35dc79` 接续 WP-STUDIO-020，在 PostgreSQL 独立测试 schema 中验证。

- `CreateTask` 和存储配额事务移除本地订单预留。内部 `prepareOnly` 保留无写入的准入准备，任务 ID、Agent 轮次、生成和审批 ID 继续由服务端绑定。
- 创作提交保留请求快照、提案批准、租约、版本复核、任务绑定和并发去重。通用创作报价退出微积分金额与本地计费方式，显示 Platform 报价暂不可用。
- Agent 单项及批次退出旧订单预算累加和费用投影，保留任务次数、视频时长、并发、试跑、结果回填和租约。平台原生媒体仍经 `enqueueStudioAgentTask` 与原报价审批执行。
- 旧媒体准备态版本升为 2，版本 1 需重新审批。依赖及规格哈希、角色版本和 Resource 签名继续复核。原生 Pi 加密报价沿独立合同。
- 短剧任务与作业绑定事务解除订单参数，本地短剧账务汇总及渠道价格依赖仍在后续清理范围。

验证结果如下。

| 范围 | 命令与结果 |
| --- | --- |
| PostgreSQL 创作与 Agent | `go test ./internal/app -run 'Test(Creation\|AgentMedia\|CloudAgent.*(Media\|Preflight\|ImageSize\|Budget\|ApprovalSettings\|Idempotency\|Conversation\|Exclusivity)\|DramaTask)' -count=1`，通过，59.545 秒 |
| Platform 原生审批与恢复 | app／handler 定向 `go test -race`，通过，5.954／19.187 秒。覆盖 Platform 资金权威、Pi 媒体报价、原文本请求恢复与未知提交恢复 |
| 最终内部准备标记 | 四项并发创作、Agent 来源、自动审批与媒体批次专项，通过，7.132 秒 |
| 前端 | 创作恢复、工具审批、分镜提交和重新生成四个测试文件，30 项、126 断言通过；`bun run typecheck` 通过 |

共享测试夹具删除本地账务表后，首轮暴露三处旧测试直接查询 `billing_orders`。已改为检查真实任务、审批、画布状态和幂等结果，重跑通过。旧 Token 账单授权测试随已退出功能删除，Platform 报价冻结与资金权威专项继续执行。

本批为本机源码验证。自有 Release、真实平台全矩阵和生产升级仍需按最终提交接续，WP 保持 doing。
