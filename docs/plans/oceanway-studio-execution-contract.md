# Studio 执行与恢复合同

## rc.7 原生媒体消费（2026-11-11）

当前 SDK 固定为 `platformclient/v0.1.0-rc.7`，发行提交 `9ed3fd9d6478ba04e8a55a190ce83908a395b8fa`，无本地 replace。下文 W7/rc.6 章节保留对应历史证据；新 Gateway 原生 profile 使用本节合同。

模型能力只消费 Platform 采用的 Gateway 发布版本。只接受已实现的 JSON POST/create 变体：OpenAI 图片生成/编辑、Gemini 原生/Chat 图片，以及 Sora、New API、ModelArk 视频。能力必须明确声明 path、kind、request_schema、defaults、modes 和输出限制；未发布、未适配、仅 multipart 的变体不可用，不根据模型前缀补造能力。

普通按钮、画布、批量设置和 Agent 共用 `nativeVariantId` 与 `nativeParameters`。前端配置中的参数是 JSON 字符串，发送给 Studio 的 `input.config.nativeParameters` 是以发布 JSON Pointer 为键的对象。表单按 schema 展示原字段/默认值/enum/const，切换变体清除旧参数；复杂对象和数组使用 JSON 输入。后台合并所选变体默认值并验证完整原生请求及条件，不拼接另一变体的参考数量或取值范围。`false`、`0`、`null` 和嵌套值保持原样，旧 size/videoSeconds 等控件不覆盖原生参数。

模型 ID 与调用路径来自发布合同；ModelArk 使用 `/api/v3/contents/generations/tasks`，New API 视频使用 `/v1/video/generations`，Sora 使用 `/v1/videos`。请求时长是原生 `duration`，不生成 `requested_duration`。预览携带 `NativeProtocol`，同一原生正文/三版本/额度冻结后提交；异步偏好走 SDK `PreferAsync` 请求头，不注入正文 `async` 或 `stream`。图片 `/n` 控制单次请求输出数，不能在浏览器重复提交 n 次。

参考输入只从当前用户的 ready Platform Resource 固定版本解析到 `material://asset/version`，按当前变体校验 role、数量、MIME、字节及尺寸关联；禁止参数覆盖提示词、素材容器或模型信封。Agent 草稿、报价、批准与执行保存相同原生参数；审批只读，调整须编辑草稿并重新报价。设置了本地视频时长上限时，无法确定上界的自动时长明确拒绝。

`/create` 结果通过固定 Resource ID 转入画布，逐项重新核对归属/就绪/类型并在持久化成功后消费 handoff URL。重复恢复按 storageKey 幂等；不创建旧 Asset 或再次上传原件。视频静态封面通过原 Resource 的 `poster` 访问，不在浏览器抽帧上传另一份图片。画布只保存固定引用和本站资源 URL。

本地免费联合验收、重现步骤与限制见 [rc.7 联调记录](../verification/studio-native-rc7-20261111/README.md)。七种协议组包测试不等于七种真实 Gateway 供给全部开放；复杂 JSON 编辑体验、商业供给、实际 Pi 浏览器链和其他产品分别验收。

## 入口与权威

普通生成与原生 Pi 的模型步骤共用 OAC Task、队列、租约和文本回放。服务端重新读取 Platform 发布目录，只接受文本、图片、视频的明确协议。浏览器不能指定供应商地址、凭据、工作流或本地计费。音频及未适配协议返回暂不可用。

`POST /api/tasks` 接受稳定 `commandId` 与 OAC 创作意图。同一账号同一 command 只对应一个任务；修改意图返回 409。创建响应丢失时按 command 查询原任务。先保存不可变意图，再以该任务 ID 申请 EXA；SDK 的同 task_reference 原授权合同允许恢复授权响应丢失，不产生新调用。

## 持久化与派发

schema 94 新增服务端专用 StudioExecution。原生协议、精确请求字节、Content-Type、价格版本、额度约束和原授权加密保存；任务公共 JSON 不包含这些数据。保留 `.settings-key`，不保存 Cookie。执行表和 OAC Task 通过事务进入队列。

Worker 持有效 OAC 租约，以条件写把 prepared 改为 submitted，成功后才发送唯一一次模型 POST。断线、超时、503、进程崩溃、not_found 都只进入原 ID 查询；不换模型、不补发、不构造新收费请求。即使崩溃发生在标记后而网络发送前，也保持待核对，不假定没有执行。

文本只使用 SDK 非流式 ChatCompletions / Responses / Messages；将最终结果接回 OAC 回放/SSE，保留工具调用和思考签名。上游成功正文先加密落盘，随后解析并与业务结果原子完成。空输出或格式错误保留原响应，不能自动重新收费。失败仅依据原请求的明确终态。

