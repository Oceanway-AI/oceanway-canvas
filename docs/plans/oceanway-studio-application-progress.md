# Studio 应用侧并行实施进度

更新：2026-11-08。应用分支：`codex/studio-platform-application`；基线 `b05d71d`。此文件由应用侧会话写、主协调会话读。

## 分工和首发范围

应用侧负责 `backend/`、`web/`、`.env.example`、`AGENTS.md`、`docs/design/`、Studio 计划与平台盘点；已有未提交的 API/数据库文档和索引归属此前身份批次，一并保留。部署侧负责 deploy、Docker/Compose/nginx、发布脚本、独立测试入口及 PG/Redis/Platform 版本核对。应用侧不修改这些部署文件，不推送、不部署、不操作远端配置。生产 Console 不改配置、不重启；旧线上数据不清理。

首发验收范围调整为：平台身份 + 素材上传/采用/引用/结果 + 文本/图片/视频生成 + 原生 Pi Agent 对话/审批/同链工具 + 画布/任务恢复。其余 OAC 创作源码全部保留，未接通的功能明确暂不可用，不以音频、3D、全插件覆盖阻塞首发；完整范围在后续批次推进。不恢复旧 Studio、独立 Creative 或 Runtime。

## 批次状态

| 批次 | 状态 | 行为与证据 |
| --- | --- | --- |
| A 身份/画布 | 已提交 `4e64c70` | 既有 dirty 全部保留；SDK/HTTP/SQLite、前端专项、构建、浏览器夹具通过，详见 first-milestone |
| B 模型目录 | 读取已接通，预估/提交继续 | `/api/model-catalog` 经 SDK Models，保留完整平台价格/能力；OAC 选择器使用原模型名和精确小数展示；同一模型替代变体不合并。当前不启用生成 |
| C 素材 | 上传/采用核心实施中 | 新增 v93 上传意图与固定素材映射；HTTP 夹具验证完成响应丢失恢复、重复采用、跨账号拒绝与签名访问。浏览器票据直传、账号切换隔离通过首批测试；素材库 UI/任务结果关联继续 |
| D 普通生成 | 待实施 | 复用 OAC 任务/租约/事件；保存执行授权与原请求，文本→图片→视频，不本地扣费/重发 |
| E 原生 Pi Agent | 待实施 | 模型桥/压缩/审批/工具走同一执行边界；恢复不重复收费 |
| F 首发联调 | 待环境 | 专项验证完成后交协调侧冻结与最终全量检查 |

## 测试与可验收入口

此前测试：后端 platformintegration/handler/server/repository 通过；前端身份、实时、缓存专项、typecheck、eslint、build 通过。当前正式 API 仅身份和画布，生成与 Agent 未开放。没有真实付费调用或部署证据。后续批次在此增量记录红灯测试、通过命令和提交 ID。

B1 红灯：`TestStudioCatalogUsesPlatformAndNeverLocalChannels` 原路由返回 501；`TestPublished*` 缺少实现；前端 platform-catalog 缺少价格投影。实现后 `go test ./internal/platformintegration ./internal/handler -run 'TestPublished|TestStudio' -count=1` 通过；`bun test test/platform-catalog.test.ts test/model-picker-groups.test.ts` 12 项 / 48 断言通过，typecheck 通过。目录没有本地渠道兜底、匿名不能读取产品 API、目录 Cookie 轮换逐项保留；仅价格草稿保持不可用。下一步为素材与提交前精确参数/预估验证。

B1 补充红灯：OAC 安装协议 ID 与目录映射不一致、缺省数字参数被投成空串。修复后 `go test ./internal/app ./internal/platformintegration ./internal/handler -run 'TestPublished|TestStudio' -count=1` 通过；前端同范围 13 项 / 52 断言、typecheck 通过。平台模型直接使用发布投影，不继承旧插件虚构的上下文、尺寸或参考图上限。目录读取完成，参数提交验证、报价、执行仍未开放。

C1 合同见 `oceanway-studio-materials-contract.md`。HTTP 红灯为上传接口 501；前端红灯为仍走旧 `/resources` 和账号切换后继续处理。当前改为 materials v2，SDK 原上传 ID 恢复、固定版本采用、binding 确认后才 ready；浏览器独立 fetch 不携带账号凭据，使用增量 SHA256 避免整段视频读入内存。数据库/HTTP 专项通过，前端正在补充分片、票据拒绝和既有边界回归。尚未提交。

