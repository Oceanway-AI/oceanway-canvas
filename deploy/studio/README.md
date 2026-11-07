# 新 Studio 首发与双会话交接

更新：2026-11-08。应用分支 `codex/studio-platform-application`；发布分支 `codex/studio-release-20261108`。两者共享完整 OAC 实现，发布侧合并应用提交，不复制第二套业务代码。

## 分工与门槛

应用侧负责 SDK、身份、模型、素材、原任务调度与 Pi Agent，进度见 `docs/plans/oceanway-studio-application-progress.md`。发布侧负责本目录、容器、代理、环境及实际验收。先写部署失败测试，再改配置；只读查看另一方工作区，不重置未提交工作。

首发必须完成：真实登录/退出/换号；模型与消费预览；素材上传、读取、引用、删除保护；文本/图片/视频生成；Agent 原生审批与同链工具；结果回填、关页和服务重启恢复。其余创作源码全部保留，未接通功能明确暂不可用。健康检查、SDK 发布及免费夹具不能代替真实服务验收。

生产 Console 禁止切换、重启或改配置。新 Studio 测试入口采用 `https://test.oceanway.tech:18448`，Platform 仍为 `https://test.oceanway.tech:18447`。这两个端口使用相同受控主机与 W7 的 host-only Cookie；必须保留请求 Host 的端口。新 Studio 使用独立 PG、Redis、业务数据卷和 Agent token，不接旧 Studio 数据库。测试通过后再准备 `studio.oceanway.tech` 的正式切换；测试账号、素材桶与模型不得被描述为生产数据。

## 当前运行核查

| 服务 | 实际版本 | 当前事实 |
|---|---|---|
| 生产 Console | `cca3f516`，镜像 `0e068d30c7ea` | 旧契约；不得为本次发布切换 |
| W7 Platform | `23d0f402`，镜像 `2933ff284eb6` | 素材 v2 已有实际验收；59 个模型为禁用草稿，连接数为 0 |
| Media acceptance | `4aec533`，镜像 `55f96a9a2fcd` | 运行合同仍描述旧 video-only gateway；不能宣称 ADR-0032 统一供给已发布 |
| 新 Studio 应用 | `4e64c70` 首批 | 身份及空白画布；模型/素材/Agent 接入进行中 |

用户于本轮明确新 Media Gateway 尚未完成，确认临时复用 ow-main `45.58.184.2:18420`。它实际运行 `87e1f9f` / `v1.0.0-rc.48-ow-async.20261105.2`；已通过既有服务凭据只读取得 3 个 Seedance 型号、6 个 Gemini 图片型号。临时供应执行仍经 Platform，长期素材仍归 Platform，不把该旧网关记成 ADR-0032 已完成。

Platform 要求托管连接使用 HTTPS；测试 edge 的独立 18450 端口为已确认的 18420 网关提供 TLS 入口，只转发生成/查询 API，不开放管理、账号和支付路由，不保存供应凭据。上游仍按其原 Bearer Key 鉴权。此入口不承担任务或计费逻辑。

## 部署验收要求

- 复用根 `docker-compose.deploy.yml` 的 PostgreSQL、Redis、migrate、独立 Agent、backend、web 六服务，Studio overlay 注入完整服务端配置。
- 镜像绑定同一源码提交。禁止默认拉取未改造的上游 Agent，也不启用指向上游的在线更新器。产品凭据不进入前端构建参数、镜像层或 Git。
- 测试 TLS edge 只监听 18448；生产 Console、W7 18447、旧 Studio 容器和数据卷不复用或清理。
- HTTP、WebSocket 与 SSE 均保留真实 Host/Origin，Cookie 分别透传；签名和会话参数不写访问日志。
- 先记录受保护服务的容器 ID、镜像、启动时间和配置摘要，操作后对比。回退以新 Studio 的原镜像/配置和独立备份为单位，不恢复旧 Runtime。
- 先做免费真实文件与真实数据库验证，再在可用的正式模型合同下做有限次数真实生成；原失败任务只查回、不自动重发。

## 验证记录

本目录配置仍在开发；未部署新 Studio，未执行收费生成。失败与通过结果写入 `docs/verification/studio-release/`，明确区分配置测试、免费集成、真实服务和正式切换。