图片/视频冻结原生 JSON，以同一 JSON 做 PreviewMedia 和 SubmitMedia；必须验证 allowed、combination_supported、price_version 与明确 wallet_max_points。参数按所选发布变体校验，不合并其他变体。平台负责扣费与精确价格计算，本地不预占/退款。

## 素材与 Agent

引用只接受当前账号已采用的固定 resource → asset/version/file 映射；提交使用 material://asset/version，由 Platform 准入。产物通过原 EXA 查询 TaskOutputs/Original，固定版本采用、绑定确认后才宣布任务成功。签名 URL 不作结果身份。

Pi 保留原编排、审批和工具执行。后台只持原 EXA，不持浏览器 Cookie；每个付费步骤先持久化独立请求标识。审批后的创作任务走相同入口，未知结果保持原步骤等待恢复。已提交任务不经旧的自动重试、供应商取消或本地账本。

每轮 Pi 由原生非计费控制任务承载，一轮一个 EXA；目录快照与 EXA 加密保存，每个模型/压缩步骤使用独立 Task 和稳定 request ID。步骤完成不能释放整轮授权，只有轮次结束且已提交任务全部恢复到终态后才能释放。停止轮次通过与派发共用的数据库锁取消未提交步骤，已提交请求继续恢复。平台管理余额及结算，Studio 不宣称本地累计积分上限或累计消费；每轮必须明确 1–64 步上限。

Pi 媒体审批分为草稿、报价、批准三步。后台只按冻结目录准备原生草稿；浏览器显式 POST 报价，以真实会话获取新目录和 PreviewMedia，展示十进制积分上限与有效期。批准必须携带该报价 ID；不自动换价或接受编辑后的参数。报价冻结原生正文、所选变体、画布依赖和固定素材，原始信息加密保存。批准时校验报价/依赖/归属，与同轮 EXA 的媒体 Task 和 Agent 检查点同事务入队；拒绝不提交任务。并发批准和响应丢失只能恢复原决定，不能再次提交。参数修改通过拒绝、编辑草稿、重新申请报价完成。

## 验证顺序

先写 HTTP/数据库红灯：三种文本协议、响应丢失后进程恢复、同 command 幂等与冲突、授权丢失恢复、跨账号隔离、租约竞争、密文/日志泄漏、未知请求不重发。再验证精确媒体报价、固定素材引用、产物恢复和 Pi 工具调用。只用隔离免费夹具；真实登录/付费验收交由协调侧。

## W7 直接图片/视频验收合同（2026-11-08）

本批仅核对 Studio 应用，不改 Platform/18420，不部署、不提交真实生成。协调侧负责五个 GPT 图片模型人民币0.2元/张与视频官方标准 Token 费的登记和最终验收；Studio 不写死人民币金额、汇率或 Token 公式。

已只读核对协调侧保存的 Seedance 插件能力声明及 W7 定价快照：原生视频协议为 `doubao-video`，执行路径 `/doubao/api/v3/contents/generations/tasks`，profile mode 为 `native-doubao-video`。默认变体以 `ratio` 表示比例，`resolution` 是 `480p/720p`（部分模型另含1080p），`duration` 枚举含4秒。价格 `usage_schema` 的 `tokens`、`resolution`、`video_input` 是计量事实；请求参数来自 `media_generation_profile` 的所选变体，不把用量字段作为用户请求参数。Platform 根据原生请求计算预估和终态用量。

先红后绿修复两处：目录投影只读 `aspect_ratio` 导致原生 `ratio` 遗失；前端内部保存 `720`，提交时未恢复发布值 `720p`，违反后端所选变体 JSON Schema。现在投影优先使用声明的 `aspect_ratio`，否则读取 `ratio`；Platform 提交边界仅转换能匹配发布枚举的分辨率，未知值仍交准入拒绝。现有历史快照中的启用状态和价格不作为协调侧本轮新发布成功的证明。

验收入口：`GET /api/model-catalog` → 真实首页生成按钮 → `POST /api/tasks/preview` → `POST /api/tasks` → `GET /api/tasks/:id`。目录保留原始 `platform.price`（含 usage_schema/version）；提交 `platform.expectedPriceVersion`，报价返回 `price_version` 与精确字符串 `wallet_points` 后固定 `walletMaxPoints`。版本变化或报价超限须拒绝；提交响应丢失只查 `/api/tasks/by-command/:commandId`。后台以原 EXA/原 request ID 恢复 Platform task，再通过 TaskOutputs 的固定 asset/version/file、Binding 确认得到 `resource:`，回填消息及画布，不走旧 Asset 物化、不重新生成。

