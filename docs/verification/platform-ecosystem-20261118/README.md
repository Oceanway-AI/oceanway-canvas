# WP-STUDIO-020 本机实施与验证

后续自有 v2.0.2 的 CI、生产安装、真实 Platform 浏览器验收与剩余工作，见 `../platform-ecosystem-production-20261118/README.md`。本目录保留首批本机验证的版本与环境范围。

本批执行 FS-STUDIO-001 v1.2，工作包保持 doing。最终验收代码位于本地提交 `75f556e82c6df4e3ee4a60a96dc6c7a876db6707`，树为 `7d1de2940777f74bf4a3eae8947ab9d58f079240`，基线为 `63f35e9be548ed7ca4a2839147d636992929cca5`。首批实现与浏览器验收提交为 `e8df8f5562c07d72df40c5262fc767f3aa4fcc9a`。后续 f2844bb 删除未注册的存储后台查询代码并修正测试数据库隔离，75f556e 调整模型选择器测试的初始化。SDK 固定为 `platformclient v1.0.0-rc.5`。

## 实现范围

- 请求内主体直接使用 Platform 数字用户 ID，登录不创建本地账号或身份映射。内容动作按当前产品角色校验，Skill 作者使用业务快照。
- 写请求同时核对身份版本和标签页主体。浏览器采用新的用户缓存命名空间。
- 开发、测试、运行和部署配置统一要求 PostgreSQL + Redis。SQLite 驱动入口、备份／VACUUM、旧迁移链及跨库转换命令退出。
- 新库仅建立 Studio v98。清理命令默认盘点，显式执行先释放 Platform 引用和委托，再事务性重建。释放日志支持失败重入，普通启动拒绝旧表或未完成的清理日志。
- 旧认证、短信、支付服务与 worker 退出生产装配。旧钱包、充值、订阅、返佣及后台页面退出挂载。财务统计、模型定价管理、兑换码模型和部分渠道管理源码已删除。
- 任务列表、详情和 Pi 诊断统一读取 Platform 资金回执，保留精确积分字符串，区分零值、预留、释放和未知。浏览器删除未注册的旧供应商查询按钮。
- 安装器、更新器及 Compose 改用 OceanWay 来源，要求显式 Platform 配置，保留根路径和 `/studio/` 子路径。
- 删除未注册的旧存储管理路由、列表、跨用户预览、统计和专属测试。素材删除引用检查仍有共享调用，保留其原专项约束。

## 环境与验证

本机使用 Go 1.26.3、Bun 1.3.14、PostgreSQL 18.4 和 Redis 8.6.3。数据库测试使用独立 PostgreSQL schema，清理 DDL 通过 advisory lock 串行释放。浏览器使用实际 Studio 前后端、模拟 Platform HTTP 服务及免费图片／视频夹具。

完整检查结果记录在 `results.json` 与 `quality-summary.txt`。首轮默认检查为 11 项中 1 项失败，失败用例 `TestReferenceTransportResolutionDoesNotTrustTaskConfig` 的夹具缺少启用模型记录。补齐夹具后，4 项参考媒体传输专项及包含 393 个顶层用例的原分片通过。

第二轮针对干净提交 e8df8f5，11 项中 1 项失败，用时 606.9 秒。`TestAdminSystemPerformanceReportsPostgres` 的数据库容量查询在并行 schema 测试期间耗尽 3 秒健康检查时限，夹具还缺少 `system_settings`。数据库级检查改用独立数据库并补齐配置表后，相关用例连续五轮通过，用时 25.053 秒。服务的健康检查时限与降级判断保持原逻辑。测试账号需具备 CREATEDB 权限。

删除旧存储查询入口后，编译发现两处旧查询断言仍在 Live2D／TOS 测试中，移除这两个退役断言。保留的删除保护、素材引用、Live2D 和存储协议专项通过。

第三轮针对干净提交 f2844bb，11 项中 1 项失败，用时 532.5 秒。后端全部分片通过。前端模型选择器的首次组件加载位于 5 秒用例时限内，该用例超时后又干扰后续 React act。75f556e 将组件加载移到隔离子进程的测试初始化阶段，保持产品逻辑和全部断言，29 项／109 断言的子进程专项通过。最终默认检查针对干净提交 75f556e，结果随原始摘要归档。

