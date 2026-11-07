# Studio 首批实施与验收记录

日期：2026-11-08。对应[实施计划](oceanway-studio-implementation.md) A1–A7。状态：SDK、个人身份和空白画布 API 已实施并通过隔离测试；真实账号端到端验收尚未完成。当前版本不作为全功能 Studio 发布。

## 已实施范围

- `cmd/server` 必须通过 `platformintegration.NewGatewayFromEnv` 和 `service.NewStudio` 启动；配置缺失先拒绝启动，不创建数据目录，不退回本地认证。
- `RegisterStudioAPI` 挂载身份、只读账号资料、外观/功能开关、画布数据/编辑租约/实时连接/公开分享。旧认证、充值/支付、生成、供应商中转及其他待接入领域不挂载，未知路由返回 501。旧 OpenAPI 尚未更新，当前阶段不暴露它以免宣告不存在的接口。
- Platform Identity/Logout 使用 SDK rc.5；会话请求只使用真实入站 Host、Origin、Cookie，不携带产品密钥。模型 Worker 凭据隔离通过 SDK 恢复接口夹具验证，尚未启用实际生成 Worker。
- SDK rc.5 未提供 LoginReturn 方法；登录准备只调用固定 Platform origin 下已发布的匿名 `GET /api/platform/v1/auth/login-return`。回跳固定为 `STUDIO_PUBLIC_ORIGIN/platform/login/complete`，只接受服务端确认的同源 `/login?return=...`，不接收用户指定的外部回跳。
- 产品用户按固定 Platform origin + 数字用户 ID 关联；每次请求以当前身份和 `studio:admin` 产品角色为准，不把平台数字管理员级别或数据库缓存角色视作 Studio 管理授权。画布归属仍由产品校验。
- HTTP 逐项透传 `Set-Cookie`；WebSocket 周期复核遇到会话失效、主体变化或 Cookie 轮换时以 1008 / `session expired` 关闭。浏览器先用 HTTP 更新会话再决定重连；退出或换号停止旧连接并应用用户 scope 隔离。
- 前端认证页面只提供 OceanWay 登录。账号资料由平台维护，Studio 提供只读资料和控制台链接；加载中或失败时也不出现旧密码、验证码或头像修改表单。跳转目标只保留站内路径；会话请求缓存按代次失效，旧请求不能覆盖新账号状态。
- 不创建本地登录会话、钱包、账单或注册奖励。不启动上游生成/退款/支付后台循环。仍存在的旧代码、表结构和构造器初始化在 P8 清除，当前没有宣告空库结构已完成精简。

## 配置与线上只读核对

用户明确指定服务器 `oceanway`，本次只读取配置和执行匿名登录准备检查，没有修改线上配置、创建用户、部署新 Studio 或发起收费生成。

| 项目 | 核对结果 | 限制 |
| --- | --- | --- |
| 生产 Platform | `https://console.oceanway.tech`；Studio Origin/回跳已登记；存在具有 `model:invoke` 的 Studio 产品凭据 | 配置存在不证明完整执行、素材及团队契约已验收 |
| 测试 Platform | `https://test.oceanway.tech:18447`；当前只登记测试站自身 Origin/回跳；Studio 凭据仅有 `operation:read` | 不能只切换 base URL 就执行 Studio 登录/模型验收，须由负责该服务的一侧补齐登记 |
| 生产登录准备 | HTTP 200、`success=true`，原样确认 `https://studio.oceanway.tech/platform/login/complete` 并返回 `/login?return=...` | 仅验证匿名回跳批准；没有使用用户会话，也没有完成浏览器真实登录 |
| 本机产品配置 | Git 忽略的 `.local/private/studio-platform.env`；目录 0700、文件 0600 | 仅限服务端环境，不进入前端、日志或提交 |

