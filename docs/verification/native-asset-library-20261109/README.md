# 原生资产库 A1 免费验收与交接（2026-11-09）

规范：oceanway-docs `PDOC-STUDIO-03-004`、`DEV-STUDIO-07-004`；过程记录 `PROC-02-068`。本目录为具名执行证据，不替代规范。

## 版本与状态

- 基线：`ee5a5df`，独立 `codex/studio-release-20261108`。
- 失败测试：`5653362`（正式路由）、`c0ab5c2`（浏览与采用）、`164ab6f`（同步短路）、`d0b11fc`（引用冲突、图片尺寸与恢复）。
- 实现：`855fbed`。A1 的本机实现与免费验收完成；A2/A3 未完成。
- 未推送、未部署、未切换 Console；未修改 Platform/SDK/TOS/CDN、真实数据或原失败任务。模型调用为 0。

## 修复与源码

| 根因或边界 | 实现 |
| --- | --- |
| `/assets` 被简化文件列表替换，正式 Studio 原资产路由 501 | `web/src/lib/workspace-route-modules.ts` 恢复原 `pages/assets/index.tsx`；`backend/internal/handler/user_data.go` 抽出并共享原资产/文件夹注册，`studio_api.go` 接入 |
| Platform 身份跳过原生资产读写，页面提示成功但未落库 | `web/src/services/user-data-sync.ts` 恢复显式 Asset 的原保存/读取链；画布 Resource-only 路径保持 |
| 新增资产丢失已选文件夹 | 原生表单注册 `folderId`；视频/音频编辑保留原文件，开放标题、分类、标签及备注 |
| 只浏览共享素材也创建采用绑定 | `studio_material_access.go` 与 `api/platform-materials.ts` 使用当前账号只读 Access；不创建 Resource/binding、不上传 |
| 重复采用覆盖编辑或写错资源 | `platform-material-selection.ts` 用固定 Resource 派生业务 ID，保留已有编辑，拒绝类型/引用冲突；缺图片尺寸时读授权原件测量，失败不写假尺寸 |
| 旧 OAC 清理会影响共享文件 | `studio_asset_library.go` 校验本人 ready Resource 与固定映射，归一稳定地址；旧永久删除返回 409，自动回收清理在 Platform 模式停用 |
| 卡片拉取大原件 | 原 `AssetMediaPreview`/资源缓存走 image thumbnail、video poster；详情沿原 preview/playback，下载取原件 |

未另建 Asset CRUD、任务执行器或素材存储。Studio 持有创作元数据，Platform 继续持有原件、版本、容量、权限和引用。

## 检查结果

| 检查 | 结果/证据 |
| --- | --- |
| 原始失败 | `backend-red.txt`、`frontend-red.txt`、`sync-red.txt`、`adoption-boundary-red.txt` 保留，不改称通过 |
| 前端专项 | `frontend-final.txt`：14 文件、50 PASS / 0 FAIL，含原资产、上传恢复、Resource-only 画布、同步及采用 |
| 正式 Studio handler | `studio-handler.txt`：`go test ./internal/handler -run '^TestStudio'` 通过 |
| 后端原资产专项 | `native-backend.txt`：app/canvas/repository 相关专项通过 |
| 新边界 race | `backend-race.txt`：3 顶层测试及3子例通过，0 skip；包括重新构建 service/router 后从同一 DB 读取。此项不是跨进程/真实 PG 重启证明 |
| 静态与构建 | `typecheck.txt`、`lint.txt`、`vet.txt`、`format-check.txt`、`build.txt` 通过。构建仍有大 chunk 与混合动态导入警告，未宣称性能收口 |

后端重跑命令：

```sh
cd backend
go test -race ./internal/handler -run '^(TestStudioNativeAssetLibrary|TestStudioMaterialBrowse)' -count=1 -v
go vet ./internal/handler ./internal/app
```

没有跑全仓测试、真实 PG/Redis 故障矩阵或 PR CI；本批未改变资金、任务和数据库 schema，不用本机专项冒充发布验收。

## 浏览器实际操作

Chrome 使用真实 Vite 页面与正式 Studio router/service/SDK；外部 Platform 和字节访问为免费夹具。独立 SQLite 内存库、独立端口 18285/18286，未接真实账号/桶/供应商。

1. 创建「A1 免费验收」文件夹。纯浏览图片后 assets/resources/references/bindings 均为 0，Access 为 1。
2. 采用公开免费图片及视频：2 Resource、2绑定。重复采用图片不增加文件/绑定，测得 640×427。
3. 新建「A1 云端说明」；编辑视频为「A1 视频参考已编辑」，道具分类、验收标签及备注；原生视频实际播放。浏览器媒体元素读取 960×540、5.055秒，业务元数据尚为未知，未伪造补齐。
4. 视频移入回收站并还原；保持 2 Resource、2引用、0删除任务，永久删除菜单不出现。
5. 真正关闭页面并新开 `/assets`，3条业务记录的 ID、标题、文件夹、状态及更新时间逐项相同；详情备注/标签保留，视频再次可加载。`browser-before-reopen.json` 与 `browser-after-reopen.json` 数据一致；详情及计数另存 JSON。

图片与视频来自 MDN 公共示例；夹具缩略图/封面也使用公开 JPEG，**不是本轮真实 TOS、CDN 派生或缓存命中证据**。本轮没有浏览器上传动作，上传原路径由免费协议/恢复测试覆盖，真实上传复验仍需后续环境。

最初用于复现同步短路的旧本地草稿未落库，留在测试浏览器缓存，导致侧栏全部计数 4、服务端资产 3；不算成功保存。本批新建文本已核对落库。此测试来源的残留未删除真实数据；迁移前应审计线上是否存在同类旧本地草稿，不自动清除。背景壳仍有未开放路由 501 与 Agent HMR abort 日志，未将本次资产通过扩大为全应用通过。

截图：`browser-reopened.png`、`browser-video-reopened.png`。测试页已关闭，18285/18286 已停止监听；夹具日志最终 PASS，不作为上线常驻服务。

## 下一步 A2

1. 修 `/create` 已有 ready Resource 无 Asset 投影导致转画布误报；复用同一资源，不重上传/重生成。
2. 统一原生画布/Agent 选择器与本资产库的业务记录消费；保留审批、任务和固定文件版本。
3. 补项目/历史引用确认与释放的使用者语义，再开放永久删除；不得用删除 Resource 代替释放一个使用关系。

音频浏览器验收、完整上传复验、团队、媒体元数据、角色卡、完整批量、性能与部署属于后续 A2/A3，不能把 A1 称为全部素材能力完成。
