# Studio 应用侧并行实施进度

更新：2026-11-08。应用分支：`codex/studio-platform-application`；基线 `b05d71d`。此文件由应用侧会话写、主协调会话读。

## 分工和首发范围

应用侧负责 `backend/`、`web/`、`.env.example`、`AGENTS.md`、`docs/design/`、Studio 计划与平台盘点；已有未提交的 API/数据库文档和索引归属此前身份批次，一并保留。部署侧负责 deploy、Docker/Compose/nginx、发布脚本、独立测试入口及 PG/Redis/Platform 版本核对。应用侧不修改这些部署文件，不推送、不部署、不操作远端配置。生产 Console 不改配置、不重启；旧线上数据不清理。

首发验收范围调整为：平台身份 + 素材上传/采用/引用/结果 + 文本/图片/视频生成 + 原生 Pi Agent 对话/审批/同链工具 + 画布/任务恢复。其余 OAC 创作源码全部保留，未接通的功能明确暂不可用，不以音频、3D、全插件覆盖阻塞首发；完整范围在后续批次推进。不恢复旧 Studio、独立 Creative 或 Runtime。

## 批次状态

| 批次 | 状态 | 行为与证据 |
| --- | --- | --- |
| A 身份/画布 | 已提交 `4e64c70` | 既有 dirty 全部保留；SDK/HTTP/SQLite、前端专项、构建、浏览器夹具通过，详见 first-milestone |
| B 模型目录 | B1 已提交 | `/api/model-catalog` 经 SDK Models，保留完整平台价格/能力；OAC 选择器使用原模型名和精确小数展示；同一模型替代变体不合并。媒体预估继续 |
| C 素材 | C1–C4 已提交，最新 `8e542a8` | v93 上传意图与固定素材映射、v95 会话素材授权、工作台/画布选择及生成结果消费已接通；项目/任务引用生命周期继续 |
| D 普通生成 | D1/D2 已提交 | 三种文本协议、精确媒体预估/上限、原请求恢复、固定产物采用；参考素材与原生图片/视频请求专项通过，真实媒体验收待协调侧 |
| E 原生 Pi Agent | E1 已提交 `78c37a4`；E2 已提交 `991e462` | 根轮次共享 EXA，模型桥/压缩走持久执行；Node/Pi 画布工具与媒体审批往返通过，真实浏览器媒体验收待协调侧 |
| F 首发联调 | F1 入口门禁专项通过；真实联调待协调侧 | 专项验证完成后交协调侧冻结与最终全量检查 |

## 测试与可验收入口

此前测试：后端 platformintegration/handler/server/repository 通过；前端身份、实时、缓存专项、typecheck、eslint、build 通过。应用侧使用免费夹具，部署与真实付费验收由协调侧记录。后续批次在此增量记录红灯测试、通过命令和提交 ID。

B1 红灯：`TestStudioCatalogUsesPlatformAndNeverLocalChannels` 原路由返回 501；`TestPublished*` 缺少实现；前端 platform-catalog 缺少价格投影。实现后 `go test ./internal/platformintegration ./internal/handler -run 'TestPublished|TestStudio' -count=1` 通过；`bun test test/platform-catalog.test.ts test/model-picker-groups.test.ts` 12 项 / 48 断言通过，typecheck 通过。目录没有本地渠道兜底、匿名不能读取产品 API、目录 Cookie 轮换逐项保留；仅价格草稿保持不可用。下一步为素材与提交前精确参数/预估验证。

B1 补充红灯：OAC 安装协议 ID 与目录映射不一致、缺省数字参数被投成空串。修复后 `go test ./internal/app ./internal/platformintegration ./internal/handler -run 'TestPublished|TestStudio' -count=1` 通过；前端同范围 13 项 / 52 断言、typecheck 通过。平台模型直接使用发布投影，不继承旧插件虚构的上下文、尺寸或参考图上限。目录读取完成，参数提交验证、报价、执行仍未开放。

