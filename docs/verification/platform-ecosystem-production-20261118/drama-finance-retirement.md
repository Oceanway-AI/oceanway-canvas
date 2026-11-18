# 短剧本地账务退出

本批基于 `c45679c` 接续 WP-STUDIO-020。

短剧列表、制作台、精修画布侧栏和 Agent 上下文退出本地预算、花费及冻结金额。创建、草稿和设置合同删除对应微积分字段。工作量估算保留集数、镜头、参考图、视频、文本调用数与时长。

调度器移除旧订单汇总、账单终态等待及预算暂停分支。任务和作业继续原子绑定，续任务保留开始时间。`RecordDramaJobAttempt` 继续使用 `attempts_json` 乐观锁，原任务处理记录先于结果应用落库。现有取消、失败重试、参考图、镜头、视频和成片调度测试在旧账务表已删除的 PostgreSQL schema 上执行。

待发布的 v100 迁移增加删除 `drama_productions.budget_microcredits/spent_microcredits` 与 `drama_jobs.cost_microcredits`。v98 升级测试种入旧列及数据，验证移除后保留任务绑定、运行状态和历次执行记录。生产当前仍为 v98，本批源码尚未发布。

| 验证 | 结果 |
| --- | --- |
| PostgreSQL `TestDrama*`、作业记录及 `TestStudioSchema*` | app 15.781 秒、database 8.551 秒通过。repository 编译通过，本次表达式未匹配测试 |
| 作业绑定及乐观锁 race 专项 | 通过，6.000 秒 |
| 前端类型检查 | `bun run typecheck` 通过 |
| 短剧纯逻辑与控件专项 | 17 项、10 断言通过 |

删除了随本地资金模型退出的短剧账务测试，保留并改写任务状态与并发断言。短剧完整生成仍属于尚未接通 Platform 的创作入口，生产开放需沿相应功能规格验收。
