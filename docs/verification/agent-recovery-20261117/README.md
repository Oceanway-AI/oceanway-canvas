# Agent 请求恢复与 OAC 接续验证

2026-11-17，Asia/Shanghai。Studio 修复已通过 PR #6 合入 main `577e3e6e90ecced43a3318b8baa7aa6e16a52712`，Platform 通过 PR #55 合入 main `86d4c90a070d176b43f758fae6fa753647fa385b`。SDK `platformclient/v1.0.0-rc.3` 已发行并完成空缓存下载，Studio 已固定该正式版本并以 `GOWORK=off` 验证。实际发布与原请求关闭回执见 `release.json`。

## 测试环境发布与原请求关闭

北京时间 15:15:30，ow-test 的 Platform API、素材 worker、Studio backend/web/Agent 五个服务完成切换。Studio 镜像源码为 `c200645`，与 main 的差异仅为一处测试夹具补表。Platform 镜像源码为 `53b41b1606c8`，源码树与合并后的 main 相同。五个服务运行正常，配置了健康检查的四个服务均 healthy，Studio schema 97 就绪，28 个其他运行容器的 ID、镜像和启动时间核对一致。Agent 并发上限设置为 8。

子步骤 `ag20305637a9766795b24ff88f5f78309a` 于 15:15:45 通过原请求恢复进入 failed，轮询次数停止在 5617。根轮次 `ag9225123585abceecd3e334cf035abd90` 为 cancelled，cleanup_pending 为 false，活动任务引用清空，根及两个子步骤执行授权均已释放。浏览器刷新后显示运行中 0，并展示原 Platform 请求号。原失败请求费用为 0，第一条成功文本请求的结算记录保持一致。本次关闭由部署后的正常恢复流程完成，直接数据库更新为 0，新模型提交为 0。

Studio 推送前完整验证为 11 项、失败 0，提供独立 PG 与 Redis，前端 3403 项通过。独立 Agent 静态检查及三个正式 Dockerfile 的 linux/amd64 构建通过。CI `44037362259` 通过 workflow_dispatch 检查本次固定 head，九项全部通过，覆盖五片后端、依赖用例检查、Agent 静态检查、支付插件 amd64 和前端检查。Platform CI `44033683672` 三项全部通过，前端 439 文件、5056 测试通过，原有一项手动 live PG 审计保持跳过，正式 HTTP/SDK/PG 专项 12 项通过。正式 SDK 独立 race 为 76 顶层、56 子例，Studio 正式 SDK 专项 race 为 104 顶层、74 子例，均无测试用例跳过。

发布验证包含原请求恢复、健康检查、HTTPS 页面与任务中心回读。本轮真实供应商两轮 Responses、新图片输入及长会话压缩仍待付费验收。下文保留发布前的源码与免费夹具验证记录，旧发行状态由本节和 `release.json` 更新。

## 版本与原故障

| 项目 | 固定版本 |
| --- | --- |
| OAC main | `3a746a85c69e35be2a0d00124f422afb3686e40a`，2026-11-17 再次通过 `git ls-remote` 核对 |
| Studio 基线 | `acfaa527f9b9dd799731218e6a8df51c6131d6a7` |
| Platform 基线 | `7ac5c2c669f21969963ffede7d6c21f02087ed0f` |
| Studio 分支 | `codex/studio-agent-recovery-20261117` |
| Platform 分支 | `codex/platform-agent-recovery-20261117` |

实际验收树为上述基线加本批未提交文件，文件摘要见 `source-files.json`，测试清单和命令见 `checks.json`。源码摘要按路径和文件 SHA256 排序计算，排除本证据目录以避免自引用。

根轮次 `ag9225123585abceecd3e334cf035abd90` 中，Platform 请求 `202611162152595072177948268d9d6kEtFyXi6` 对应第一次成功的模型调用。第二次调用的 Studio 子步骤为 `ag20305637a9766795b24ff88f5f78309a`，上游以 `Unknown parameter: 'input[3].item_reference'` 拒绝。争议字段已存在 Studio 冻结正文，正文 fingerprint 与 Platform 一致。Platform 将 FAILURE 且 Started=true 的记录投影为 pending，Studio 持续恢复该原请求，根轮次取消后仍等待子步骤收尾。