C1 合同见 `oceanway-studio-materials-contract.md`。HTTP 红灯为上传接口 501；前端红灯为仍走旧 `/resources` 和账号切换后继续处理。当前改为 materials v2，SDK 原上传 ID 恢复、固定版本采用、binding 确认后才 ready；浏览器独立 fetch 不携带账号凭据，使用增量 SHA256 避免整段视频读入内存。数据库/HTTP 专项通过，前端正在补充分片、票据拒绝和既有边界回归。尚未提交。

C1 补充验证：绑定 POST 已提交但返回 503 时，重复采用通过 Binding(command_id) 恢复并确认，只有一个 Resource/一次 Bind；file/version/owner 不匹配拒绝。前端上传专项 10 项 / 58 断言通过，typecheck 通过。首发仍未开放生成或 Agent；没有实际对象存储写入或收费请求。

C2 运行入口：素材库、画布侧栏/弹窗选择器、设置中的素材空间及侧栏容量均改用 Platform；上游素材编辑器源码仍保留。选择器插入前采用用户所选 version，画布只保存 resource:ID 与本站资源 URL，不保存 CDN 签名。前端专项共 11 项 / 65 断言通过，typecheck/变更文件 ESLint/Prettier/build 通过。浏览器实际 dist 配免费夹具验证素材列表、空间、预览、503→重试、搜索空态、深色主题；侧栏旧容量接口已在检查中修正。画布插入服务已专项测试，画布完整联调在首发验收继续。

## 接口/环境要求（协调侧提供）

1. 独立 Studio HTTPS origin、保留 Host 的反代入口、测试 Platform origin、对应产品凭据文件位置（不在文档写密钥）。当前 rc.5 锁定，必须核对服务实现实际匹配。
2. W7 已登记 Studio 测试入口、修复同主机跨端口回跳；协调侧确认登录与画布保存/恢复，materials v2 已单独验证。仍按每个实际调用验证权限，不以登录或模型列表替代全链验收。
3. PG/Redis 测试连接的私密配置路径与可用模型清单（文本工具调用、图片、视频）；真实收费调用预算与测试账号会话由协调侧安排。
4. 生产 Console 只作已核对的历史参考，不把其密钥用于付费测试；应用侧仅跑隔离 HTTP 契约夹具。

## 当前接口缺口

协调侧环境核查：生产 Console 为 `cca3f516` / image `0e068d30c7ea`，旧版不含 W7 素材 v2，禁止切换/重启。W7 `https://test.oceanway.tech:18447` 已更新至至少 `9cf4c914`，素材/CDN 已由协调侧验证。用户明确新 Media Gateway 尚未就绪，临时使用旧网关，由协调侧接到 Platform；Studio 仍只使用 platformclient，不直连供应商。Studio 测试入口 `https://test.oceanway.tech:18448` 登录/画布已验证；应用侧仅使用免费夹具。

- rc.5 不含 LoginReturn SDK 方法，身份批次已限定使用发布的匿名 HTTP 登录准备合同。
- 协调侧最新：旧网关明确为 ow-main `45.58.184.2:18420`，隔离 HTTPS 入口 `https://test.oceanway.tech:18450`，通过 W7 接入，应用侧不直连。W7 测试版 `9cf4c914` 修复同主机跨端口回跳，Studio `:18448/api/auth/login-url` 200。
- 后续环境更新：协调侧已通过 W7 管理 API 复用线上 5 条 standard-text 连接并启用 32 个已有定价模型；候选 `gpt-6.1-sol`（Responses）、`claude-haiku-4-5`（Messages）、`deepseek-v4-flash`（Chat/Responses）。图片/Seedance 旧网关连接已登记、具体参数发布中。应用侧仍只使用免费夹具，模型目录不等同实际生成验收。
- C2 已合并到独立发布分支，协调侧准备部署目录/素材批次；Messages 普通文本 `STUDIO_TEXT_OK`、指定 studio_ping 工具 `STUDIO_TOOL_OK` 已经 rc.5 真实冒烟，两个原请求均 RecoverText ready 且内容一致，未重发。这支持 Pi 模型候选，不等于 Pi 浏览器验收。W7 图片 new-api 类型/路径缺口已由协调侧先红后绿及 race 验证，准备仅更新测试环境；视频报价尚未发布，不重试旧失败。
- 音频、转写、外部工作流无对应公开 SDK 执行枚举，首发暂不可用，源码保留。
- 模型、素材、执行恢复合同正在逐项核对；未知项不伪装成支持，后续将精确记录字段/端点/影响。

