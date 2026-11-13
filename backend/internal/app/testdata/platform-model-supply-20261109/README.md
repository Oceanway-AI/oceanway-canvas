# Platform → Studio 冻结参数样例

来源：Platform 会话 `01a1bbad-1640-7ac3-8c82-ae59a56fae48`，`codex/admin-model-settings`，读取时 HEAD `7401e5cfd28e46cf52346552a23b25967276d83f`，含该会话尚未提交的模型配置改动。不能把这个 HEAD 单独当作样例生成实现版本。

交接补充：提供方最终候选已提交为 `b20e4715b999df4ee1cedb9704f3f4f33e650347`。本目录仍保留原始冻结文件，未在该提交上重新导出；不可改称最终提交的生成证据。接续以 `docs/verification/model-supply-joint-20261109/README.md` 的版本表与复验要求为准。

提供方 `TestPlatformFrozenModelHandoffPostgres` 在独立 PostgreSQL 中通过真实管理 HTTP 发布合成配置，再导出公开目录和消费预览；两个样例 `generation_requests=0`。不是生产导出，也不是供应商能力或实际生成证明。提供方原始导出位于 `.local/studio-model-handoff-20261109/`，最终来源交接由该会话记录。

| 文件 | SHA256 | 预览事实 |
| --- | --- | --- |
| image-published.json | 3a7c041e65c099d45d881a3e47478546746e62e947a50cbae3b642ec33fa067b | HTTP 200、allowed=true，但 combination_supported=false / output_mapping_unverified；必须拒绝创建 |
| video-published.json | d89fc68646c3c48d581468a0564d3087d4b7ad857a86d4a196bfabfb26f9b053 | ready、allowed=true、combination_supported=true；仅表示合成配置预览获准 |
| studio-projection.json | fab641a9ec3ea01256bb21ff447be65a11c1b3701e4b6e7fbeea670993e6cafb | 上述模型经 Studio 正式 SDK Models 与目录投影生成，供前端同源测试读取 |

`TestStudioPlatformPublishedFixtureConsumption` 校验 SDK 目录、原生正文与提供方预览的对应关系、三版本、拒绝回执和互斥变体。`web/test/platform-model-supply-fixture.test.tsx` 直接读取 Go 投影，验证原控件、任务参数和丢回执后只查同 command。更新投影前必须核查合同差异，再运行：

```sh
cd backend
STUDIO_UPDATE_MODEL_FIXTURE=1 go test ./internal/app -run '^TestStudioPlatformPublishedFixtureConsumption$' -count=1
```

不带环境变量时测试只比较文件，不更新。不得将拒绝回执改成成功以使测试通过；新增 ready 样例另存并记录来源。