最终默认检查为「共 11 项，失败 0 项，用时 76.8s（并行 3）」。前端 3232 项／77234 断言通过。Go 源码与上一轮完全相同，后端测试复用上一轮全通过的 Go 缓存。安装配置静态专项 9 项通过。

任务资金回执、直接身份、schema 和清理的 race 专项通过，覆盖 database、handler、app。`go vet ./...`、`go build ./...` 和前端生产构建通过。前端构建仍报告已有的大 chunk 提示。

`retirement-inventory.json` 记录最终源码的运行模型、显式清理名单、测试模型差异、主要剩余调用方和依赖闭包。运行包及项目测试包中的 SQLite 包均为零。`go.mod` 的 `mattn/go-sqlite3` 间接条目由 GORM 自身依赖测试引入，项目运行和项目测试不导入该包。错误脱敏词表及历史记录中的 SQLite 字样保留。

## 浏览器结果

`browser-results.json` 保存接口与页面结果。根路径画布 `cs8Nug3doqwz1kVdNIFex` 和子路径画布 `1uuhYD9FFrTqrD7QrIFsz` 均在采用两项既有结果后重开，保持 2 个节点、2 个 Asset、2 个 Resource、2 个引用／绑定及 0 个物理删除任务。

模拟 Cookie 从 alice 切换到 bob 后，旧标签页写入得到 409／`studio_actor_changed`，读取 alice 画布得到 404，bob 的任务列表为空。切回 alice 后原画布恢复为 2 个节点。

任务列表和详情验证高精度字符串 `9007199254740993.123456789012345678`、显式零积分、预留与未知。资金测试行直接写入本次隔离夹具库，未向模型提交请求。首次截图发现翻译键和长金额裁切，修复后复看 `task-funds.png`，金额完整换行。390×844 深色偏好页重开保留默认图片数 2，文档宽度未超出视口，截图为 `preferences-mobile.png`。

重新启动夹具会清空该夹具 schema。旧浏览器缓存对应的画布因此返回 404，最终业务验证使用新浏览器会话。子路径首次取证发生在画布转入尚未完成时，随后等待节点落库并再次刷新，最终保存状态为 2 个节点。

## 开放项

- E3 仍有 `finance.go`、`task_billing.go`、旧任务生命周期／worker 分支、渠道分组定价及 OSS 配置耦合。`testdb.DomainModels` 为保留创作源码测试仍建立部分旧域表。生产构造和 Studio schema 已隔离这些域，源码物理退出尚未完成。
- E4 尚未对任何部署实例执行清理。本批只清理自身隔离测试 schema，正式实例清理需固定实例、停止写入并确认原引用／委托可释放。
- E5 的真实 Platform 账户、Console 付款返回、真实云素材、付费生成和跨进程生产场景尚未复验。本批结果适用于免费模拟 Platform 夹具。
- E6 尚未完成具名 Release 的容器安装、升级及回退。核对时 OceanWay Canvas Release 列表为空，本机 Docker daemon 不可用。
- 本机缺少 golangci-lint v2.5.0。Agent 架构测试已包含在后端测试中。默认检查按改动范围跳过预演台 E2E。

验收后已关闭本批浏览器会话、两个前后端夹具，以及独立 PostgreSQL／Redis 进程。临时数据库目录保留用于追溯早期测试。

## 复跑

先提供 `CANVAS_TEST_POSTGRES_DSN` 和 `REDIS_URL`，保持 PostgreSQL 与 Redis 可访问。

```sh
bun scripts/verify-before-push.mjs --jobs 3
cd backend
go vet ./...
go build ./...
go test -race ./internal/database ./internal/handler ./internal/app \
  -run 'Test(StudioTaskFunds|StudioIdentity|StudioSchema|StudioReset|PostgresOnly|StudioCurrent|StudioRetired|StudioAgent.*Preflight|CloudAgentTaskDiagnostic)' -count=1
cd ../web
bun run build
```

浏览器使用 `studio_asset_browser` build tag，`STUDIO_ASSET_FIXTURE_HISTORY=1`，分别设置 `STUDIO_ASSET_FIXTURE_ADDR` 和可选 `STUDIO_ASSET_FIXTURE_BASE_PATH=/studio`，通过 `go test -timeout 0` 启动 `TestStudioAssetBrowserFixture`。模拟会话 alice／bob 仅用于这套免费夹具。Vite 的后端地址与 base path 需匹配。