## 修复顺序与实现

| 顺序 | 修复内容 | 主要源码 |
| --- | --- | --- |
| 1 | Responses 两种工具历史转换删除非法 `item_reference`，保留 `id/call_id` 及结果配对 | `backend/internal/app/provider_agent_protocol.go` |
| 2 | Platform 保存安全失败码、原 HTTP 状态、未执行事实和重试许可，明确拒绝返回 failed，未知结果继续查询原请求 | Platform `service/platform_execution_text.go`、SDK `types.go` |
| 3 | Studio 核对恢复回执身份、协议、正文摘要和重试许可，识别历史 400 参数拒绝，超时恢复采用 30 秒退避 | `studio_execution_worker.go`、`studio_text_failure.go` |
| 4 | 异常退出标记待清理，终态清理先读控制列，损坏快照也能完成收尾，取消后的子任务终态驱动活动引用与授权释放 | `cloud_agent_pi_coordinator.go`、原恢复与执行 repository |
| 5 | 任务中心返回根轮次状态、子步骤关联、收尾状态、安全错误和原 Platform relay 请求号 | `studio_agent_task_output.go`、`task_output.go`、原任务列表与详情组件 |
| 6 | Platform 管理页显式配置文本上下文、输出和图片能力，Studio 从已发布目录冻结能力并覆盖客户端值 | Platform 原目录编辑器、`studio_agent.go`、`studio_catalog.go` |
| 7 | 普通文本及 Agent 通过固定账号 Resource、版本、binding 获取图片，核验文件后将图片字节放入三个文本协议 | `studio_text_vision.go`、`studio_execution.go` |
| 8 | 原响应 usage 回传 Pi，OpenAI 缓存 token 避免重复计数，确认上下文超限后交给 Pi 压缩，移除 Go 二次历史裁剪 | `studio_text_usage.go`、`cloud_agent_pi_model.go` |
| 9 | 同步 OAC 的 Pi 1.5.2、Go runner、独立 Agent 服务、临时目录生命周期及并发准入 | `backend/agent-runtime/pi/`、`backend/internal/agent/runtime/`、`yingce-agent/` |
| 10 | 成功响应超出工具数量或输出限制时进入原有有限纠偏，损坏工具参数保存结构化回执并禁止执行无效工具 | `cloud_agent_pi_model.go`、原 invalid-call 路径 |

模型能力由管理员明确填写。上下文窗口与输出上限用于本轮模型配置，图片能力包含数量和单图字节限制。Studio 获取 `download/original` 票据仅用于服务端读取、校验及压缩图片，模型收到内联图片字节。任务中心沿用现有列表和详情，以“Agent 轮次”“Agent 模型步骤”及根 ID 表达关系。

v1 恢复协议继续以 failed 表示确认未执行。网络超时、已提交但结果丢失等情形保留 pending 和原 RequestID，早先不确定的尝试会阻止后续明确拒绝覆盖其确定性。只有同时满足 `no_execution` 与 `retry_allowed` 的临时错误可进入新尝试。历史兼容只接受原始 HTTP 400 参数错误、匹配的协议与正文摘要，以及缺少确定性字段的旧失败记录。

## 验证范围

所有执行检查在 ow-test 的 linux/amd64 隔离目录完成。Go 1.25、PostgreSQL 16 专用数据库与非超级用户角色、Node 24.22.1 和 Bun 1.3.9 用于源码检查。正式 `yingce-agent/Dockerfile` 构建的镜像使用 Node 22.19、uid 100，实际运行一次免费模型桥、settled 事件和退出目录清理。

`checks.json` 记录每组最终状态、顶层测试与子用例数量、具名用例和日志 SHA256。重复执行的同名用例按同一项目和包去重。SDK 的两个 support 包没有测试文件，单独记录其 package skip，实际测试用例没有跳过。

