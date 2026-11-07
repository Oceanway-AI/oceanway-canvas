# Platform 接入盘点与实施记录

## 已确定环境

- 开发 Platform：`https://test.oceanway.tech:18447`。
- 生产 Platform：`https://console.oceanway.tech`。
- Studio 域名：`studio.oceanway.tech`；入站 Host 为 `studio.oceanway.tech`，允许的 Origin 为 `https://studio.oceanway.tech`。切换环境还必须核对对应产品登记和 Cookie 范围，不能只改服务地址，不以 Platform URL 冒充 Studio Host。
- 已按用户授权从 `oceanway` 只读取回 Studio 产品配置，凭据保存在 Git 忽略的 `.local/private/studio-platform.env`（目录 0700、文件 0600）；不进入源码或浏览器。反向代理须保留实际 Studio Host，不通过 forwarded-host 改写身份。
- 生产配置已登记 Studio Origin/回跳及 `model:invoke` 产品权限；匿名 login-return 检查通过。测试配置目前仅登记测试站 Origin/回跳，Studio 产品权限为 `operation:read`，尚不能用于所计划的 Studio 跨域闭环。没有修改服务器配置或部署服务。
- SDK 固定 `v0.1.0-rc.5`，commit `0bd81e95d0892aef60a849950b82bee41fb69b0a`；go.sum 已记录模块与 go.mod 校验值。已下载独立模块，不依赖 Platform 根项目。
- 本地通过夹具验证，不代表上述服务已部署相同契约。

## 调用地图（首轮，持续补齐）

| 入口/模块 | 当前路径 | 改造与依赖 |
| --- | --- | --- |
| 当前用户 | handler/auth.go → app/studio_platform.go → platformintegration | 已按实际请求 Host/Origin/Cookie 查询 Platform，映射产品用户引用 |
| 画布实时连接 | handler/canvas_realtime.go | 已替换周期复核；失效/换号/轮换 Cookie 时关闭连接，由前端 HTTP 刷新后决定重连 |
| 登录/注册/退出 | handler/studio_api.go、web 的 Platform 登录组件 | 已接入登录准备/回跳/会话/退出；生产注册器不挂载本地密码/验证码/OAuth 写入口 |
| 普通生成 | task_creation.go → task_execution.go → provider.go | 服务端模型、授权、持久请求身份、资金及结果关联整体切换 |
| 原生传输 | provider_http_client.go、provider_protocol_request.go | 现有供应商 HTTP 通道不能成为 SDK 的旁路 |
| 第三方工作流 | workflow_provider.go、workflow_provider_outputs.go | 独立上传/任务调用；rc.5 未发现对应公开 SDK 方法，需平台合同 |
| 专用供应商能力 | provider_ark_private_assets.go、provider_jimeng.go | 不能按普通图片端点假定支持；单列资源/执行契约 |
| Agent 模型桥 | cloud_agent_pi_model.go | 对话/压缩和自动重试均需平台执行身份；保留 Pi 编排 |
| 本地资金 | task_billing.go、payment*.go、handler/finance_routes_test.go | 每条执行链同步撤出扣款退款，最后删除失效代码 |
| 持久存储 | storage*.go、task_media_storage.go、handler/storage_grants.go | 平台素材引用替换原件存储，临时处理不混入原件权威 |
| 字幕/渲染 | handler/routes.go 的 timeline/transcriptions、timeline/renders | 转写需音频合同；纯渲染保持产品计算，输入输出使用统一素材 |
| 新库迁移 | internal/database/migrations.go | 旧序列包含支付、渠道、用户平台表；不能全跑后宣告新库已去平台化 |

## rc.5 能力初核

| 能力 | SDK 源码证据 | 状态 |
| --- | --- | --- |
| 身份、退出、SSO | identity.go | 身份/退出已接入；rc.5 无 LoginReturn 方法，匿名登录准备使用发布的固定 HTTP 合同；真实登录待验收 |
| 模型目录、媒体预估 | catalog.go | 有契约；实际模型能力待服务端核对 |
| 文本、图像编辑/生成、视频、Gemini、豆包视频 | execution.go | 有原生枚举/方法；具体模型支持不由 SDK 发布保证 |
| 工作区/团队 | workspaces.go | 有上下文合同；真实部署待核对 |
| 素材 | materials.go | 有方法；资源与 Studio ID 映射待实施 |
| 音频/转写 | execution.go 未提供对应媒体路由枚举 | 平台缺口待核对，不擅自扩展任意路由 |
| RunningHub 等外部工作流 | 未发现专用 SDK 调用面 | 阻塞对应功能切换，其他基础链路继续 |
| 非模型处理收费 | 未确立合同 | 不创建本地积分替代 |

## 首批数据关联

| 关联 | 已确定边界 | 实施状态 |
| --- | --- | --- |
| 平台用户 → 产品主体 | 以固定 Platform origin + 数字用户 ID 生成稳定 UUID；唯一 identity subject 绑定；不以可变用户名或 email 匹配 | 已实现个人主体；工作区/团队上下文待 P2 补齐 |
| 平台模型 → 创作模型视图 | 保留平台模型标识、能力、参数及报价版本，不以本地渠道作为执行权威 | P3 待实施 |
| 业务任务/Agent 步骤 → 执行授权与原请求 | 派发前持久绑定；结果未知时查询原请求，不新发收费调用 | P5/P6 待实现并验证非原子边界 |
| Studio resource → 平台 asset/version | 稳定 ID 关联；签名地址只作临时访问，不作长期身份 | P4 待实施 |

## 已实施：SDK 与身份首批

`backend/internal/platformintegration` 提供严格配置初始化与真实入站 SessionContext 提取。产品标识固定 studio；缺少 origin、product key、允许 Host 或 Origin 时拒绝初始化。不读取 forwarded-host、不生成缺失 Origin、不混用会话与执行凭据。

配置变量：`STUDIO_PLATFORM_ORIGIN`、`STUDIO_PLATFORM_PRODUCT_KEY`、`STUDIO_PUBLIC_ORIGIN`、`STUDIO_PLATFORM_SESSION_HOSTS`、`STUDIO_PLATFORM_ALLOWED_ORIGINS`，列表以逗号分隔。公开 Origin 必须是精确 HTTPS origin，并匹配允许列表。产品密钥只在后端环境注入，不进入 Vite 配置。

已验证：`go test ./internal/platformintegration`，身份凭据隔离、恶意 forwarded-host、缺失 Origin 的写请求拒绝、多 Set-Cookie 轮换、错误不泄漏密钥。

边界已注入实际启动组合根，缺失配置在创建数据目录前拒绝启动；阶段注册器只开放身份、画布及分享，未启动上游生成/资金 Worker。后端专项验证画布持久化、重复身份、跨账号拒绝、角色撤销、实时复核及无本地账号会话/钱包/账单写入。数据库迁移和部分构造器仍包含待 P8 清除的旧平台结构，不把路由关闭视为全部代码清理完成。完整证据见[首批实施记录](oceanway-studio-first-milestone.md)。P0 首轮地图未覆盖全部插件，仍进行中。
