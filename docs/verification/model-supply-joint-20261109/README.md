# Platform / Studio 模型参数联合验收

用户要求与会话 `01a1bbad-1640-7ac3-8c82-ae59a56fae48` 联合调整供给及可用参数。分工已实际通知并得到对方接续：Platform 维护模型设置与能力发布；本会话维护 Studio 公开目录投影、完整 OAC 控件与消费。没有编辑对方工作区。

## 最终接收状态（2026-11-09）

提供方会话已完成，模型设置候选已本地提交 `b20e4715b999df4ee1cedb9704f3f4f33e650347`。本次核对Git提交、其已提交验收JSON、冻结样例摘要和会话最终状态，只整理交接，不重复开发、构建或发布。下述版本足以接续开发；尚未通过最终双仓版本的完整发布验收。

| 接收对象 | 固定入口与版本 | 实际核验状态 |
| --- | --- | --- |
| Platform | `codex/admin-model-settings`，`b20e4715b999df4ee1cedb9704f3f4f33e650347`，父提交 `7401e5cf` | 对方固定源码验收：207前端、28顶层后端专项及type/lint/build通过；本次核对提交代码树与其验收树仅差新增验收JSON |
| Studio | `codex/studio-assets-a2-20261109`，功能 `f210dfa`，完整上一批交付 `ae7caa9` | 已跑99前端/431断言、24顶层Go race＋40子例；本次只补交接记录 |
| SDK | `platformclient v0.1.0-rc.6` | 不换SDK，无replace；OpenAPI 0.25.0是提供方候选，非新SDK已发行 |
| 规范与事实 | Docs分支 `codex/studio-assets-a2-docs-20261109`，计划 `f1fa61d0`，首批事实 `40bdf78e`，本次续记PROC-02-067 | 另一Docs工作区中的PLAT-02-060、DEV-X-01-012、PROC-02-069仍未提交，待按范围整合，不能整体覆盖本分支 |

Platform提供方验收原文已从该提交复制为 `producer-candidate-validation.json`；发布样例历史来源仍是原 `producer-manifest.json`，不改写成最终提交重新导出的证据。验收代码树 `657208b8e57ba22d846f9cef0410511cb9a772dc` 与最终提交树 `da05e971996cdc613dd1eec769a381f8f5f36eab` 只差 `docs/verification/model-settings-candidate-20261109.json`。冻结图片/视频SHA未变化。抽查发布消费相关8个源码文件，7个与样例生成时一致，`service/platform_media_configuration.go`有差异；因此仍需最终提交上的样例复验，不能将历史联验改标为最终版本联验。

Platform共享工作树还有用户/团队、页面和视频seconds计费等其他会话修改；18447/18129等运行入口未在本次复核，不代表上述SHA。两边均未推送、合并main或部署本批，PR CI未跑；生产Console和旧失败任务保持。

## 接手顺序与责任

1. **集成执行者先冻结双仓版本。** 以表中SHA建立独立检出，不在Platform共享dirty工作树切分支、reset、整体提交或构建发布。Studio继续本工作区前先检查status。本地仓库入口为Platform `admin-team-provisioning/oceanway-platform` 与Studio `studio-assets-a2/studio-release-20261108`（均在Codex worktrees目录）；不要通过当前运行实例反推版本。当前只是交接，不执行合并或发布。
2. **Platform提供方补最终提交契约证据。** 在 `b20e4715` 独立检出，使用自建自清理PG重跑下面能力发布/预览门禁，并重新导出正反样例。旧图片 `output_mapping_unverified` 文件保留；图片ready样例另存。比较schema/defaults/bindings/references/独立变体语义，三版本须各自对应预览；时间戳、重新发布版本变化应如实记录，不强求新文件SHA等于历史样例。正式迁移0046增加渠道series/type，已有未分类渠道须明确分类，不能自动假定已就绪。
3. **Studio消费者先补尺寸，再补参考和变体。** 使用相同提供方样例，按下文未完成清单依次写文档和失败测试，再接原生控件与Task/Pi链。尺寸预设、复杂限制、首尾帧和默认变体之外的选择不能仅靠后端拒绝算完成。
4. **发布执行者最后接收。** 两边候选整合后重跑受影响门禁与仓库推送前检查，确认CI及同一制品运行版本，再按具体发布授权处理测试环境与真实调用。当前交接不表示上线就绪；既有部署、Console、在库数据和失败Task不得被本轮文档操作改变。

提供方命令（固定检出根目录；脚本自管临时PG，不设置共享DSN）：

```sh
python3 scripts/platform-commercial-test.py --filter '^TestPlatform(ModelSeries|MediaDocumentation|MediaConfiguration|ManualCapabilityHTTPPostgres|CapabilityRevisionHTTPPostgres|FormalContractRoutes)' --packages service controller router
GOWORK=off go test ./service -run '^TestPlatform(MediaConfiguration|MediaDocumentation|ManualCapabilityExact)' -count=1
```

这两条覆盖联合参数合同，不冒充提供方28项全部重跑；原价格投影另有独占PG夹具，详见候选验收JSON。Studio复验命令和golden入口见本文末尾。范围仍为本机免费开发，不从此前真实调用授权推导本次应发起模型请求。

