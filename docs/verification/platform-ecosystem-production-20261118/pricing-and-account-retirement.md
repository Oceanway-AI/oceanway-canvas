# 价格结构与本地账号查询退出

本批从 `fae3a07` 接续 WP-STUDIO-020。验证环境为本机 PostgreSQL 独立 schema、模拟 Platform 与原生 Node Pi，实际生产版本在发布记录中另行登记。

## 源码与数据

`ChannelModelPriceTier` 拆为 `ChannelModelVariant`，保留生成参数与上游 SKU 映射，删除单次、时长、Token、缓存、参考视频价格、价格版本、分时规则和本地算价函数。模型选择继续检查能力和参考输入，正式 Studio 目录、资金预览、预算确认与执行仍使用 Platform。

浏览器配置中的模型元数据改为 `modelDetails`。模型选择器、设置面板、Creation、画布及 Agent 消费同一份能力信息，删除旧价格档准入与 `/models/*/quote`、`/model-catalog/quote` 调用。工作台费用说明复用已有 `/tasks/estimate` 平台预览，多任务时标明首个任务的估价。旧电商报价展示和本地价格门禁退出，电商、短剧、方舟及工作台的开放状态继续由现行 Studio 路由决定。

独立积分开关、支付插件能力声明、支付弹窗、签到和推广入口的闲置翻译随调用方退出。正式账户中的 Platform 充值、订阅和账务导航继续走中央合同。

本地 User／UserAvatar 仓储退出。技能治理搜索使用作者快照及主体 ID，日志搜索使用记录自身字段。保留的方舟创作源码使用稳定主体 ID 展示和筛选，逐个授权校验规范十进制 ID，其生产入口仍未开放。`testdb.DomainModels` 删除本地 User、UserAvatar 和商业实体，创作测试使用请求内主体。

v100 将 `creation_submissions.price_signature` 保留原值并重命名为 `configuration_signature`。迁移继续使用 PostgreSQL 事务和 advisory lock，测试核对待审批记录、画布、Task 绑定和历史执行数据。v98 制品需配套备份恢复，不能直接运行于 v100 数据库。

## 本批验证

| 范围 | 结果 |
| --- | --- |
| 全后端编译 | `go test ./... -run '^$'` 通过 |
| 规格、模型路由、Creation、Pi 审批和 schema 专项 | app 33.709 秒、repository 0.738 秒、database 6.040 秒通过 |
| v98 升级与待审批配置保留 | database 5.359 秒通过 |
| Platform 已发布合成样例 | 1.576 秒通过，Studio 投影仅删除旧 priceTiers／referenceVideoBilling 字段 |
| 前端类型 | 通过 |
| 首轮前端全量 | 3193 通过，5 失败，原因包括旧算价引用、旧价格图标断言和电商报价文案断言 |
| 上述失败定向重验 | 30 测试、159 断言通过，覆盖模型选择、原生价格展示、Creation 参数和电商页面生命周期 |
| 账号退出后端专项首轮 | repository、handler、skills 通过，app 三项旧断言仍要求短剧本地冻结积分、按本地价格拒绝规格、按本地估价拒绝 Agent |
| 上述后端失败定向重验 | app 2.914 秒、repository 0.637 秒通过，覆盖无本地账务字段、基础模型与规格匹配、无效预算拒绝和零账号表作者快照搜索 |

最终提交前质量检查、真实 Platform 联验及自有 Release 发布继续按 WP 执行。完整通过结论以最终树的后续记录为准。
