# 账务与用户分组依赖退出

本批从 `6e5e570` 接续 WP-STUDIO-020。验证使用本机 PostgreSQL 独立 schema、假 Platform 和已安装的原生 Node Pi。

删除本地账单创建、预留、结算、成本计算、Token 公式报价、参考视频收费、积分策略及分组倍率实现。移除 `BillingOrder`、`CreditAccount`、`CreditLedgerEntry`、`UserGroup`、`UserGroupChannel` 及专用仓储。测试 schema 同步退出这些实体，原零财务写入断言改为核对退役表没有被重建。

任务活动数限制、重试状态校验、Agent 同会话并发准入继续由 task admission 仓储执行。线路切换移除账单调整，保留运行状态和预期原线路 ID 的条件更新。新用例验证正常更新、迟到切换拒绝、终态拒绝及主体和重试世代保持。

模型目录与创作调用方移除用户分组查询、兑换比例和本地价格展示。正式 Studio 目录、任务授权与报价沿 Platform。保留源码的能力目录继续用于规格与默认参数，旧上游 SKU 映射和定价结构的进一步拆除仍在接续清理范围。

Creation 审批签名改为 `CreationConfigurationSignature`，继续锁定并核对执行配置，移除价格表及 credit_policy 查询。现有审批配置变更、并发执行、失败回滚和刷新去重用例通过。

v100 待发布迁移追加移除 tasks 的 billing_order_id、authorized_charge_microcredits 及 api_call_logs 的 billing_order_id。v98 升级用例显式种入旧列，迁移与重复校验通过。

| 验证范围 | 实际结果 |
| --- | --- |
| 创作、能力目录、短剧、电商与 Pi 专项 | app 53.848 秒通过，database 7.617 秒通过，handler 2.792 秒通过 |
| 任务、Agent 模型与媒体、创作工作台及图片参数回归 | app 36.042 秒通过 |
| Platform 预算与 Pi 三项审批顶层测试 | race 30.796 秒通过，覆盖报价冻结、原协议请求保持和真实 Node 审批往返 |
| PostgreSQL 双连接 Agent 准入、线路切换 CAS | race 2.706 秒通过 |

首次双连接测试直接读取 Dialector DSN，未取得 testdb 的运行时连接配置，导致连接失败。已使用既有 `testdb.Reopen` 重建同一隔离 schema 的独立连接池，重验通过。

上批任务准入记录中的 Pi 覆盖表述已修正。本批明确执行 `TestStudioPiMediaApprovalFreezesQuoteAndSharesAuthority`、`TestStudioPiNativeMediaApprovalPreservesProtocolAndRequest` 和 `TestStudioPiRuntimeMediaApprovalRoundTrip`，不将测试名未匹配的旧批次算作 Pi 通过。

本批尚未进入自有 Release 和生产，完整质量检查与真实 Platform 联验继续以最终树执行。