C1 补充验证：绑定 POST 已提交但返回 503 时，重复采用通过 Binding(command_id) 恢复并确认，只有一个 Resource/一次 Bind；file/version/owner 不匹配拒绝。前端上传专项 10 项 / 58 断言通过，typecheck 通过。首发仍未开放生成或 Agent；没有实际对象存储写入或收费请求。

C2 运行入口：素材库、画布侧栏/弹窗选择器、设置中的素材空间及侧栏容量均改用 Platform；上游素材编辑器源码仍保留。选择器插入前采用用户所选 version，画布只保存 resource:ID 与本站资源 URL，不保存 CDN 签名。前端专项共 11 项 / 65 断言通过，typecheck/变更文件 ESLint/Prettier/build 通过。浏览器实际 dist 配免费夹具验证素材列表、空间、预览、503→重试、搜索空态、深色主题；侧栏旧容量接口已在检查中修正。画布插入服务已专项测试，画布完整联调在首发验收继续。

## 接口/环境要求（协调侧提供）

1. 独立 Studio HTTPS origin、保留 Host 的反代入口、测试 Platform origin、对应产品凭据文件位置（不在文档写密钥）。当前 rc.5 锁定，必须核对服务实现实际匹配。
2. 测试 Platform 的 Studio Origin、回跳、模型调用与材料 v2 权限；当前已知 W7 只登记测试站自身并只有 operation:read，不能据此假定 Studio 全链可用。
3. PG/Redis 测试连接的私密配置路径与可用模型清单（文本工具调用、图片、视频）；真实收费调用预算与测试账号会话由协调侧安排。
4. 生产 Console 只作已核对的历史参考，不把其密钥用于付费测试；应用侧仅跑隔离 HTTP 契约夹具。

## 当前接口缺口

协调侧环境核查：生产 Console 为 `cca3f516` / image `0e068d30c7ea`，旧版不含 W7 素材 v2，禁止切换/重启。W7 `https://test.oceanway.tech:18447` 为 `23d0f402` / image `2933ff284eb6`，素材/CDN 已由协调侧验证，59 个模型仅禁用草稿、连接数 0。用户最新明确新 Media Gateway 尚未就绪，临时使用旧网关，由协调侧接到 Platform；Studio 仍只使用 platformclient，不直连供应商，不恢复旧素材权威。Studio 拟使用 `https://test.oceanway.tech:18448`，W7 同主机跨端口登录回跳被拒由协调侧修复；应用侧仅使用免费夹具，不把生产域名登记视为完整功能支持。

- rc.5 不含 LoginReturn SDK 方法，身份批次已限定使用发布的匿名 HTTP 登录准备合同。
- 协调侧最新：旧网关明确为 ow-main `45.58.184.2:18420`，隔离 HTTPS 入口 `https://test.oceanway.tech:18450`，通过 W7 接入，应用侧不直连。W7 测试版 `9cf4c914` 修复同主机跨端口回跳，Studio `:18448/api/auth/login-url` 200；普通文本模型来源仍待协调侧确认。
- 后续环境更新：协调侧已通过 W7 管理 API 复用线上 5 条 standard-text 连接并启用 32 个已有定价模型；候选 `gpt-6.1-sol`（Responses）、`claude-haiku-4-5`（Messages）、`deepseek-v4-flash`（Chat/Responses）。图片/Seedance 旧网关连接已登记、具体参数发布中。应用侧仍只使用免费夹具，模型目录不等同实际生成验收。
- 音频、转写、外部工作流无对应公开 SDK 执行枚举，首发暂不可用，源码保留。
- 模型、素材、执行恢复合同正在逐项核对；未知项不伪装成支持，后续将精确记录字段/端点/影响。

## 提交记录

`4e64c70 feat(studio): 平台身份 - 接入 SDK 会话与画布持久化`：已纳入原有应用 dirty、SDK 身份/画布与文档；没有推送。后续按完整行为分批提交。专项检查由应用侧执行，最终全量检查由协调侧执行，避免共享目录并发检查。

`ceebee9 feat(studio): 模型目录 - 接入平台发布能力与精确价格`：B1 只读目录、精确价格显示和 OAC 协议/能力投影；未推送。模型可调用环境与提交链仍待后续。

`1d61a51 feat(studio): 素材接入 - 平台上传采用与原状态恢复`：C1 上传/采用/访问 API、v93 迁移、浏览器票据直传与恢复；未推送。C2 将替换素材列表/画布选择/存储设置的运行入口，保留 OAC 上游源码；只在点击插入时固定版本采用，不把签名 URL 作为画布身份。
