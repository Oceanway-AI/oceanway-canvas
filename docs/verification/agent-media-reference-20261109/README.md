# C4 Agent 媒体审批与参考素材验收

日期：2026-11-09，独立 W7 / oceanway-canvas 测试入口。本批原 Agent 审批、真实图生视频与关页重开通过；C4 整体及其他未列项不代表已完成。

## 本批代码和运行边界

- Studio `d04c2a6`：原生 Skills 读取与已有技能使用路由由 OAC/Studio 共享注册；外部安装、发布、技能管理仍关闭。未传明确消费上限仅预览，不写可执行报价或审批事件。
- Studio `809b5e3`：原生单一 Agent 行为合同 v21/编译器 v7，区分 Platform 与本地资金；Platform 不投影本地 maxCredits=0，摘要恢复保留该事实。没有新增执行或授权框架。
- 后端已部署独立 18448，前端保持 `ff6a3ce` 的镜像 `55a46ea43f12…`，原 Pi 和 schema95 保持。Platform/18420 未更换镜像。
- Mini 通过正式管理 API 补充一张 reference_image（PNG/JPEG/WebP，12MiB上限）。实际原插件 createTask 转发 content，来源代码及旧接口文档仅作为结构依据；完整多模态上限不在本批验收范围。发布版本见 `mini-publish.json`，价格/连接/路由未变。

## 先 RED 后修复

1. `agent-preview-red.log`：未确认上限的报价返回可执行 ID。修复后取消/重读不持久报价，后续只读预览不替换已确认额度。
2. `agent-skills-red.log`：正式 Studio `/skills` 返回501。复用原生路由后，账号隔离、私有拒绝、添加与使用模式生效；管理员写入保持关闭。
3. 浏览器首轮读画布成功，但目录零参考图与旧 maxCredits=0 阻止生成，截图 `agent-blocked-before-policy.png`。该轮未提交媒体。`agent-policy-red.log` 重现错误执行事实；修复原生合同、编译及摘要后回归通过。

## 免费验证

- App 42、Skills 14 个用例（含子例）通过，见 `agent-c4-green.jsonl`；其中首次 Handler 新夹具误以为可移除自建技能，按原 OAC“自建技能始终保留”更正期望，未改产品行为。
- 最终 Handler 33 个用例（含子例）通过：`agent-c4-handler-green.jsonl`。
- 取消报价与账号 Skills 的 race 专项通过：`agent-c4-race-final.log`。
- 前端原消费确认与Task提交专项18项/42断言：`agent-preview-web.log`。未修改前端生产代码。
- 新 Platform预算与原生本地预算、压缩摘要、原审批链回归：`agent-policy-green-final.log`。受影响 Go 包 vet 通过；linux/amd64 后端构建和实际健康检查通过。
- 没有运行全仓CI，不声称其他工作包或全部故障矩阵通过。

## 真实验收

画布：`https://test.oceanway.tech:18448/canvas/sQ3Xqpn5rFRjYHy6Pjigf`。

1. 原生 Agent 读取画布与模型，创建参考草稿并等待审批。打开消费预览后取消、刷新/重开，仍没有可执行报价 ID、加密计划或新媒体任务。随后“暂不执行”，原运行 `ag2668ba5c6963a004f6d0f0cb04884255` 为 rejected，空草稿及连线保留。证据 `cancelled-preview.json`、`rejected.json`。
2. 新运行 `agebdddd12eb9546b86f13e02d8cab915a` 明确创建新视频，审批 `agebdddd12eb9546b86f13e02d8cab915a-3-0`：预估198.72点，手动确认最高250点，再点击一次原生同意。媒体 Task `ag92cdfea00afe0b4607ebc07a22675f08`；根/全部子步骤的原加密授权一致，只导出相等布尔值。
3. Platform `media_e7bc0b3194cc6eba65cfedf0d0c6117aec2e96ea93c51006061dbc893d76f097`，request `studio-ag92cdfea00afe0b4607ebc07a22675f08`；原 operation `op_67369b30b603c0ddd33ac81bbcb0ca9ce038ac5210296cad`。网关 `task_3iCv09iv3guRMOdU7N0oHdyFq2UjVtkY`、供应 `task_zEkLP6o72utxzu9xQAY5Qf2OTCKTqTBz`。仅一个媒体请求，实际 action=image_to_video，87,299 tokens，结算200.7878点，未超过原250点上限。
4. 输入是原茶壶图片固定素材/version/file，`task_input` 的 `/content/1/image_url/url` 引用从 active 变 released。没有把展示用签名地址持久化成身份；原件供应访问票据为24小时，与15分钟展示票据分开。
5. 原 material-worker 一次保存成功：MP4 2,204,447字节，SHA256 `dc9a1121e747910fbf9e23e6f2e2e8c223b5038884a4dded4032096e5d6da0ea`；Studio Resource `ba707dd5-b219-5f45-b3fc-9c1239caf898`，画布节点 `ag24d62c07bd0d2c0c9a23a0df76525007`。授权原件GET摘要/长度一致；CDN封面及播放版两次GET摘要一致。本次没有X-Cache头，不声称缓存命中验收。
6. 供应处理中真实关页重开仍等待同一Task；完成后再次关页/新开，4节点、2连线、Task、素材版本、Resource及原资金逐项一致，原图片/小船节点未变。浏览器重开后播放至0:04；Agent恢复同一完成对话。`checks.json`、前后快照和截图提供对应证据。

`video-probe.json` 使用现有 material-worker 的 ffprobe 读取已验原件（没有重生成）：H.264、1280×720、AAC，容器实际时长4.042秒。宿主机未安装ffprobe的初始记录保留在 `material-verification.json`，不改写为成功。

## 发布、保护与接续

代码提交 `d04c2a6`、`809b5e3` 均在 Studio；当前backend为809b5e3 / `b071f12d2d84…`。两次测试发布各有数据库备份，web/Pi/schema95保持，Platform dba921b5及18420原插件未替换；Mini能力经正式管理保存/check/publish生效，未改价。

`protected-services.json`：两次发布各自的110个受保护容器及nginx保持；整轮原有108个受保护容器、两个Console容器和nginx均不变，但服务器另外出现4个news候选/验证容器，非本批创建，不声称整台服务器容器集合不变。`old-operations.json`核对两笔旧受限视频的状态/预留/上限不变。

下一批优先补 C2 两类图片202异步与原ID恢复、另一具名视频系列，再补C1跨进程投影/缓存与C3/C4完整异常矩阵。C4尚缺无画布本批真实媒体、付费运行时Backend/Pi重启、并发编辑/删除、团队ACL全链；Resource行宽高/时长元数据未回填的问题仍保留。没有以本批一次Mini参考图成功代表全部型号、多参考或视频/音频参考支持；C5未完成。

证据脚本 `check_final.py` 仅核对已脱敏本地快照，可离线重复；`snapshot.py` / `verify_materials.py` 是具名测试环境只读取证入口，凭据只从远程私有文件/运行环境读取，不提供执行新生成的逻辑。

私有 Cookie、EXA、产品 Key、签名URL、数据库备份不入仓。旧失败/受限任务保留且不重试；生产Console与其他受保护服务不切换。
