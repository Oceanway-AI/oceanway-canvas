# Studio rc.7 原生媒体本地验收

基线 `73c77c7223e2ff5339366167e843f4b2a4e8674d`；SDK 为正式 `platformclient/v0.1.0-rc.7` / `9ed3fd9d6478ba04e8a55a190ce83908a395b8fa`。模块校验和 `h1:ECHq8oAigmiHHX3pcljExN9L163sgAWB96jmgvB5+18=`，无 replace。用户授权 SDK tag 发布和继续产品接入，应用本批仅本地提交。

## 已验证行为

普通 Chrome 扩展/CUA 操作真实 `/create`：Gemini 图片、OpenAI 图片、ModelArk 视频分别预览并完成一次生成，进入原 Platform Task/资金/素材 worker，再回填 Studio Resource。请求视频时长为 4，免费 Provider 实际输出 1 秒，Platform 按实际用量结算；各 Task 为 SUCCESS、settled、10 quota。两张测试 PNG 为黑色 1×1，MP4 为蓝色 1 秒，不能把测试内容当商业模型效果。

点击“画布中继续”插入 3 个节点，URL 在保存后消费 handoff；刷新后仍为相同 3 个固定 Resource，页面显示已同步，视频可播放。服务端画布 revision 4，只有 2 个 image 和 1 个 video Resource，没有嵌套本地 videoPreview 或持久签名 URL。见 `browser-result.json`。

视频静态预览请求原 Resource 的 `poster`，真实签名派生路径返回 200，见 `poster-network.json`。CDP 报告缓冲截断，因此该片段只证明 poster 访问，不能单独证明完整时段零上传；数据库的 3 条资源及画布元数据另行核验未产生额外封面资源。

测试宿主使用正式 Studio API/rc.7、Platform HTTP 路由、独立 PostgreSQL、既有 Gateway 29200 免费 fixture。只有 loopback 身份传输映射已登记的 HTTPS Host/Origin；素材显示桥只信任测试 edge 的固定证书并保留实际签名路径/票据。没有浏览器证书例外或生产 TLS 降级。本批不证明跨站 SSO、生产 CDN 或真实付费供应。

## 检查

- `go test ./internal/app ./internal/platformintegration ./internal/handler ./internal/canvas/contract -run 'Test(Studio|Native|Published|Generation|Options)' -count=1` 通过；contract 无测试文件。
- `go test -race ./internal/app ./internal/platformintegration -run 'Test(StudioNative|StudioAgent|Native|Published)' -count=1` 通过。相关四包 vet 通过。
- 最终相关 Web 7 文件、49 测试、218 断言通过；typecheck、lint、build 通过。构建保留既有 chunk/动态导入提示。新增字段使两条审批字段白名单断言失败，更新预期并增加 nativeVariantId、零值/false 参数保留断言后通过。
- Studio `TestStudioNativeGatewayBrowser` 与 Platform `TestPlatformGatewayStudioHost` 最终 round05 均通过。Platform 独立 PG preview race 通过、源码前后摘要一致、临时 PG 已清理。Studio/Platform/Vite 临时浏览器宿主已停止，Gateway 29200 由所属会话继续管理。
- `source-manifest.json` 固定本批代码文件摘要；私有 manifest、日志、SQLite、凭据和截图留本机 `.local`，不入仓。

## 失败与边界

round01 缺素材签名配置，不能算生成通过。round02 Gemini 已归档，但测试状态类型断言及派生显示未完成。round03 两图成功，视频预览被 Platform 的 legacy output_mapping_unverified 拒绝；修复已通过发布能力验证后的 combination_supported 判定。round04 三项成功并转入画布，但发现浏览器再次上传封面导致同步停留。round05 修复后上述画布恢复/同步和封面路径通过。保留各批次边界，不回写早期失败为全通过。

七种协议的目录/组包/审批由本机专项测试覆盖；本次实际 Gateway 三型号仅包含 Gemini、OpenAI 和 ModelArk。当前视频 release 没有发布 Sora/New API，这两种预览正确拒绝。未运行商业型号、实际 Pi 文本到媒体浏览器流程、完整参考模式或其他产品。复杂 JSON 输入仍由服务端 schema 准入，union string/object 的编辑提示需要后续专门完善。全量 CI/推送前检查未运行，本批不推送 Studio。

## 重现入口

1. 由 Gateway 会话提供固定免费运行清单；Platform 用 `scripts/platform-commercial-test.py --packages controller --filter '^TestPlatformGatewayStudioHost$' --gateway-fixture <runtime.json> --capability-browser --output-dir <isolated-output> --timeout 30m` 启动独立正式宿主。
2. Studio 在 backend 中设置 `STUDIO_JOINT_PLATFORM_MANIFEST=<isolated-output>/browser/studio-private.json` 与 `STUDIO_JOINT_OUTPUT_DIR=<ignored-local-directory>`，运行 `go test ./internal/handler -run '^TestStudioNativeGatewayBrowser$' -count=1 -timeout 30m`。
3. 读取 Studio ready.json 的 URL，设置 `VITE_API_PROXY_TARGET`，用 Bun 启动现有 Vite。普通 Chrome 完成三型号生成、转画布、刷新和播放。私有 manifest 不打印、不复制到文档。
4. 两宿主各自输出目录写 `stop` 后检查最终断言/PG 清理，再停止自己启动的 Vite；不停止其他会话的 Gateway。
