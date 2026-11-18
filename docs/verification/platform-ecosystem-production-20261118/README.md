# WP-STUDIO-020 平台融合、生产发布与验收

## 最终验收

最终应用提交为 `4c1d5758b6ee6c2c4fde05d12d7d0d93890bb2f5`，自有 Release `v2.0.7`，Studio schema101，正式 SDK `platformclient v1.0.0-rc.5`。本机最终检查为「共8项，失败0项，用时355.2s（并行3）」。发行工作流44324670635的18个job全部通过，2026-11-18 14:41:12 UTC完成受管生产升级。WP-STUDIO-020 按 FS-STUDIO-001 v1.5／WP v1.7 本包范围完成，Docs PROC-02-094 v1.3记录对应结论。下方v2.0.2内容保留首次安装时点。

| 验收范围 | 实际结果与证据 |
| --- | --- |
| 源码退出 | 旧任务账务、分组定价、独立 OSS、User 查询、配置和翻译退出。97个运行模型，56个仅 reset 识别的退役表名，运行依赖无 SQLite。`final-retirement-inventory.json` 固定 f949450／schema100，schema101 增加绑定命令列 |
| 真实身份与权限 | 同名双账号、跨账号404、旧 actor／UUID写409、临时角色授予及撤销、Cookie刷新、禁用及退出会话通过，见 `real-identity-results.json`、`real-session-revocation-results.json` |
| 真实素材 | 上传、采用、重复采用、跨账号拒绝、原件摘要，以及清理后同 Resource 新绑定采用通过，见 `real-materials-results.json`、`real-readopt-results.json` |
| 真实免费生成与 Pi | 普通参考图成功；视频丢回执后 SIGKILL 重启，以原命令恢复成功；Pi 批准后完成图片及文本、拒绝和取消均终态。成功生成结算0，见 `real-generation-results.json` |
| 清理与恢复 | 缺 key 失败保持数据，恢复 key 后释放3条 Studio引用及1个实际持有 Pi委托，重新登录／画布重开通过。schema101 再次释放2条新命令引用后零画布／资源／引用／任务／执行，两个自建测试账号禁用，见 `real-reset-results.json`、`real-post-reset-results.json`、`real-final-reset-results.json`、`real-test-cleanup-results.json` |
| 提供方对象 | Platform原件、生成历史绑定和内部授权保留，物理删除0，见 `real-platform-retention.json`。2条普通生成使用平台内部授权，不能计为Studio持有委托 |
| 配置 | 7项必需配置缺失及显式SQLite共8类均启动失败，见 `real-startup-config-results.json` |
| 平台吸收审查 | 固定旧 Studio 与 Platform 源码，已有覆盖和五项评估建议见 `retired-capabilities-audit.md`，五项均为尚待Platform实施的建议 |

真实测试 Platform 为 `a447af59b4427dfb3e213e897ab8c92fd622e821`，tree `8e26e74e5af6012170959298232e3b52d6b6f2d5`，镜像 `sha256:8d93f496b823c75b45e5b36fc6d00c071b812236ec0f858918cc22d1f2bb577d`。模型为实际 Platform／Media Gateway 中发布的零价格夹具，付费生成提交为0。普通文本探针因planner夹具要求工具返回422，双图纯生成探针因供给不可用失败，原失败保留。普通文本、多输出和其他创作链由本地回归覆盖，真实Pi文本和单张参考图成功另行记录。

首笔视频因测试 Gateway poller 停止而失败并释放资金。恢复既有 poller、修正素材源精确出口白名单后，以新用例验证原请求恢复成功。测试身份配置补入 studio:admin 定义，临时授权已撤销。这些测试配置修复已记录，Platform／Gateway源码沿原版本。

联验发现并修复两个产品缺陷。v2.0.6 `8bc71a3` 按匹配的终态 Pi执行排除 text_replay 清理阻塞，8场景先失败后通过。v2.0.7 `4c1d575` 在引用创建前持久化新 binding_command_id，支持丢回执、并发赢家、历史 released 绑定续接及迟到回执拒绝。schema101 保留历史命令回退，清理读取当前命令。RED／GREEN见 `reset-replay-*.txt`、`readopt-*.txt`，race 的 handler／database／app 均通过。

v2.0.3 本地13项0失败457.0秒，v2.0.6 本地8项0失败351.7秒，v2.0.7 本地8项0失败355.2秒。v2.0.7差异无web，默认检查按范围跳过Web；本机缺少golangci-lint，发行CI补齐全量Web、预演台E2E、Agent静态检查与后端PostgreSQL／Redis。

