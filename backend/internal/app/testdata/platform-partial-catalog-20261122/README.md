# Studio 产品详情与原生请求准备

`WP-MEDIA-CAP-001` / `DEV-X-01-019`，2026-11-22。

这些 gzip 文件解压后是隔离 Platform 通过 `GET /api/platform/v1/products/studio/model-catalog/detail` 返回的原始 HTTP 200 正文。Platform 的 `TestPlatformPartialGatewayPersonalDetailHTTPPostgres` 用测试产品凭据和一次性用户会话，在 PostgreSQL 中读取已批准的价格和有效供给。Gateway 冻结发行号为 `e136dfd8-0487-40d0-8cd4-fc801c06ff0a`，完整发行正文 SHA-256 为 `01d02a41d20f049bfb63ff6f5b379260721494a588d700fb17b1fd7a43e3ab94`。

模型 `aggregate-image` 的 `openai-image-generate` 变体有两个供给，均无参考槽。第一档支持 `1024x1024`、`medium`、`n=1`，第二档支持 `1536x1024`、`high`、`n=2`。Gateway 分别报告某一档仍可用时，Platform 的 `partial-0` 和 `partial-1` 回执只保留对应供给。`all` 回执包含两档。

`before/` 是初次实测保留的两份回执。初版 `partial-1` 已发布正确的第二档宽高，推荐尺寸和唯一预设仍为 `1024x1024`。修复前的 Studio 运行了 14 个组合，显式选择 `1536x1024` 成功，空草稿因自动填入旧默认值而失败。新版 Studio 从当前供给固定宽高补全必填尺寸，并保留显式冲突供用户修改。修订后的 Platform 将第二档推荐尺寸、预设改为 `1536x1024`，能力版本从 `fdd4d189ae7d1fdb92e3dbb010a99a2ecb0ed949c559069fefb23f6e0ddf2881` 改为 `e097ce153d2e5305b2d43862ef4452589214ede73f030c001950e763b6355806`。在当前价格和连接版本下，Studio 会以 HTTP 409 `catalog_revision_conflict` 拒绝使用旧能力版本的选择。

| 文件 | 原始 HTTP 正文 SHA-256 |
| --- | --- |
| `before/studio-detail-partial-0.json.gz` | `c71065bf2e0c110bc05114d3a827e9474f2ca11ce529a79f9257946d2b6ac71a` |
| `before/studio-detail-partial-1.json.gz` | `31ee09db30801c9c9b0947f57c0e468d1ead9cf55d57578f0ef9655b8b044987` |
| `studio-detail-partial-0.json.gz` | `55fe7717ba40342789878ebeae9de3a414f1f273ba9352ed918d448555d3065e` |
| `studio-detail-partial-1.json.gz` | `ca29707b2838f490cfd53edfad5f74c008408939cf7dc60fd2bef53554b24adf` |
| `studio-detail-all.json.gz` | `922749f2cb05032e0aa7e8aa532e3512e2f58263d62d1f37dd12a11ad16be92b` |

Studio 在 macOS arm64、Go 1.26.3 运行 `-race`。依赖取自 `backend/go.mod`、`backend/go.sum` 的已发布 `platformclient v1.0.0-rc.6`，来源提交 `10778ca37f9f3354de6a988068b6271cfc16a788`。以下测试用本地 HTTP 重放产品正文，经 SDK `ModelDetail`、宿主 `StudioModelCatalogDetail` 和生成请求准备函数，核对版本、完整原生规格、逐项保留的用户参数和最终正文。五份原始响应覆盖 44 种请求输入，包括空草稿、只选一项、交叉冲突及明确带入参考图。另有两个派生用例把公共和原生字段均标为必填，验证选择供给后补尺寸的行为。独立版本冲突用例固定旧能力版本和当前价格、连接版本。回放在生成正文后结束，费用为零。

```sh
cd backend
go test -race ./internal/app -run '^TestStudio(PartialSupplyProductDetail|CompleteSupplyProductDetail|RequiredImageSize|RevisedProjection)' -count=1 -v
```
