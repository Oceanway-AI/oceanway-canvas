# WP-STUDIO-020 生产发布与剩余验收

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

## 剩余交付

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