免费验收已覆盖：真实首页视频按钮将 `input.config.channelId=oceanway-platform`、原始模型名、`size=16:9`、`videoSeconds="4"`、`vquality=720p` 送至任务预览，版本变化时零任务创建；服务链保留 usage_schema、新价格版本和精确小数上限，创建回执丢失只查询原 command。图片和原生 Doubao 视频的 HTTP 测试验证预览/执行原生正文逐字节相同，媒体503后原请求恢复，绑定回执丢失后恢复同一绑定，最终固定 asset/version/file；目录更新后旧价预览409且原任务仍可读。原生视频不提交 `tokens` / `video_input` 计量事实。前端结果测试继续覆盖图片/视频的消息与画布回填、重复消费、跨账号拒绝及刷新重读。

画布节点补充：全局默认可能是只有文本能力的模型。节点参数预检不得读取缺失的图片 `parameters`；缺失能力返回空参数，不补造默认能力。真实图片/视频节点在当前目录没有对应模型时显示提示、隐藏参数设置并拒绝提交；有已发布模型时按节点模式解析，恢复旧模型偏好不崩溃。组件夹具不等于线上浏览器验收，实际图片生成与720p/4秒视频验收仍由协调侧执行。

## 媒体消费版本冻结补齐（2026-11-08）

先红后绿确认：直接生成与 Pi 审批原来仅冻结价格版本/额度，丢弃消费预览的 `capability_revision`、`connection_revision`，正式 EXA 无法通过能力准入。已发布 rc.5 也缺少这两个字段；协调侧已发布 rc.6，应用已从远端固定 `v0.1.0-rc.6`（commit `889e4c75`），没有 SDK replace。首轮失败测试曾使用独立副本，正式回归全部使用已发布模块。

当前合同：三种版本与原生正文、精确额度一起保存进原 `studioExecutionEnvelope.Consumption`，经 SDK `MediaConsumptionConstraints.ExpectedCapabilityRevision/ExpectedConnectionRevision` 发出；不新增授权或任务体系。capability revision 必须为平台发布的64位小写十六进制，connection revision 必须为正整数，缺失不得回填当前目录值。浏览器还会拒绝不能精确表示的整数，避免连接版本在 JavaScript 中失真。

直接生成的现有展示预览与创建前复核保留：浏览器从展示预览冻结两个 revision 并作为 `platform.expectedCapabilityRevision/expectedConnectionRevision` 提交；创建复核必须与其相同，变化返回409，缺失请求版本返回400，不能接受新版本代替原确认。授权响应丢失时读取已加密 envelope，恢复同 task_reference，不重新预览。Agent 的报价阶段冻结两个 revision，批准只用原报价和整轮 EXA，缺失的旧审批准备态直接拒绝；版本在批准后变化由 Platform 对原冻结约束拒绝，Studio 查询原请求的明确失败，不重报价/新建请求。

旧持久任务：尚未提交且缺少冻结版本的 envelope 明确失败，不补造版本、不调用供应商；已经 submitted 的原请求即使缺少旧字段也继续原查询，不误报未执行或另行提交。验证先补 HTTP/数据库与浏览器服务红灯，再接线：图片/视频的完整消费头、缺失及版本变化拒绝、审批持久化、授权/执行回执丢失恢复、零额外报价与零重复模型 POST。

## 固定参数与图片尺寸边界（2026-11-08）

协调侧反馈发布 f6de329 的真实图片任务成功并已显示；Mini 视频止于 preview400，未建 Task/未调用供应商。应用失败测试确认发布字段 `resolution: {type: "string", const: "720p"}` 被目录投影漏掉（原来只认 enum），内部 `720` 无法还原；Pi 同样不能从 const 取得默认值。现在 const 投影为单一可选值及同类型默认值，通用于图片参数、视频分辨率/比例/时长。直接提交继续按目录恢复原值；Pi 仅将能匹配所选发布值的内部表示还原。缺省参数可使用已发布 const/default，显式冲突仍由完整 JSON Schema 拒绝，不能静默换成支持的规格。原三版本、正文、EXA 与恢复合同保持不变。

协调侧确认成功图片原件为1024×1024，但画布显示320×320。SDK Asset/File 尚无原件尺寸；画布从 thumbnail 自然宽高写入 naturalWidth/naturalHeight，图片信息栏还会用节点显示框兜底。现在依据访问回执 actualVariant 决定测量资格：有完整原件尺寸或当前内容确为 original 才记原件宽高；thumbnail/preview 仅展示，缺少原件尺寸时保持未知，信息栏显示「尺寸未知」。display/original 实际返回 preview、加载失败后重签仍返回 preview 均遵守相同边界；不为尺寸显示下载原件、不推算生成参数对应的产物尺寸，也不自动改写既有画布。保留已有正确尺寸和本地原图解码行为。