原生根路径与子路径完整夹具见 `../platform-ecosystem-20261118/`，含采用、账号切换、画布重开、窄屏深色与偏好。此次同源 `/studio/` Web构建和真实页面路径通过，后端返回登记的HTTPS回跳。云端子路径登录未在本批重跑。本机HTTP Origin不匹配登记HTTPS时明确拒绝。预构建Release Web使用根路径，子路径需同源Web构建参数 `VITE_APP_BASE_PATH=/studio/` 与后端基础路径匹配。

自有更新器在v2.0.4修复GHCR owner大小写，v2.0.5修复systemd ProtectHome下的Docker凭据目录，两次失败发生于停止服务前。生产完成v2.0.2→v2.0.3→v2.0.5→v2.0.7受管升级。独立回退演练恢复v2.0.2镜像、schema98、3画布、2引用、0任务及配套 `.settings-key` 后核验通过，演练服务已停止。最终运行、3份配套备份摘要与回退结果见 `production-v207.json`，升级操作见 `updater-v207.json`。

首轮发行44320684824在Agent arm64 npm ci时出现qemu illegal instruction并挂起，质量检查已通过，约15分钟后取消。单job重试未进入执行，重新派发同一源码的44324670635后全部成功，首轮记录见 `ci-v207-attempt1.json`。正式Release发布于14:37:24 UTC，两个架构更新器与Compose附件完成下载校验，见 `release-v207.json`、`release-v207-verified-assets.json` 和 `ci-v207.json`。

生产五个常驻服务健康，migrate退出0，backend／web／yingce-agent／migrate revision均为最终4c1d575，schema101的binding_command_id存在，56个退役表均不存在，Host Updater二进制摘要与Release一致。升级前v2.0.5备份14437788字节，SHA256 `b13d3a8997d567692d741158abb104f367297cbb4749da4d92e8b178875f3cb9`。

升级后的浏览器当前会话属于另一Platform主体，旧验收画布返回归属拒绝。沿当前会话新建 `Y2FsaPlM0WH4HfymqEqgk`「WP-STUDIO-020 v2.0.7 生产验收」，原生上传799字节PNG和2291字节MP4，并生成1212字节JPEG预览图。图片320×240加载完成，视频320×240、1秒完整播放，readyState4且无媒体错误。关页重开后2节点、0连线、已同步，三个新引用的绑定命令持久化、owner一致且Resource均ready。生产总计4画布、5Resource、5引用、0Task、0StudioExecution。新验收画布及小型素材保留，模型生成提交0。见 `browser-v207.json`、`canvas-v207-reopened.jpg` 和 `production-v207.json`。截图裁去账号区域。

WP-STUDIO-018 的团队协作、团队生成和购买返回，以及资产A3的完整永久删除／团队／元数据仍由对应工作包交付。本目录覆盖 WP-STUDIO-020 的直接身份与旧平台能力退出。

## v2.0.2 首次安装历史

2026-11-18，按用户直接部署生产 Studio 的授权，发布自有 Release `v2.0.2` 并在 `studio.oceanway.tech` 运行。源码为 `8d68e13422a445c73b1734ab99da6e0a81230978`，SDK 为 `platformclient v1.0.0-rc.5`，schema 为 98。工作包保持 doing，规范与状态沿 Docs 的 FS-STUDIO-001、WP-STUDIO-020、PROC-02-094 维护。

## 发布和安装

自有仓库 `Oceanway-AI/oceanway-canvas` 的工作流 `44265051566` 全部成功，生成前后端及原生 Agent 镜像、Compose、amd64／arm64 Host Updater 和 SHA256SUMS。Release 于 `2026-11-18T08:21:21Z` 发布，版本及附件摘要见 `release.json`，工作流各 job 见 `ci.json`。

生产通过原安装器和 Release Compose 安装，独立 PostgreSQL 17、Redis 7.4 与原生 yingce-agent 均运行。backend、web、agent、postgres、redis 五项健康，migrate 退出码为 0，Host Updater 为 active。web 绑定宿主机 loopback，由 Studio HTTPS nginx 转发。生产使用新建的专属数据库，初始化前创作数据为零。私有 GHCR 镜像使用已有 GitHub 凭据完成主机拉取配置，凭据留在受限主机配置中。

