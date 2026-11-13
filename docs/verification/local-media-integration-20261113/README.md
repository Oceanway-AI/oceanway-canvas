# 2026-11-13 本地媒体链路整合验证

- Task：PLAT-MEDIA-INTEGRATION-20261113；Owner：当前协调会话。
- Base：`00396bccd051fe4000e61dd5cb859aedb138f3ba`；整合代码：`13bc449`。
- 分支：`codex/studio-local-media-integration-20261113`。
- 范围：已完成的图片尺寸消费与统一模型选择、展示、健康信息。正式依赖保持 `platformclient/v0.2.0-rc.1`，无本地 replace。
- Owned Paths：本分支既有 `backend/`、`web/` 变更及本证据页；本轮整合无新增业务代码修复。

## 验证

- `bun scripts/verify-before-push.mjs --jobs 2`：11 项通过，0 失败。包含前端 3328 个测试、487 个文件、77641 个断言，以及 Go 后端其余包与 app 四个分片、变更格式、类型检查、lint 和 gofmt。
- `bun run --cwd web build`、`go build ./cmd/server` 与 `GOWORK=off GOMAXPROCS=4 go vet ./...` 通过。
- 使用本机 PostgreSQL 16.14、UTF8、随机 loopback 端口及专属临时目录补验数据库用例：4 个包、25 个顶层测试，含子例 60 个 PASS；`-race` 开启，0 FAIL、0 SKIP。数据目录及进程已清理，源码摘要前后一致。
- PG 证据在本机 `.local/cache/verify/canvas-postgres-20261113/results.json`；其他门禁在 `.local/cache/verify/`，构建日志在 `.local/local-media-integration-20261113/`。

默认门禁没有运行无关的预演台浏览器 E2E；未安装的 `golangci-lint` 与付款插件 Docker 制品检查未执行，不能计为通过。本批没有新增付款插件改动。浏览器验收使用普通 Chrome，会在协调交付记录中登记本地实际运行版本。

用户已要求暂停线上部署。本批只进行本地提交、合并、编译和免费验证；未推送远端、未发布新 SDK、未发起付费生成。