## 提交记录

`4e64c70 feat(studio): 平台身份 - 接入 SDK 会话与画布持久化`：已纳入原有应用 dirty、SDK 身份/画布与文档；没有推送。后续按完整行为分批提交。专项检查由应用侧执行，最终全量检查由协调侧执行，避免共享目录并发检查。

`ceebee9 feat(studio): 模型目录 - 接入平台发布能力与精确价格`：B1 只读目录、精确价格显示和 OAC 协议/能力投影；未推送。模型可调用环境与提交链仍待后续。

`1d61a51 feat(studio): 素材接入 - 平台上传采用与原状态恢复`：C1 上传/采用/访问 API、v93 迁移、浏览器票据直传与恢复；未推送。C2 将替换素材列表/画布选择/存储设置的运行入口，保留 OAC 上游源码；只在点击插入时固定版本采用，不把签名 URL 作为画布身份。

`b97f0de feat(studio): 素材工作台 - 接通平台素材库与画布选择`：C2 已提交，未推送。D/E 继续，执行合同已写入 `oceanway-studio-execution-contract.md`，先补响应丢失与原请求恢复红灯。

D1：首个红灯为 POST /tasks 的 501。已实现 schema94 加密执行记录、三协议文本提交与原请求恢复；补测授权响应丢失、原请求 not_found 连续查询、同 command 冲突、跨账号拒绝、无本地账单。原生协议字段与恢复响应 envelope 按 SDK/发布合同校正。前端创建响应丢失只 GET 原 command，账号切换不采用结果，2 项/6 断言及 typecheck 通过。图片/视频/Pi 继续实施，尚未宣布首发完成。

D1 验证：`go test ./internal/handler ./internal/database -run 'TestStudio|TestCurrentSchemaVersionMatchesMigrationPlan' -count=1` 通过；前端任务提交专项、typecheck、变更文件 ESLint/Prettier 通过。Studio 专用 worker 只启动任务调度与回放清理，不启动旧账务/供应商取消 worker。SSE 每 15 秒重新验证平台账号。

`2965b70 feat(studio): 文本生成 - 接通平台原请求与断线恢复`：D1 已提交，未推送。HTTP fixture 证明三协议工具结果与断线恢复，不是 Pi 验收。

D2 红灯为 `/tasks/preview` 501 和缺少完整规格验证。当前媒体以 JSON Schema 校验所选变体（拒绝外部 schema 加载），原生 JSON 先 PreviewMedia，再冻结相同 JSON/价格版本/精确十进制上限。前端点击生成先读取报价，以返回 wallet_points 作为本次最高额度，不自动接受上涨或换模型。Worker 只提交一次，后续经原 EXA RecoverMedia → TaskOutputs → 固定 Asset/version/file → binding active → OAC Result。HTTP 模拟提交和绑定响应丢失后恢复，只有一次模型 POST/Bind。Gemini、OpenAI video、Doubao 参数映射与资源引用归属/数量专项通过；不保存 CDN URL、不从本地钱包扣费。前端任务专项 4 项/11 断言、typecheck、lint、format、build 通过。

`b56e914 feat(studio): 媒体生成 - 固定平台报价与素材产物恢复`：D2 已提交，未推送。

E1 红灯为缺少 Studio Pi 授权入口。已接通原生控制任务、冻结目录与整轮 EXA，每一步独立稳定请求 ID；子步骤不释放共享授权。未提交步骤与派发共用锁，停止后已提交请求继续恢复；禁止截断、失败与恢复后的旧自动重发。真实 Node/Pi + 免费 HTTP fixture 完成两次模型调用、canvas_get_state 工具结果回传、会话快照持久化；一次授权，整轮结束后一次释放。初次 Node 测试缺依赖，已按原 package-lock 执行 npm ci 后通过。前端改为明确 1–64 步上限，不再宣称本地积分限额/累计消费。媒体审批未开放，下一批继续接入会话报价与明确额度。