| 最终检查 | 通过数量 |
| --- | --- |
| Studio Go 专项与 runtime race，去重 | 107 个顶层测试、74 个子用例 |
| Platform 失败、能力及隔离 PG race | 11 个顶层测试、20 个子用例 |
| SDK 全包 race | 75 个顶层测试、56 个子用例 |
| Studio 前端 | 44 项、186 个断言 |
| Platform 前端 | 77 项 |
| Pi Node 审批、压缩、生命周期 | 18 项 |
| 构建与静态检查 | 双仓前后端构建、相关 vet、前端 typecheck/lint、改动格式及契约闭包通过 |

验收源码共核对 Studio 51 个及 Platform 33 个变更代码/契约文件，本机与远端 SHA256 全部一致。专用 PostgreSQL 容器及其匿名卷已清理，源码、镜像与日志保留，环境回执见 `environment.json`。

专项覆盖两轮 Responses 工具历史及序列化恢复、明确拒绝、超时查询原请求、错误身份与摘要拒绝、跨进程恢复、取消和损坏快照清理、任务状态与账号隔离、三协议真实图片字节、能力防伪和输出上限、原始请求号、usage、Pi 压缩/审批/生命周期、输出纠偏。Platform PG 验证非零预留、明确拒绝释放和超时继续保留原预留。

## 验证期间的纠正

最初超时测试得到余额 1000，曾被误判为未知结果退款。随后读取恢复前状态，发现原 FrozenQuota 和 ReservedQuota 均为 0。夹具只更新旧 ModelPrice 缓存，原生计价读取持久化 options，导致没有发生预留。夹具补写持久化 ModelPrice 后，预留 10、余额 990，超时后原预留仍保留，最终 PG 竞态检查通过。

首次 Platform 前端测试用 Bun 执行 Vitest，在导入 Zod 时失败，未执行测试。使用相同锁文件与依赖，在 Node 下运行 Vitest 后 77 项通过。另有前轮缺少上游容量错误 helper、Linux 传输混入 AppleDouble 文件和单挂 web 目录漏掉跨目录依赖，均修正后重跑。新增纠偏夹具曾缺产品目录路径、worker 取消映射和画布工具作用域，且直接传入了不能进入正常 JSON 桥的截断正文，最终按实际模型桥返回的参数对象及原画布作用域验证。原始日志保存在本轮私有缓存中，证据目录只保留安全摘要。

## 联合构建配置

`checks.json` 的命令使用 `VALIDATION_ROOT`、`GO_MODULE_CACHE` 和 `GO_OBJECT_CACHE` 表示本批独立目录及缓存挂载。验证目录内包含 `studio/`、`platform/` 以及如下临时 `go.work`。运行 Platform 资金专项时，两项 AUDIT_DSN 均连接专用隔离 PostgreSQL 数据库。

```go
go 1.25.1

use (
    ./studio/backend
    ./platform/platformclient
)

replace github.com/Oceanway-AI/oceanway-platform/platformclient v1.0.0-rc.2 => ./platform/platformclient
```

Pi 用例从 Studio 根目录运行 `node --test backend/agent-runtime/pi/*.test.mjs`，依赖按原锁文件安装。独立镜像从同一根目录使用 `docker build --platform linux/amd64 -f yingce-agent/Dockerfile .` 构建。

## 发行与验收接续

1. 将 Platform 修复与 SDK 契约作为同批变更审阅，发布真实 SDK 版本。
2. Studio 固定该实际版本，移除临时 `go.work` 依赖进行常规构建及消费者回归，再发布 Platform、Studio 和独立 Agent 镜像。
3. 在测试环境用新任务验证真实供应商 Responses 两轮工具、实际图片、长会话压缩、关页恢复、任务中心显示和请求号对账。付费验收需要对应授权。
4. 旧具名请求通过原 RequestID 恢复和清理，部署后的原记录状态需要再次核对。

本批验证使用免费 HTTP/素材夹具。真实浏览器、真实供应商、生产资金与实际 CDN、完整全仓回归及 PR CI 尚未执行。Studio 数据库验证主要为 SQLite，Platform 的资金和恢复验证使用隔离 PostgreSQL。此次联合编译依赖本批 SDK 源码，正式 SDK 发行是应用独立构建与发布的前置步骤。
