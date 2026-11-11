# Studio 本机真实媒体接线与提交诊断

本次在 `156d961` 基线上建立独立分支。产品继续使用正式 `platformclient v0.1.0-rc.7`，身份、模型目录、费用预估、执行授权和素材均通过 Platform。未在产品内直连 Media Gateway 或供应商。

## 产品端修改

- `STUDIO_PLATFORM_BROWSER_ORIGIN` 可指定浏览器能够访问的 Platform HTTPS 地址。内部服务请求地址与用户登录/控制台链接分别使用各自配置；内部账户归属与原任务标识保持不变。
- Worker 保留初次媒体提交的安全诊断，仅包含 HTTP 状态与结果是否确定，不保存公开的服务端正文、凭据、供应商地址或素材参数。
- 只有原请求 ID 匹配、版本为 `oceanway.media-request.v1`、`status=FAILURE` 且 `no_execution=true` 的 Platform 回执可按未执行拒绝终结。回执可关联 Platform 的稳定拒绝任务 ID。普通 400/503、网络异常、不同请求或错误版本继续查询原请求，不能推断为可重新提交。
- 明确的初次拒绝回执加密保存，重启后仍采用同一判断。成功执行继续按原 Platform Task 查询产物、核对固定 asset/version/file 身份并采用素材。
- 协议选择器按已发布的参考素材字段显示可区分的选项，包含首帧/尾帧、图片/视频/音频参考、必填性与数量限制；零参考的视频选项标记文生视频。不按型号名称猜测能力，也不把多个协议的参考能力合并。选项保留原始 variant ID，较长的说明可换行显示。
- 媒体恢复中的 `task_available` 只表示原任务存在。`UNKNOWN`、`NOT_START`、`SUBMITTED`、`QUEUED`、`IN_PROGRESS` 每轮继续查询原请求，重启后同样处理已缓存回执；只有 `SUCCESS` 开始读取产物。回执版本、原请求 ID 与已冻结任务 ID 必须一致，更新受到任务租约和原密文比较保护。`FAILURE/no_execution` 与已执行任务失败分别呈现，不推断退款或重复生成。
- 列表和详情从已保存的原请求 `FAILURE/no_execution` 回执展示“未执行生成”，不会继续沿用提交时的等待诊断；读取展示不回写业务数据或推断退款。后续拒绝终态将阶段与诊断一起保存。Platform 图片/视频任务详情的 worker 次数标记为“处理次数（含状态查询）”，不称为生成尝试次数。
- 任务详情从正式结果的 `resource:` 标识读取图片/视频，不再只靠 URL 扩展名识别结果。详情及缩略图通过 ResourceAccess 的 preview/thumbnail 获取展示，图片放大通过 original、视频通过 playback；访问失败或资源身份不匹配不会回退到供应商地址。原始数据不回写，不重复上传或生成。

## 本机验收边界

本机 Studio 正式服务、Worker 与已构建页面启动成功；普通测试主体正常登录，模型目录读取成功。Gemini 图片、GPT 图片、ModelArk 视频的真实费用预估已返回允许与固定版本。视频的 480p 请求被当前发布能力拒绝；720p 请求通过。实际预估金额与发布价格挂钩，不能以测试金额证明真实采购成本。

素材授权曾返回 503，定位到 Platform 本机缺少素材授权签名密钥；由 Platform 运行 Owner 补齐后，Studio 素材授权及素材列表读取成功。

浏览器采用受限回环 HTTP 反代，转发到固定本机 HTTPS 服务并验证对应证书；只映射固定 Host/Origin，拒绝外来 Origin 与未携带 Origin 的写请求。不修改系统域名解析或证书信任。这一入口用于真实产品能力、预估、生成与产物展示验收，不证明跨域 SSO/TLS 部署。

真实生成、实际扣费、最终原件读取由主会话统一控制，采用用户明确的总费用上限；产品子任务不追加付费调用。初次 Gemini 提交遭遇 Platform 型号状态分类错误，Platform 修复及原任务结果由对应 Owner 记录，本文件不将未完成任务写为成功。

2026-11-12 实际只读核对：Gemini 原任务 `76fad0fd-6cdb-57bb-b08e-f2117490d4a3` 与 OpenAI 原任务 `5ce61dd9-7b50-5c9b-9580-ee51f2d13ad5` 均自然进入 succeeded/completed；各自 Gateway 只有一个成功 Attempt。Gemini 的 `aspectRatio=16:9/imageSize=1K`、OpenAI 的 `size=1024x1024/n=1/quality=low` 在 Studio 冻结正文与 Gateway 接收、Attempt 快照一致。原视频 `6ccfdb62-6581-56ac-add7-4ffd199e9d82` 实际选择 Sora `POST /videos`，正文是 `duration=4`、`metadata.resolution=720p`、`metadata.aspect_ratio=16:9`；该原任务被旧发布调度政策阻塞，尚无 Attempt，本次不写为视频成功。请求不使用 requested_duration、second 或 seconds；Gateway 内部 duration_seconds 不作为公开参数。

主会话使用普通 Chrome 复验成功图片任务详情及放大原件，随后将两张真实图片通过素材库插入同一画布并观察同步与显示。证据位于本机联合验收目录 `MG-CANVAS-LIVE-20261112`：`native-parameter-chain-readonly.json`、`studio-detail-results-before.json`、`canvas-images-reuse.json`；页面截图 `studio-openai-detail-before.png`、`studio-openai-detail-after.png`、`canvas-real-images.png` 由主会话保管。本次只更新前端静态构建，后端保持 `f871e57`/PID 47917，无重启或追加付费请求。详细费用和最后视频验收由主会话统一收口。

## 本机检查

- Gateway 登录边界和公共 HTTPS 跳转回归通过。
- 首次媒体拒绝六种情形回归通过：明确拒绝、持久拒绝任务、普通 400、503、不同请求、错误版本；断言只发送一次生成 POST。
- 相关媒体版本冻结、初次拒绝、原请求恢复、固定素材回填及登录回归通过；Platform integration/handler 的 vet 与正式服务编译通过，差异检查通过。
- 原版本漂移测试遗漏正式恢复合同的 version 字段，补齐后再次通过。没有放宽正式回执版本校验；未运行付费夹具替代真实验收。
- 协议选项标签及原生参数恢复五项 Web 专项测试通过；修改文件 ESLint、完整 Web 类型检查与生产构建通过。真实页面展示由主会话使用普通 Chrome 的回环预览入口核验。
- 原任务恢复专项覆盖五种正式非终态至成功、已缓存未知状态后重建 Service、原任务失败与未执行拒绝、请求/任务/版本不匹配、旧观察及过期租约不能覆盖新回执；固定原件采用流程仍验证只生成一次。定向 handler/repository 测试、恢复与租约的 race 检查、相关 vet、正式服务编译通过。
- 任务详情正式资源解析及访问专项 5 项、已有媒体失败占位/任务操作回归 5 项通过；修改文件 ESLint、完整 TypeScript 检查与 Vite 生产构建通过。真实结果读取回归只读使用既有任务，无重新生成。
