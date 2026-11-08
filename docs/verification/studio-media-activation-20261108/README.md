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