## 提交与状态

| 层 | 本批事实 | 版本 |
| --- | --- | --- |
| Docs 规范先行 | PLAT-04-029 §0、PLAT-02-059 C1/C4 联合工作顺序 | `f1fa61d0` |
| Studio 文档 / 失败测试 | 原执行合同增量；Go 两顶层失败、前端三项失败先复现 | `13ab270` / `2b25b89` |
| Studio 修复 | 范围、音频别名、原生控件和跨仓样例消费 | `f210dfa` |
| Platform 候选 | 模型设置已本地提交；原发布样例保留历史来源，最终候选验证另列 | `b20e4715`；历史来源见 `producer-manifest.json` |
| SDK / 发布 | 正式 rc.6，无 replace；上述 Studio 改动未推送、部署或调用供应商 | `v0.1.0-rc.6` |

## 修复内容及证据

- `studio_catalog.go` 保留 `multipleOf` 的零起点语义。图片参数、视频时长选项、创建页、工作台和原画布设置面板共用同一规则；原生非 Platform step 保持以最小值为起点。合法小数得以保留，不再将 6 秒静默改成 7 秒。
- 选定 schema 的 `audio` 与 `generate_audio` 投影到原音频控件，默认值和原生绑定一致。Platform 模型走已发布控件，避免进入按型号猜测的 Seedance 专用分支或暴露供应商私有素材上传。
- 提供方在独立 PG 经正式管理 HTTP 发布两个合成样例，公开目录投影由实际 service 读取，消费预览经正式产品 HTTP。Studio 读取其冻结文件，以正式 SDK Models 和真实目录/原生 body 函数消费；再将 Go 投影 golden 直接交给 TS 原控件、请求准备和提交/恢复函数。
- 视频默认变体只提供 720p、6/9/12/15 秒，第二变体的1080p/8秒不能混入；显式选择第二变体的服务端参数校验通过。UI 非默认变体入口仍待接。
- 图片样例 HTTP200，但 `combination_supported=false / output_mapping_unverified`。前后端均验证拒绝，0任务创建；不将 `allowed=true`、显示参数或 SDK 成功解包当作可调用。

冻结样例、SHA和生成方式见 `backend/internal/app/testdata/platform-model-supply-20261109/README.md`。提供方原交接为其 `.local/studio-model-handoff-20261109/HANDOFF.md`；发布源码集合摘要 `4113986a07f87f1837c29626f2e0365741f28fe27319ef70ed09cc2d94587cf1`。文档编号冲突已由对方把未发布记录改为 PROC-02-069，本方既有067/068保留，双方 Docs 分支尚未合并。

## 免费验证

`validation.json` 列出精确用例与日志摘要。Go race：24顶层＋40子例、3包全部通过，含原 Node/Pi 媒体审批、拒绝/版本变化、同请求恢复和固定产物回填。前端99项/431断言通过，含真实 React 组件 SSR/DOM 夹具、目录/原生控件/普通提交、审批、结果与恢复；不称浏览器真实联调。typecheck/lint/Web build、go vet/server build、gofmt/Prettier及Docs 8测试/475页面检查通过。

扩大前端回归首轮97通过/2失败：原视频测试尚未模拟新增的消费上限确认。仅补明确确认和“确认前零创建”断言；产品预算门禁保持。原失败日志摘要保留，最终99项通过。Web build为既有大chunk与动态导入提示；Docs为4个既有长文警告。未跑完整CI、实际PG/Redis或云素材，本批无付费/部署/旧失败重试。

## 下一批联合工作（未完成）

1. Platform 候选提交已完成；下一步在固定提交上复验发布样例，保留当前图片拒绝样例，另补 ready 图片样例及其输出映射证据，禁止把当前回执直接改为成功。
2. Studio 接通 `size_presets` 和 `x-size-constraints` 的预设/自定义尺寸校验、总输出数量与字段别名。当前复杂尺寸限制只由 Platform 最终门禁兜住，不能声称界面全支持。
3. Studio 补参考 `requires_fields/excludes_fields`、`max_reference_items`、`required_any_of` 及首尾帧单值 url 传输。每项先取得提供方正反样例，再补红灯和原 Task/Pi 免费回归。
4. 明确非默认变体的原生选择入口，并验证 Agent 的扩展视频参数/自动时长/精确大整数。当前视频种子等仅能跟随发布默认，不能声称全参数可编辑。原审批、三版本与body冻结仍是唯一执行合同。
5. 以上与 A3 素材引用释放、团队/元数据和发布矩阵分别跟踪；本批不将素材剩余项勾完。正式发布与真实效果另行按同一制品验收，生产 Console 保持。

复验命令：后端 `go test -race ./internal/app ./internal/handler ./internal/platformintegration -run 'TestStudio(Published|PlatformPublished|Catalog|Doubao|Media|NativeMediaBody|Pi)|TestPublished' -count=1`；前端以 `web/test/platform-model-supply-fixture.test.tsx`、`platform-parameter-ranges.test.tsx` 为跨仓入口，再运行 validation 所列相关回归。
