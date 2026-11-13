# W7 图片/视频定价与真实验收（2026-11-08）

用户确认：已接入的五个 GPT Image 模型售价为人民币 ¥0.2/张；视频先用供应文档官方标准 Token 费用，不等商务折扣，完成配置并跑通。目标为 W7 18447 与 Studio 18448，生产 Console 不切换；原失败任务不重试。

核查前状态：五图三视频目录与连接已存在。GPT Image 2 已发布的能力快照价格版本过期，其他七个尚未发布/启用；三视频没有持久价格。这些是当时观察到的配置缺口，不能作为提示全部根因。价格、模型发布和服务运行开关是独立步骤。

配置顺序：读取最新版本、保存范围快照；免费价格预览（图片1/2张，视频无/有参考视频及实际 Token）；通过原管理 API CAS 保存；发布精确能力参数，启用并回读实际 ready 状态。保持原 TaskAdaptor、资金及素材 worker。价格变化只影响新任务。

图片：现有政策 points_per_cny=100，cny_per_standard_unit=1，model multiplier=1；因此目标为20积分/张，原生计价表达式按图片数量相乘。此1:1是测试平台配置，不是外汇市场汇率。不改变全局政策。

视频报价来源：https://doc.newtoken.club/api-docs/seedance-official.html 。Fast 37/22、Mini 23/14、2.5 70/42 元/百万 Token（无/有参考视频）。以 usage.total_tokens 结算；先按720p、4秒、无参考素材做最小验收，不把仅目录确认写为已生成。旧18420存量duration表达式与插件tokens用量不匹配，必须通过其原管理接口修正后才提交视频。

预计验证：五图三视频配置/预览/目录全覆盖，真实生成先选GPT Image 2一张和Seedance Mini一段4秒；保存原请求ID，原Task查询恢复，核对Platform素材及画布回填。其余模型仅记配置验证，不声称逐个收费实测。

当前结果：五图三视频价格已保存并发布，19项免费价格预览通过；仍未提交真实图片或视频。实际 media/config 回读 enabled=false、reason=disabled，容器和持久 platform.env 均 OCEANWAY_PLATFORM_MEDIA_ENABLED=false，造成8个模型统一 unsupported_usage。先保存失败验收，再仅开启 W7 platform 该开关；回读实际状态并比对保护服务指纹。不能只隐藏提示或把配置成功写为生成成功。配置备份与含凭据的验收状态均仅保存在.local/private；本文件禁止存储密钥、Cookie和签名素材URL。

总开关已实际开启，44个非W7容器和宿主nginx指纹未变。五个GPT图片已 available_media；三个Doubao readiness configured但价格页仍unsupported_usage，具体源码缺口为 platformMediaExecutionAllowed 未包括 native-doubao-video，正在独立Platform分支补回归与修复。

浏览器：统一登录已返回Studio并可打开原画布；当前a5054c2首页references和添加图片节点parameters崩溃，应用会话处理中，本轮未创建收费任务。18420管理员已由用户亲自登录；编辑Mini期间发现并发配置变更，已暂停后续价格写入等待协调，不能把期望表达式当作实际保存结果。

## 续批实际状态

应用崩溃已由abf4d5e修复并合入0e15a36，测试18448部署0e15a36。完整应用门禁11项检查、0失败（含已有明确跳过范围）；三镜像与独立数据库备份在服务器private目录。原生画布恢复正常。

第一次真实按钮点击：画布ow3bFkuS2McNFgipOuhGZ的红纸船图片节点，2026-11-08T07:08:15.233Z，POST /api/tasks/preview返回409，request_id=req-de71b494737c2a5167e78475。Studio tasks=0、executions=0，Platform仍仅2条原文本任务，未产生供应图片请求。失败节点保留不重试。

定位到正式消费预览HTTP200但combination_supported=false/output_mapping_unverified。W7缺少原生输出编号开关；已有原生图片解析与素材worker支持，3项正式HTTP免费版本/归档/恢复回归通过后仅在W7开启。现在五图三视频均allowed/combination_supported=true。5图每张20积分；视频Mini720p4秒预留198.72积分，最终以usage结算。

