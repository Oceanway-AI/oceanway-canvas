# Studio A3 首批：引用保护修正

## 范围和版本

- 用户指令：按照计划进行修正。继续完整 OAC、原 Go/Pi/Task 和宿主 Platform 文件路径。
- 独立工作区沿用 `codex/studio-assets-a2-20261109`；基线 `ba92450`，未编辑原 A1 工程。
- Docs 计划 `200623f7`；RED `faa07bc`、`6ade364`、`ee83aaf`、`687db0c`；实现 `f12070b`。
- 规范：oceanway-docs 的 DEV-STUDIO-07-004、PDOC-STUDIO-03-004；全局事实：PROC-02-068 的 A3 节。
- 仅本机提交；无 push、部署、线上/云配置变化、收费 API、旧失败任务重试。SDK 仍是正式 `v0.1.0-rc.6`。

## 已修复

| 缺口 | 实现与证据 |
| --- | --- |
| `/create` 交接会话保存了结果但未被索引 | `internal/assets/references.go` 识别固定 `resultStorageKeys` 和本地 `resultUrls`；原快照事务真实保存引用索引。RED 原 count=0，修复后 count=1 |
| 旧快照曾漏建索引 | `repository/canvas_history.go` 对 Platform 文件从原快照正文补充精确检查，限定所属用户，不改正文；另一账号的同字符串不能保护该资源 |
| 生成输入仅在加密协议中，普通清理无法识别 | `app/studio_execution.go` 的 `studioPublicTaskInput` 将已校验的图片/视频/音频/mask ID 去重写入原 Task；普通媒体、Agent 媒体均复用，公开输入不含原件签名 URL 或供应协议正文 |
| 持久 Agent 使用被临时 lease 遗漏 | `repository/resource_reference_agent.go` 读取原控制状态、消息、事件、Pi JSONL、撤销历史；原 collector 识别 canonical/Pi 工具返回的结构化 JSON。自由文本不递归解析，外账号不纳入 |
| 旧本地清理可删 Platform Resource，却遗留绑定 | 候选查询排除；孤儿清理、资产强删、管理员、单资源和两条公告删除事务读取实际 provider 后拒绝，混合批次不部分提交；Resource、Asset、历史、审计及删除 job 一并保持 |
| 回收状态被误报为网关错误 | 按提供方真实 `state=trashed`，新采用返回409；已有 active binding 仍按固定版本 Access，换号或会话撤销不能借旧 binding 访问 |

原生永久删除继续禁用。保护引用不等于已经完成最后使用者退出后的 release。

## 验证

摘要及逐测试状态见 `validation.json` 和 `*.jsonl`。JSONL 保留动作与失败诊断，过滤成功用例的大量 GORM 初始化输出；完整本机输出在 `.local/cache/a3/`，摘要记录其 SHA256。测试之间有重叠，不将数字累加为独立用例数。

- 路由、引用收集、历史专项：35 顶层通过、27 子例通过、0 skip。
- 原生 app 审批/引用/删除/恢复 race：首轮35顶层通过、2失败；两个失败均因本工作区未安装 Pi 的 `undici`。按原 `package-lock.json` 执行 `npm ci --no-audit --no-fund` 后仅重跑这两个真实 Node/Pi 用例并通过。最终37顶层、32子例通过，0 skip；未改依赖版本或锁文件。
- 新增及相邻正式 handler race：8 顶层、7 子例通过、0 skip。
- `go vet ./internal/assets ./internal/repository ./internal/handler ./internal/app` 通过。
- `go build -o ../.local/cache/a3/studio-server ./cmd/server`、变更 Go 文件 gofmt、`git diff --check` 通过。未保留或提交构建产物；仓库原 `backend/server` 保持原内容。
- Docs `docs:check`、`docs:test` 和 registry 检查通过；既有4项超长文档提示不属于本批。

核心复跑命令（从 `backend/`）：

```sh
go test ./internal/assets ./internal/repository ./internal/handler -run 'TestCollectDocumentResourceReferences|TestStudio|TestCanvasHistory|TestCanvasSnapshot|TestSaveCanvas|TestResource' -count=1
go test -race ./internal/app -run 'TestStudio|TestCollectOwnedAssetDocumentReferences|TestResourceReferenceFields|TestDocumentReferencesResources|TestDeleteAsset|TestDeleteGeneratedAsset|TestPurgeConfirmedAsset|TestResourceDeletionWorker|TestExpiredArchivedAsset|TestDetached|TestResourceCleanupCandidates|TestCanvasHistory|TestAnnouncement' -count=1
go test -race ./internal/handler -run 'TestStudio(TransferredConversation|PreexistingHistory|NativeDeletion|DurableAgent|MediaTaskRetains|RetainedBinding|HistoryProtects|CanvasRemoval)' -count=1
```

使用独立 SQLite、假 Platform HTTP 与免费 Node/Pi 模型夹具。没有浏览器 UI 改动，本批未重新做浏览器操作；没有实际 TOS/CDN、生产 PostgreSQL/Redis、多进程或真实模型验收。未运行全仓推送前脚本和远端 CI，推送前仍须执行仓库门禁。

## 下一批及禁止误判的边界

1. **完整释放仍未接通。** 最后使用者检查必须涵盖旧 Task 的加密固定版本与尚未入队的授权意图。本批新公开输入只覆盖新建普通 Task、新冻结 Agent 媒体 Task；不改写、重试旧失败。继续做并发采用/保存与释放串行化、持久 release outbox、原 binding 响应丢失恢复和完整项目快照版本，验收后才开放永久删除。
2. **媒体元数据契约待提供方补齐。** 正式 rc.6 的 `AssetFile` 无宽高/时长；上传 meta 和原图片浏览器探测只是业务信息。按 DEV-STUDIO-07-004 的提供方要求补原件元数据及探测状态、正式 SDK，再接 `studioMaterialCall.adopt`；禁止用请求参数或缩略图尺寸充当原件信息。
3. **旧待授权意图与历史投影需盘点。** 当前所有 Platform Resource 均被本地删除防线保护，不因公开字段缺失而释放。只读索引扫描的规模性能尚未验收，后续用真实数据量验证再定索引方案。
4. 团队、批量/角色卡、性能及 C1–C5 剩余模型/故障矩阵仍未完成；A3首批修正不代表整体可发布。
5. 发布前把 A1/A2/本批整合到唯一 Studio 发布分支，按 AGENTS 执行推送前门禁，再在授权的独立测试环境确认前后端运行 SHA。当前本机改动没有自动改变 W7。