协调侧发现素材访问待补：浏览器 GET 文件缺 Origin，而 SDK Access 为 POST；固定引用访问还需 BindingID。将在 E1 提交后独立补失败测试与修复，不通过改写通用 Session Origin 绕过。

`78c37a4 feat(studio): Pi 对话 - 接通整轮授权与原步骤恢复`：E1 已提交，未推送。

C3 素材读与画布同步修复：红灯确认 BindingID 缺失、Cookie 增加导致素材授权丢失、前端重绑为平台素材新建两份本地资产、文本节点被误要求文件身份。修复使用当前身份 SID 摘要索引加密的 material-only grant；浏览器 POST 初始化授权，无 Origin 文件 GET 使用 WorkerMaterials，未修改通用 Session 来源。数据库升至95。平台素材画布只校验同用户 ready 固定绑定，不再等待旧资产接口。后端 HTTP 采用→文件读取→跨账号拒绝、SID 隔离/撤销及画布保存/重开通过；浏览器实际播放与同步状态待协调侧部署验收。生成结果的旧本地物化、Pi 媒体审批继续审计，尚未宣称首发完成。

C3 验证：`go test ./internal/app ./internal/handler ./internal/database -run 'TestStudio|TestValidateCanvasMediaAssets|TestCurrentSchemaVersionMatchesMigrationPlan' -count=1` 通过；前端素材节点、缺失资产修复、会话缓存、素材选择和画布资源同步 19 项/54 断言、typecheck、变更文件 ESLint/Prettier 通过。没有实际生成、部署或远端配置变更。协调侧另已确认5个 GPT Image 和3个 Seedance 可发现但仍 disabled；应用按发布能力处理，不另写模型分支。

`4a5205b fix(studio): 素材访问 - 绑定会话授权并解除画布旧资产依赖`：C3 已提交，未推送，包含 schema95 迁移。

E2：红灯为缺少平台媒体报价/审批入口，继而核对原生工具权限、Pi 必填参数与草稿运行身份。现在媒体仍由 OAC generate_media 创建草稿与独立审批；浏览器真实会话读取新目录及 PreviewMedia，精确小数和参数显示在原生审批卡，批准携 quoteId，固定请求/价格/上限与整轮 EXA 同事务入队。拒绝、停止、过期、画布依赖改变、报价不匹配均不提交；503 后只查原请求。审批公开投影剥除内部加密准备态。参数在审批卡冻结，调整需拒绝后改草稿并重新申请。

E2 验证：`go test ./internal/app ./internal/handler -run 'TestStudio|TestCloudAgent(Media|Approval|Tools|ToolAllowed)' -count=1` 通过。真实 Node/Pi 免费夹具完成申请→报价→批准→媒体503→原请求失败恢复→工具回执→继续对话，模型提交一次/报价一次/整轮授权及释放各一次。成功产物固定 Resource 回写画布的 Go 专项不创建本地 Asset。前端真实 React 审批卡和普通生成提交共6项/14断言通过，typecheck、变更 ESLint/Prettier、build 通过；构建仍有既有 chunk 大小/动态导入提示。未部署，未做实际收费调用，生成结果浏览器物化路径继续修复。

`991e462 feat(studio): Pi 媒体审批 - 冻结平台报价并恢复原步骤`：E2 已提交，未推送。

C4 生成结果消费：先补6个失败场景，确认仍进入旧 Asset 物化和 Pi 节点准备守卫。Platform 任务现在只校验固定 Resource，直接回填多图、视频和消息，保留原生 attach effect 幂等；不新建 OAC Asset，不调用旧资产同步/外部素材源。每次恢复绕过资源元数据缓存复核归属、状态和类型；换号/取消拒绝。持久结果使用本站文件地址，移除旧 Asset ID 与过期签名 URL。原生 Pi 回填节点也可以直接准备画布保存。

C4 验证：平台生成/素材、旧结果恢复、画布回填及跨页存储一致性共29项/201断言通过；新增测试包含真实画布存储写入→清空内存→重读全部图片。typecheck/变更 ESLint/Prettier/build 通过。没有实际浏览器、付费生成或部署；上游素材远端撤销仍由真实素材访问授权确认，本地校验不代替该权限检查。

