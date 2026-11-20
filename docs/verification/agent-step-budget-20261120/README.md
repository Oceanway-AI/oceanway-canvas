# Studio Agent 分镜轮次执行上限

2026-11-20，生产故障只读诊断及独立工作区回归。采用 `oceanway-devops` 2.4.2 开发、运行、测试步骤，Docs `7e11cf5c` 的 PDOC-STUDIO-03-002 v2.3 和 FS-STUDIO-001 v1.8。复用现有 Pi、Task 与画布写入规则，本次独立缺陷记录不变更已交付的 WP。

## 生产观测

- Studio 后端与 Pi 容器均标记源码 `4438e99d679905ed4e912c6f4a7aecb1f6dc0603`，采样时重启计数均为 0。
- 后端日志在 3 个不同轮次记录了 `step budget exhausted`。
- 只读查询 `cloud_agent_executions` 和 `tasks` 得到请求上限 16、已用 16、16 个成功的模型子任务、根轮次 `failed`、`cleanup_pending=false`。根轮次无活动模型或媒体任务。
- 按 `run_id` 读取事件类型与工具名，得到 10 次 `canvas_read_storyboard`、2 次 `canvas_get_state` 和 3 次 `canvas_edit_storyboard`。变更表中 3 条编辑均为 `applied`，`undone_at` 为空。最终计划为 1 项进行中、2 项待办。核对时仅查询工具名、状态与聚合数值。

## 处理与验证

Pi 用尽步数后曾返回内部英文错误，经公开错误转换变成通用重发提示。候选修复沿用既有预算和终态，在最后两次普通模型请求附上剩余次数（含本次），达到上限时给出具体次数并指引核对已执行操作。

独立本机 PostgreSQL 夹具与模拟模型结果。生产源码上两条新用例先失败，分别重现通用提示和模型输入缺少剩余次数。修正后执行：

```sh
go test -race ./internal/app -count=1 -run 'TestCloudAgent(Pi(StepLimit|UpcomingStepLimit|Interjection|SteeredContinuation|PlanNudge|CompactionSummary)|UserFailureMessage)'
```

结果 `ok yingce/backend/internal/app 14.126s`。范围为源码候选，不含生产部署及真实模型调用。下一步为正常发布后的用户界面与真实轮次观察。

## 新轮次按实际用量结算

故障发生时生产前端默认 16 步，前后端均校验 1 至 64。用户随后确定移除执行上限设置，每轮按实际消费结算。当前候选已移除设置入口。画布、独立 Agent 和本地队列的新消息均省略 `budget`，Studio 接受无上限的新轮次。历史已提交请求按原始正文与幂等键核对。

本地 PostgreSQL 与模拟 Platform 回归中，首轮顺序执行 17 次模型调用，续轮执行 1 次。测试核对两轮分别申请与释放授权，18 个持久请求号各提交一次，每步保存对应回执中的用量。原生 Pi 文本及媒体审批、丢回执后按原请求查询、资金结算精度和旧显式上限共 8 项 Go race 专项通过。前端面板、续轮发送和旧请求重试等 3 个测试文件通过，类型检查和 lint 通过。该批采用模拟计费用量，生产发布与真实账单验收待执行。