SDK rc.5缺少当前Platform版本字段，rc.6已发布，tag指向889e4c75ec312f7e517d2da146a579be6572d944，空缓存下载验证通过。SDK与实际应用分别验收；应用会话负责直接按钮和Agent原envelope三版本冻结，未部署前不得记为真实生成通过。

用户最新明确18420价格已经亲自配置，本批停止价格写入。只读核对现存三模型使用tokens/resolution/video_input表达式，保留其币种及分组设置。本次不会把该设置当商务结算审计结论。

后续收费验证使用新画布-bKBYMSjbXzF6r99d1Nx-（Studio 图片视频真实验收 1108），保留原失败画布。新图片提示词为黄色陶瓷杯，待SDK冻结修复部署后点击一次；视频选择Mini720p4秒。所有请求只查询原记录，不自动重试。

## 真实图片已通过，视频参数回归待修

测试18448已部署f6de3296b1c160547a47f5c520bfd606fcc03fb6（合入应用2e2c2f2），固定正式rc.6；结构检查95/95，100个非Studio容器及nginx指纹不变。

- 原生图片按钮仅点击一次。Studio Task `09bbd47a-5747-5a83-9e9e-92adb5911064` 成功；Platform Task `media_cf352dba4b9ff9137baa34d454457db4515108b9a22526a2626c2994da26c8c7` SUCCESS，资金settled、result saved，100000内部quota=20积分，冻结0。
- 18420日志641、渠道13、model=gpt-image-2、一次type2成功；原请求 `202611080736303235160438268d9d63AhQtW14`，供应请求 `202611080736304069842688268d9d6GMr1KECy`。Studio attempts=3是原Task轮询调度次数，不是供应生成次数。
- 原件PNG 1024×1024、1039672字节，SHA256 `8201e10b72ee6a3c340a3cea4dd6a4816f8de31e1507d7ae7b14fac9f9ffa3f0`。Platform worker saved一次；原件下载摘要一致，thumbnail/preview从cdn-test.oceanway.tech读取、重复字节一致（未据此声称缓存HIT）。原件/派生URL不入仓。
- Canvas绑定原生resource `edb37a3c-7819-54a8-b8f6-168d59c661d8`，图片已显示；UI误把thumbnail的320×320显示为原件尺寸，应用侧补回归中，不能把该尺寸当真实原件。
- 同画布Mini视频节点 `video-1794122853974-z6ejc` 首次点击在 `/api/tasks/preview` 400拒绝，request `req-05777111c02c25c1ebdf4bbc`，原因“生成参数不符合所选模型的发布规格”。无Studio Task、无供应视频请求、无扣费。实际发布resolution为const 720p；目录投影只读enum，漏const，前端提交内部720。应用侧补精确schema回归，不改Platform规格来绕过检查。此失败节点保留，修复后仅用新节点继续验收。


## 视频修复与新任务

应用修复f678990已合入并部署dd7d6c6616f5a59353e8a89e7801301b15ce7860；原const规格投影/Agent默认与输入归一化、派生图尺寸来源已修复，前端34项及Go专项/构建通过，历史画布不批量改写。已有图片节点此前误存的320×320仍保留，原件以已核对的1024×1024为准。100个非Studio容器及nginx指纹不变。

新视频节点video-1794124436312-iaknp（白帆船，Mini720p/16:9/4秒、声音开启），原生按钮一次。Studio Task e85871d9-c8c7-5b61-8ac4-e160e75db36f；Platform Task media_b760a30df354bd4c96b069df1a6ab56968ab9f333eebf0cdd52299f7931b0472；18420原生Doubao task_avCw4Ci08kogPk00EICu4gpOlgAt0bOO，渠道5。已获原任务回执，当前排队，尚不记成功。浏览器标签页已关闭，后端继续查询原任务，无重复提交。

仅修正18447 GPT Image2的过期营销文案“1积分/张”为按消费预览确认，并移除该旧标签；没有更改价格或绑定。18420由用户配置的报价保持。