C4 补充：`generation-task-durability.node.test.ts` 3项通过。额外执行 `provider-neutral-generation-effects.test.ts` 时第1项通过，第2/3项 Bun Worker 在5秒超时，单独第2项仍超时并无法自行退出，已终止。该 effect store/Worker 用例未在本批修改；原因尚未定位，不能列为已通过，协调侧最终全量验证需复核。日志 `.local/verification/studio-platform-results-effects.log` 与 `studio-platform-effects-lock.log`。

`8e542a8 fix(studio): 生成结果 - 直接回填平台素材并恢复画布`：C4 已提交，未推送。

F1 首发入口：合同见 `oceanway-studio-release-surface.md`。红灯确认旧功能开关仍宣传短剧/插件/转写等能力，分享请求仍进入旧交付链。现强制关闭未接通标志，真实路由边界阻止工作台/后台/公开分享子页面挂载；设置只开放资料、偏好、模型、素材空间，不调用供应商素材库预查询。启动不再迁移旧插件启用状态或采纳渠道 URL，平台会话禁止旧插件 fallback。分享路由整体撤出 Studio，原 OAC 创作源码保留。身份/画布/任务路径继续回归；完整 P8 清扫与视觉、素材引用生命周期和真实服务验收仍未完成。

F1 验证：`go test ./internal/handler -run 'TestStudio' -count=1` 通过；路由/插件门禁与平台生成、会话缓存、目录、素材选择共14项/150断言通过。typecheck、变更文件 ESLint/Prettier、build、diff whitespace 检查通过。未推送/部署/修改远端配置，未调用收费生成；构建仍有既有 chunk 大小/动态导入提示。

E1 验证：`go test ./internal/app ./internal/repository -run 'TestStudio|TestCloudAgent(Request|RuntimeCompletes|Tools|ToolAllowed)'`、`go test ./internal/handler -run TestStudio` 通过。后补同轮幂等、变更意图冲突及跨账号读取专项通过。仓库专项验证过期租约/被接管租约无法派发，重复派发只允许一次，停止未提交与保留已提交两种路径。前端 typecheck、变更文件 ESLint/Prettier 通过。尚未进行浏览器 Pi 验收，不宣称媒体审批可用。

F2 首页能力缺省修复：真实 CreatePage 与 CreationComposer 曾无条件读取同一模型的 image.references、video.ratios；Platform 目录只发布模型实际具备的能力，导致文本/视频模式 references 崩溃，图片模式也可能在视频参数处崩溃。现在按当前模式及有效目录选择模型，仅在对应能力存在时恢复参数、显示设置和提交；空目录或失效偏好显示不可用提示，禁用生成，不注入虚构能力。

F2 验证：同一真实 React 页面测试在修复前 HEAD 重现 references/ratios 崩溃（6项失败），修复后6项/60断言通过，覆盖文本/图片/视频单能力、空目录、残留模型偏好、输入提示词与跨模式切换。typecheck、变更 ESLint/Prettier 通过；证据位于 `.local/verification/studio-create-capabilities-{baseline,fixed}.log`。未部署，待协调侧合入后验收 `:18448` 实际首页。协调侧正独立处理 W7 素材授权503和 PlatformUI连接类型验证，不在本分支修改服务配置。

`0fdd424 fix(studio): 创作首页 - 按平台已发布能力渲染模式`：F2 已提交，未推送。

F3 回归门禁修复与旧会话审计：

