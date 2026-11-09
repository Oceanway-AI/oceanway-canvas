# Studio 原生资产 A2 本机验收与交接

## 版本与范围

- 独立分支 `codex/studio-assets-a2-20261109`，起点 A1 `1094b50`；未编辑原 A1 工作区。
- 文档先行：oceanway-docs `d814895e`，规范 `DEV-STUDIO-07-004`／`PDOC-STUDIO-03-004`；全局交接 `PROC-02-068` 的 A2 节。
- 失败测试 `b9a0da3`（三项结果交接失败和历史错引用），实现 `cb0a95c`。
- 仅本机源码和免费验收；未推送、未部署、未改 Platform/SDK/CDN/线上 Console，未生成模型任务或重试旧失败。

## 修复

1. `/create` 的单个结果卡片和「画布中继续」先复核本人 ready Resource，再复用 A1 `saveAdoptedLibraryAsset`。ID 固定为 `platform-<resourceId>`，已有标题/标签/文件夹保持；保存响应丢失或部分完成后继续同一记录。过期 URL 不能代替文件身份，换号及类型不符中止。原生成 worker 的 Resource-only 路径保持。
2. 恢复原 `AssetPickerModal`、`CanvasWorkspaceAssetPanel`；Canvas 和 Agent 使用同一原生 Asset 分页/分类/转换。卡片走 thumbnail/poster，未接通外部源和角色认证不开放。共享文件采用入口保留在资产页。
3. 图像交接保留原宽高、大小、MIME；固定结果按精确 storageKey 匹配，不把其他任务输出或空 URL 当作目标。转入画布的会话历史保留 resultStorageKeys，不复制临时签名地址。
4. Platform Resource 的旧 OSS 坐标为空，原历史查询会误把不同文件视为别名；现按 Resource ID 精确查询。保留真实对象存储的旧别名规则。沿原历史事务验证移除、恢复、跨账号拒绝、另一画布及原 Task 保持，未引入引用执行器。

## 验证

| 检查 | 结果与证据 |
| --- | --- |
| RED | `handoff-red.txt` 3失败；`history-red.txt` 精确文件保护失败；保留原记录 |
| 前端 | `web-green.txt`：12文件67测试通过，0fail；含单独进程真实React按钮点击、采用恢复、原上传/结果/任务恢复。子夹具不重复加总 |
| 后端 | `handler-race.txt`：10顶层＋3子例通过、0skip，正式Studio路由＋独立SQLite；`original-history.txt`：2原生历史/删除保护测试通过，保留旧对象别名行为 |
| 静态/构建 | TypeScript、ESLint、Go vet、变更Prettier/gofmt、diff检查及Vite构建通过；构建仍有既有动态导入/大chunk警告 |
| 浏览器 | 正式Studio router/service/SDK＋真实OAC页面，隔离SQLite和假Platform；免费公共JPEG/MP4，不是实际TOS/CDN或生产授权验收 |
| 关页重开 | `browser-identity.json`前后逐项一致：2Asset、2Resource、2active binding、3节点、0连线、原会话/输出引用、0删除job；没有额外上传或绑定 |

浏览器顺序：准备已有结果（只有2 Resource、0业务Asset）→单图片卡片转画布（1Asset/1节点）→整会话继续（2Asset/2节点且图片不重复）→原素材侧栏→Agent原选择器选择图片（原生行为新增1节点并出现附件）→免费视频播放→关页新开。视频实际解码960×540、5.055秒；不以夹具元数据代表生产元数据已补齐。截图为 `browser-native-assets.png` 和 `browser-reopened.png`。未发送Agent消息，不宣称本批重新验收Pi模型或审批。

开发模式出现已有Agent头像 StrictMode AbortError；未影响上述操作，不称整站无控制台错误。浏览器取证后手动fixture达到Go默认10分钟超时退出；不是以该常驻进程的测试退出码作为通过证据。前端/后端专项各自独立通过，复跑常驻fixture使用下列 `-timeout 0`。临时端口18287/18288及验收标签页已关闭。

## 复验入口

```sh
# backend：只有免费fixture，不能用于真实登录或外部暴露
STUDIO_ASSET_FIXTURE_ADDR=127.0.0.1:18287 STUDIO_ASSET_FIXTURE_HISTORY=1 go test -timeout 0 -tags studio_asset_browser ./internal/handler -run '^TestStudioAssetBrowserFixture$' -count=1 -v
# web：另一终端
VITE_API_PROXY_TARGET=http://127.0.0.1:18287 bun run dev --host 127.0.0.1 --port 18288 --strictPort
```

打开 `/test/fixtures/studio-assets-a2.html`，点击准备后跳入真实 `/create`。该页面只通过现有浏览器会话存储API写入测试历史，不替代产品UI，也不进入正式构建入口。`/__fixture/state`仅build-tag测试服务可读，用于核对固定身份。完成后关闭本批进程；不接线上DB。

## 接续 A3

- 先核对 Resource 元数据在正式SDK/产物中的字段，再补宽高/时长；不能以播放器能播放等同业务元数据完整。
- 团队、批量/角色卡、音频浏览器、项目快照/完整confirm-release和最后使用者释放仍需独立失败测试及验收。永久删除与物理清理继续关闭。
- 性能、真实云、应用发布另验；本批不替代C1—C5剩余矩阵。需要推送时先按AGENTS执行最新分支整合及 `bun scripts/verify-before-push.mjs`，本轮未执行全仓推送前检查或远端CI。
