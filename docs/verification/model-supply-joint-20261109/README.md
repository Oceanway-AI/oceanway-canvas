# Platform / Studio 模型参数联合验收

用户要求与会话 `01a1bbad-1640-7ac3-8c82-ae59a56fae48` 联合调整供给及可用参数。分工已实际通知并得到对方接续：Platform 维护模型设置与能力发布；本会话维护 Studio 公开目录投影、完整 OAC 控件与消费。没有编辑对方工作区。

## 提交与状态

| 层 | 本批事实 | 版本 |
| --- | --- | --- |
| Docs 规范先行 | PLAT-04-029 §0、PLAT-02-059 C1/C4 联合工作顺序 | `f1fa61d0` |
| Studio 文档 / 失败测试 | 原执行合同增量；Go 两顶层失败、前端三项失败先复现 | `13ab270` / `2b25b89` |
| Studio 修复 | 范围、音频别名、原生控件和跨仓样例消费 | `f210dfa` |
| Platform 候选 | 已同步 main，模型设置本批仍未提交；不能单独用 HEAD 重现所有变更 | HEAD `7401e5cf` + `producer-manifest.json` 所记源码摘要 |
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

1. Platform 将候选提交形成可复现版本；保留当前图片拒绝样例，另补 ready 图片样例及其输出映射证据，禁止把当前回执直接改为成功。
2. Studio 接通 `size_presets` 和 `x-size-constraints` 的预设/自定义尺寸校验、总输出数量与字段别名。当前复杂尺寸限制只由 Platform 最终门禁兜住，不能声称界面全支持。
3. Studio 补参考 `requires_fields/excludes_fields`、`max_reference_items`、`required_any_of` 及首尾帧单值 url 传输。每项先取得提供方正反样例，再补红灯和原 Task/Pi 免费回归。
4. 明确非默认变体的原生选择入口，并验证 Agent 的扩展视频参数/自动时长/精确大整数。当前视频种子等仅能跟随发布默认，不能声称全参数可编辑。原审批、三版本与body冻结仍是唯一执行合同。
5. 以上与 A3 素材引用释放、团队/元数据和发布矩阵分别跟踪；本批不将素材剩余项勾完。正式发布与真实效果另行按同一制品验收，生产 Console 保持。

复验命令：后端 `go test -race ./internal/app ./internal/handler ./internal/platformintegration -run 'TestStudio(Published|PlatformPublished|Catalog|Doubao|Media|NativeMediaBody|Pi)|TestPublished' -count=1`；前端以 `web/test/platform-model-supply-fixture.test.tsx`、`platform-parameter-ranges.test.tsx` 为跨仓入口，再运行 validation 所列相关回归。