- Worker 超时：对比上游基线 `b05d71d`，测试及租约实现没有差异；本机 Bun 1.3.14 在 Worker RPC 使用 Promise matcher 时卡住回包，独立 RPC 正常。改为先 await RPC 再断言值/错误，保留跨页竞争、续租、完成回放与禁用降级断言，并补无锁时零写入断言。3项/12断言通过，无生产租约代码变化。
- 登录回归：删除检查旧 AuthPanel 源码字符串的 password-reset 测试，改用真实登录/注册/找回页面。验证仅在点击后调用 `/auth/login-url`、合法 HTTPS 跳转、安全回跳、失败重试及不安全 URL 拒绝；未恢复旧账号表单。`bun run pretest` 全部65项/801断言通过。
- Agent schema：确认 `imageParameters` 是 E2 已实现合同，金样仅在 full/approval_canvas 新增该属性。新增发布背景参数覆盖默认值的审批用例，验证报价/冻结/执行保持相同原生请求、共享整轮授权、503后恢复原请求且不重发。`go test ./internal/app -run 'TestCloudAgent(ToolSchemaGolden|SupportedToolNamesGolden)|TestStudioPi' -count=1` 通过，包含真实 Node/Pi 免费 HTTP 夹具。
- 旧登录重开：真实 AuthSessionHydrator 在已有 Cookie、空身份内存或已知身份两种情况下均先 POST `/auth/session`，完成后才开放同账号工作区并恢复原偏好。后端测试从旧 GET 身份会话且无 grant 开始，文件读取409；POST 后文件307，有效 grant 重用，过期 grant 重开补发；不同 SID、跨账号、恶意 Origin、撤销继续拒绝。该启动路径已存在，本次补回归，无需用户退出重登。必须加载新页面代码且上游 IssueMaterial 成功；尚未刷新的旧页面与 W7 503不在此成功断言范围。

F3 验证：前端专项7项通过（登录/重开隔离 harness 共6项/66断言，首页另6项/60断言）；上述 Go Agent 专项和 `TestStudioMaterialFileWithoutOriginUsesSessionBoundWorkerGrant` 通过。变更 ESLint/Prettier、gofmt、diff whitespace检查通过。证据位于 `.local/verification/studio-followup-*.log`、`studio-agent-golden-parameters.log`、`studio-old-session-materials.log`。未修改部署/hostupdate 文件、远端服务或生产配置，未实际收费生成。

`d770e49 test(studio): 发布回归 - 修复异步断言并覆盖旧会话授权`：F3 已提交，未推送。

F4 直接媒体与画布节点：

- 协调侧浏览器报告添加图片节点时 `parameters` 崩溃。新 HEAD 也能重现：`CanvasNodePromptPanel` 的初始参数预检使用全局文本模型，`modelRequestOptions` 强行读取不存在的图片能力。现在缺失能力返回空参数，兼容性检查明确拒绝缺失能力；节点生成只接受当前目录中有对应能力的模型，没有时显示提示、隐藏参数设置并禁用提交。真实 React 节点先红6项/绿8项，覆盖图片/视频、新建/旧偏好与对应能力缺失；没有通过补造能力规避错误。
- 只读核对 W7 原生 Doubao 声明后，先红后绿修复 `ratio` 目录投影及内部 `720` 到发布 `720p` 的提交边界映射。原生视频真实首页按钮提交4秒/720p/16:9及新价格版本，报价版本变化时零任务创建。服务链保留 usage_schema 和精确小数上限；创建响应丢失只查询原 command。
- 图片和原生视频 HTTP 生命周期覆盖同字节报价/执行、媒体503与绑定响应丢失后的原请求恢复、固定 asset/version/file、目录版本变化后旧任务读取和旧价拒绝。前端图片/视频消费继续验证固定 Resource、幂等消息/画布回填、跨账号拒绝及清空内存后重读。未提交 tokens/video_input 计量事实，不在 Studio 固定五个 GPT 图片价格。
- 同名逻辑路由参考能力合并与 Platform 独立 SKU 隔离改用真实首页行为验证，移除依赖变量拼写的断言。

F5 发布门禁夹具修复：

- ecommerce 长图裁片/拖入上传夹具仍模拟旧 `/resources`，更新为 `/resources/platform-uploads` 与 v2 ready 回执。保留精确内容长度、来源对应 SHA256 命令、回执丢失重试、归属、换号拒绝、失败传播和资源释放断言。画布同步复制夹具补齐 platform-materials 导入和 useUserStore，意外调用仍报错。
- 注册协议用例原来检查已撤出的本地 AuthPanel 接线；保留未挂载旧表单的协议保护断言，实际入口改测 Platform 注册页与 dialog，包括无本地表单/协议 fallback、只在点击后取登录地址、安全回跳、失败及不安全地址拒绝/重试。
- `TestCloudAgentMissingCanvasStillCheckpointsMediaFailure` 在上游基线已有相同实现。100次运行复现人工推进与后台媒体等待器争写检查点，依照相邻测试关闭该夹具的后台等待器，保留缺失画布仍写终态失败且无幽灵节点的全部断言。修复后100次通过；生产并发实现未改。