## 视频轮询失败的实际根因与修复门槛

原视频供应任务 `task_iEyD8wti2SzZt5Nreo2rlMZCPGQ2VJcG` 返回 HTTP 200、`status=processing`；18420 的内置 `plugins/tasks/doubao/plugin.js:965` 只识别 queued/running/succeeded/failed/expired/cancelled，连续20次记为 UNKNOWN 后停止跟踪。2026-11-08 08:02 UTC 左右网关与 Platform 已失败，但供应任务随后直接只读查询仍为 processing。不能将此写为供应方生成失败或继续排队成功。

修复先补实际插件测试：processing 必须保持 IN_PROGRESS，不能成为终态或触发结算；pending/未知值仍为 UNKNOWN；成功与失败规则保持。该兼容映射只根据本次已观测响应增加明确状态，不改模型参数、价格、授权、资金链或重新提交逻辑。需核对实际运行插件版本，免费通过不等于已部署。

Platform operation `op_eef8c25e9c8b294ea69358c841d17837ac7fbe6cdb0ee470` 已 funds_state=released、reserved_quota=993600、frozen_quota=0、settled_quota=0。Studio 原 Task 已 failed，浏览器重开显示原失败节点。不得手工改回 running、重试或补扣；供应方后续结果与结算需按原 ID 单独核对。

修复已提交在独立 new-api 分支 `codex/legacy-doubao-processing-20261108`，提交 `6e517a322`（基于 `706144769`，不是直接构建该整版覆盖线上）。修改仅为内置 Doubao 的 processing→IN_PROGRESS 及原插件测试；待发布补丁适用的运行源码 SHA256 为 `58da524648f3b43e6944c34cc14de59b7162123fe046b033037680a0fa4d182b`，已与本次修改前插件逐字节匹配。实际18420仍为87e1f9f，补丁未部署、未推送。发布时应在其原异步图片版本上应用此小补丁，不丢失原异步执行改动。

失败测试 `go test ./plugins -run '^TestDoubaoNativeOfficialBodies$/query_replies' -count=1` 观察到 processing=UNKNOWN，见 [失败记录](doubao-processing-red.txt)。修复后 `go test ./plugins ./relay/channel/task/jsplugin -run Doubao -count=1` 两包通过，见 [免费回归](doubao-processing-green.txt)。未调用供应方生成、未更改网关价格、未改写失败任务。

## 本批验收结论与后续

| 项目 | 实际结果 | 证据 |
| --- | --- | --- |
| 五图三视频目录、价格及消费预览 | 免费配置验证通过；只有 GPT Image 2 和 Mini 发起过本批真实生成 | 前述配置及原Task记录 |
| GPT Image 2 原生生成→Platform入库→原件/派生访问 | 通过，一张图片，原件1024×1024，摘要及字节一致 | [素材验证](real-material-verification.json) |
| 关页重开 | 3节点、0接线、2原Task、图片resource与结果版本逐项一致；无新增生成 | [绑定比较](reopen-verification.json)、[浏览器截图](reopened-image-and-video-failure.png) |
| Seedance Mini 视频闭环 | 未通过：网关误判processing，停止轮询；Platform释放冻结；尚无视频素材和播放结果 | [原任务最终核对](video-original-status.json) |
| 网关修复 | 代码和免费测试通过，尚未发布 | `6e517a322`，上列red/green |

08:13 UTC 最后一次只读查询供应原ID仍为 processing、无video_url/usage。网关与Platform quota均已释放不能等同于供应方免费或退款；供应侧最终账单仍需原ID对账。不要重试这条任务来获取结果，也不要手工伪造成功/归档/结算。

下一步：将上述小补丁应用到18420原运行源码，免费验证再发布；按原ID核对供应终态及费用。随后另行安排一次新的最小视频验收（不使用失败节点的重试），补齐成功视频的原件/海报/播放、画布回填和重开比对。真实Agent媒体审批尚未在本批覆盖。正式 Studio 域名尚未切换，生产 Console 保持不变。