启动需注入 `STUDIO_PLATFORM_ORIGIN`、`STUDIO_PLATFORM_PRODUCT_KEY`、`STUDIO_PUBLIC_ORIGIN`、`STUDIO_PLATFORM_SESSION_HOSTS`、`STUDIO_PLATFORM_ALLOWED_ORIGINS`，同时使 `CANVAS_CORS_ORIGINS` 与浏览器实际 Origin 一致。反向代理必须保留真实 Studio Host，不能用 forwarded headers 伪造身份。独立测试库不得指向归档的旧 Studio 数据。

本阶段不修改本机或远端 Platform 实现。现有 Compose/安装器尚未完成环境变量透传和 OceanWay 自有制品切换，不能直接用旧安装流程发布本阶段代码。

## 已执行验证

| 验证 | 结果与覆盖 |
| --- | --- |
| `backend/`: `go test ./internal/platformintegration ./internal/handler ./cmd/server ./internal/repository -count=1` | 通过。HTTP Platform 夹具、SQLite 完整迁移；身份重复映射、画布持久化重读、跨账号拒绝、Cookie 透传、权限撤销、旧入口不可达、启动拒绝及上游错误脱敏 |
| `backend/`: `go test ./internal/platformintegration ./internal/handler ./cmd/server -run 'TestStudio' -count=1` | 收尾修改后通过；补充分享创建/匿名读取/撤销、账号变化与 Cookie 轮换的实时复核、取消/超时原因保留 |
| `web/`: `bun test test/auth-entry.test.ts test/platform-login.test.ts test/canvas-realtime-client.test.ts` | 13 项、66 断言通过；站内跳转、刷新后重连及撤销停止 |
| `web/`: `bun test test/platform-session-cache.test.ts test/profile-session.test.ts` | 2 项、15 断言通过；旧会话请求不覆盖新缓存，用户资料应用不破坏 scope |
| `web/`: `bun run typecheck`、变更文件 ESLint、`bun run build` | 通过；实际前端产物构建成功 |
| Playwright 操作实际 `web/dist` 路由，API 使用本机隔离夹具 | 登录页面只有 OceanWay 登录入口；点击后经模拟 HTTPS 登录方回跳到 `/platform/login/complete`，最终到 `/canvas` 并显示当前账号和画布空态；清空 Cookie 后回跳显示“尚未完成登录”；账号资料接口失败时显示重试、恢复后显示只读身份和账户中心链接，全程不展示本地账号修改表单 |

浏览器夹具不使用真实凭据，也不代替 Go 持久化测试。浏览器中模型目录、素材容量、插件等尚未实现的接口返回 501，因此可见“容量暂不可用”等阶段提示；不能将这次验证写成全功能、零控制台错误或完整画布编辑器验收。本次验证后已关闭专用浏览器会话及临时静态服务。

截图证据保留在 Git 忽略的 `output/playwright/studio-platform-canvas.png`；临时夹具位于 `.local/verification/`，均不进入发布产物。

## 待验收与后续顺序

- [ ] A1/P0：补齐插件、后台指定模型、Agent 隐含调用及特殊工作流矩阵，不以首轮目录盘点代替完整调用审计。
- [ ] A7/P2：在登记完备的 HTTPS Studio 测试入口完成真实登录、刷新、退出、两账号切换、画布保存/重新打开、分享和真实 WebSocket Cookie 轮换。
- [ ] P2：个人/团队工作区合同与持久映射；当前仅支持个人用户的产品引用，未声称团队隔离已完成。
- [ ] PostgreSQL/Redis 多实例验证；目前持久化证据来自 SQLite，不能替代生产数据库及协调器验收。
- [ ] P3/P4：平台模型目录、参数/预估与素材上传/版本/引用，这是下一开发批次；现有生成及 Agent API 在适配前保持关闭。
- [ ] P5–P9、V1：普通生成、原生 Pi Agent 全调用链、所有创作场景、旧平台代码与迁移清理、OceanWay 视觉、部署制品及全功能发布。

未提交、推送或部署；未运行推送前全量验证脚本。发布前仍须完成计划中的完整质量检查与真实流程验收。