F4/F5 验证：`bun run test` 的 pretest65项及主套件3290项/77460断言全部通过（481文件）；typecheck、全量Web lint、Vite build、变更Web Prettier、gofmt及diff whitespace通过。构建仅有已有动态导入/大chunk提示。`go test ./internal/app ./internal/handler -run 'TestStudio|TestCloudAgentMissingCanvasStillCheckpointsMediaFailure' -count=1` 通过，包含真实 Node/Pi 免费HTTP夹具；后端本轮未重复完整套件，PG/Redis与线上付费验收仍由协调侧完成。

证据位于 `.local/verification/studio-{native-doubao-*,canvas-capabilities-red,direct-media-web,direct-media-go,release-fixtures-complete,missing-canvas-*,release-full-web,release-typecheck,release-lint,release-build}.log`。未推送、部署或修改远端配置；待协调侧将本批合入后构建，复验真实画布添加图片节点及媒体执行。具体端点/字段见执行合同的 W7 直接图片/视频验收节。

`abf4d5e fix(studio): 发布回归 - 修复画布节点与原生视频能力边界`：F4/F5 已提交；协调侧已合入并部署其发布分支。

F6 媒体消费版本冻结（2026-11-08）：先记录执行合同，再以直接图片/视频消费头、Pi 加密 envelope、缺失/变化准入及浏览器提交测试重现 revision 丢失。核对发现已发布 rc.5 实际没有 revision 字段；协调侧发布 rc.6 后，应用从远端下载并固定 `v0.1.0-rc.6` / commit `889e4c75`，校验和 `h1:Kb7Xwb71a3IkjHr49sPivruTNhWuNYwMkJjwBRQYdsQ=`，go.mod/go.sum 没有 SDK replace。

- 直接生成：展示预览的 `capability_revision`、`connection_revision` 经浏览器传为 `platform.expectedCapabilityRevision/expectedConnectionRevision`；创建前既有预览复核必须与之相同。缺失/无效请求版本400，预览缺失/无效或版本变化409；新 Task/EXA 尚未创建。原生正文、三个版本、精确额度与资金偏好统一冻结到原 envelope。
- Pi：报价阶段保存两个 revision，批准读取原加密准备态，子任务复用整轮 EXA，不重新预览。缺失版本的旧审批准备态409；批准后平台能力/连接变化按原冻结消费头被拒绝，Studio 查询同 request ID 的明确 NoExecution 失败，不改用新版本或重发。公共审批 JSON/quoteId 合同未变化。
- 恢复：直接任务授权回执丢失后恢复同 task_reference，测试证明一次预览、同原授权、一次模型 POST和原请求查询；缺少版本的旧 authorizing/prepared 意图在新派发前拒绝，原密文不补写。已经 submitted 的旧任务即使缺少消费版本也继续原查询。图片/视频503与绑定回执丢失的固定产物恢复、精确上限、同字节原生正文继续通过。

F6 验证：应用/handler/platformintegration/repository 的 `TestStudio|TestCloudAgentMedia` 专项通过，包含真实 Node/Pi 媒体审批；platformintegration 全套通过；新增准入/恢复/审批与图片/视频冻结用例 `go test -race` 通过。前端提交、原生视频、结果回填、真实首页与审批卡共22项/96断言通过；typecheck、变更 ESLint/Prettier、gofmt、diff whitespace 通过；Go server 与 Vite 构建通过，Vite 仍有既有动态导入/大 chunk提示。本轮没有重复全量 Web/Go 测试，未运行真实 PG/Redis。

证据：`.local/verification/studio-revisions-{red-go,red-web,admission-red,validation-red-web,regression-go,regression-web,approval-web,sdk-consumer,race,typecheck,lint,build-go,build-web}.log`。本批没有推送、部署、操作 Platform/18420/生产 Console 或提交收费生成，原失败图片节点保留。待协调侧合入后做正式 EXA 与媒体成功回填验收。
