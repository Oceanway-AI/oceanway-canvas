# 2026-11-13 本地媒体链路整合验证

- Task：PLAT-MEDIA-INTEGRATION-20261113；Owner：当前协调会话。
- Base：`00396bccd051fe4000e61dd5cb859aedb138f3ba`；整合代码：`13bc449`。
- 分支：`codex/studio-local-media-integration-20261113`。
- 范围：已完成的图片尺寸消费与统一模型选择、展示、健康信息。正式依赖保持 `platformclient/v0.2.0-rc.1`，无本地 replace。
- Owned Paths：本分支既有 `backend/`、`web/` 变更及本证据页；浏览器验收补充修复 `web/src/stores/use-config-store.ts`、`web/src/lib/user-session.ts` 及 `web/test/model-catalog-refresh.test.ts`。

## 验证

- `bun scripts/verify-before-push.mjs --jobs 2`：11 项通过，0 失败。包含前端 3328 个测试、487 个文件、77641 个断言，以及 Go 后端其余包与 app 四个分片、变更格式、类型检查、lint 和 gofmt。
- `bun run --cwd web build`、`go build ./cmd/server` 与 `GOWORK=off GOMAXPROCS=4 go vet ./...` 通过。
- 使用本机 PostgreSQL 16.14、UTF8、随机 loopback 端口及专属临时目录补验数据库用例：4 个包、25 个顶层测试，含子例 60 个 PASS；`-race` 开启，0 FAIL、0 SKIP。数据目录及进程已清理，源码摘要前后一致。
- PG 证据在本机 `.local/cache/verify/canvas-postgres-20261113/results.json`；其他门禁在 `.local/cache/verify/`，构建日志在 `.local/local-media-integration-20261113/`。

默认门禁没有运行无关的预演台浏览器 E2E；未安装的 `golangci-lint` 与付款插件 Docker 制品检查未执行，不能计为通过。本批没有新增付款插件改动。浏览器验收使用普通 Chrome，会在协调交付记录中登记本地实际运行版本。

用户已要求暂停线上部署。本批只进行本地提交、合并、编译和免费验证；未推送远端、未发布新 SDK、未发起付费生成。

## 浏览器发现与缓存修复

首次更新后，普通 Chrome 的目录接口返回 HTTP 200、业务成功和 16 个可用型号，但页面仍显示目录读取失败。通过同一 Chrome 会话捕获实际 `QuotaExceededError`：系统目录完整能力被写入用户作用域的 localStorage，实际持久化占 5,695,928 UTF-16 bytes。首次失败没有改记为通过。

系统目录现在只保留在会话内存；持久化只保存用户配置和最多五种模式的已选型号 ID。目录刷新期间撤除旧权限、价格与能力，待正式目录返回后核验恢复选择；用户主动改选优先，确认下架后清除选择，账号切换隔离保持。用户自定义渠道仍按原逻辑保存。

旧代码上先复现四项失败：配额超限、刷新丢失选择、失败重试丢失选择及账号重开丢失选择。修复后，9 个相关文件外层 37 PASS；其中目录专项由子进程执行 15 PASS，不与外层重复相加。类型检查、三文件 lint / format、独立生产 build 和差异检查通过；独立审阅没有确认阻塞。相同 16 型号实际响应的持久化缩减至 2,802 UTF-16 bytes，系统渠道不落盘。

新静态制品单独生成于 `.local/local-media-integration-20261113/catalog-cache-web`，2233 个文件、174521904 bytes，index SHA256 为 `e6aa49205d6a39c242d4d0e976fd9d1534ff1e8147d28c84517a984b0db55008`。原运行 `web/dist` 的 2233 个文件摘要完全不变。检查、源码与产物摘要见本机 `.local/local-media-integration-20261113/catalog-cache-evidence/results.json`。静态版本切换与真实浏览器复验由协调会话单独登记；本补丁不修改后端或 SDK 依赖。