运行镜像摘要、版本、schema、匿名 401、登录返回、表清单和发布前 PostgreSQL 备份摘要见 `installation.json`。备份已通过 `pg_restore --list`。生产数据库没有 users、user_identities、billing_orders、credit_transactions、payment_orders、user_groups、user_group_channels、user_oss_settings、model_channels。

安装器连接曾在尾部中断，重连检查确认安装日志已结束、全部服务健康且 updater 激活。生产 HTTPS 从原 410 切换到 v2.0.2，公开版本接口与发行提交一致。

## 修复与质量检查

v2.0.1 的远端质量检查暴露三项问题，v2.0.2 已修复并复验。

- CI 的依赖用例名称检查需要 backend/internal 源码，补入该 job 的 sparse checkout。
- Agent 记忆提案测试保存后使用未经过 PostgreSQL 精度转换的时间戳，夹具改为重新读取已保存记录。
- Agent 控制行与 journal／transcript 在 PostgreSQL READ COMMITTED 下可能来自不同快照。三个独立读取入口改用只读 REPEATABLE READ。确定性交错用例先验证失败，再验证修复通过。

最终干净源码本地检查为「共 12 项，失败 0 项，用时 472.5s（并行 3）」，原始输出见 `quality-summary.txt`。Agent／memory race 专项通过。最终发行 CI 的全部质量和制品 job 通过，补齐本机未覆盖的 golangci-lint 与预演台 E2E。

## 真实 Platform 浏览器验收

实际 Platform 运行 revision `1614a0f8392a7cfa66b9a178f41c17b54b47e800`，source tree `6688beb8997536872c61e95f5dbe148f1053fb1b`。浏览器沿现有 Platform 会话直接进入 Studio 个人空间。

通过真实页面创建并命名画布 `DI0iM5RR7FZpTAv8N8AAE`，标题为「WP-STUDIO-020 生产验收 20261118」。从 Platform 素材库引用已有 `free-blue.mp4` 和 `free-green.png`，视频完整播放 1 秒、320×240、readyState 4 且无媒体错误，图片自然尺寸 320×240 且加载完成。刷新后名称、两节点、零连线和已同步状态恢复，截图见 `canvas-reopened.jpg`。

数据库结果见 `browser-database.json`。画布 owner 为规范十进制 ID，两条素材引用 owner 与画布一致，两个 Resource 和绑定保持固定 ID。当前路径写入画布素材引用，业务 Asset 表为零。本轮 Task／StudioExecution／物理删除任务均为零。新增的验收画布和两个素材引用保留在个人空间。

## v2.0.2 发布时剩余交付

| 范围 | 剩余事项 | 对应验收 |
| --- | --- | --- |
| E3 旧任务账务 | finance、task_billing、creation／lifecycle／terminal／worker、provider 取消与恢复及 repository/finance_billing 的本地结算依赖 | SF-A08／09／12 |
| E3 渠道与分组定价 | user_groups、model_router、logical_model_quote、catalog 及 workbench／drama 等创作调用方的渠道、倍率和报价依赖 | SF-A08／09 |
| E3 OSS | 管理员／个人 OSS 设置、storage_location、resource_oss 和旧存储适配，逐项保留仍用于创作的本地系统素材与执行加密 | SF-A08／09／13 |
| E0／E3 残留依赖 | DomainModels 中旧 User／BillingOrder／UserGroup／UserOSSSetting，service aliases、未使用的 AdminAgentLessons 用户表查询、旧配置和翻译残留，最终调用／表／配置清单 | SF-A01／03／15 |
| E1／E4／E5 联验 | 真实双账号与失权／SID 轮换、迟到响应、旧身份清理引用和委托失败重入，普通生成、Pi 审批、取消、原回执丢失与在途进程重启 | SF-A02／06／11／12／13 |
| E6 升级与回退 | 自有 Release 受管升级、兼容回退、真实子路径和缺配置失败验证 | SF-A14／15 |

本轮验证覆盖首次安装、真实登录及已有云素材引用恢复。旧部署实例的数据清理尚未执行，生产采用新建数据库。在途生成、收费调用、服务重启恢复、双账号和 Console 购买返回继续待验。

WP-STUDIO-018 的 ST2／ST3 团队协作与团队生成素材、ST4／ST5 购买返回和完整样板联验，以及资产 A3 的完整释放／永久删除／元数据／团队能力，继续按各自工作包交付。
