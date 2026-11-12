# Studio 图片尺寸消费验证

对应 OceanWay Docs ADR-0046、WP-MG-001 / MG-IMAGE-OUTPUT-SIZE-01 P4。Studio只消费 Gateway 经 Platform 发布的完整能力，不维护第二套比例或尺寸换算表。

## 实现

- 预设选择采用原生值：GPT使用`size`，Gemini使用`imageSize/aspectRatio`。Gemini不显示虚构的精确宽高。
- 自定义宽高仅在已发布能力允许时显示，整数且16对齐，单张`width × height`受供给限制；不乘图片数量。完整供给分支限制保留。
- 原生参数经过同一组包与校验进入原预估和执行授权路径，视频配置不被图片尺寸控件接管。
- 恢复默认值会退出自定义模式；画布保存恢复保留相同参数。

## 已完成验证

- 13项前端测试、79个断言通过，其中4项使用真实React/AntD交互。typecheck、变更文件格式通过。
- 后端专项`-race`通过，覆盖实际Gateway编译器→Platform正式目录投影的GPT、4MP与Gemini夹具。每个预设组包与SDK准备结果字节一致。
- 4MP边界允许2048×2048及2560×1440，拒绝2064×2048、1025×1024、浮点尺寸和未经确认的auto；`n=10`不改变单张限制。
- 双供给消费者回归通过：尺寸和质量各自都在聚合选项中，但不能跨分支组合；同一供给合法组合可以组包，不支持的组合不产生预估或提交正文。这是消费约束测试，不是新的厂商能力声明。
- Web生产构建通过。上述初轮后端检查使用私有临时GOWORK对接SDK候选源码；正式版本采用与全量验证在完成后单独登记。

本批未部署Studio、未改运行配置、未发起付费生成。原始日志保留在本机私有目录，夹具不包含业务身份、真实价格、供应凭据或结果地址。

## 正式SDK与全量复核（2026-11-13）

实现及夹具修复提交`a6ca38247557ecfe9c7c897bae8ad762a48142c0`，正式采用已发布`platformclient/v0.2.0-rc.1`（`25e9d80d295847fccd004c1e23647105e6d4ad85`）；go.mod/go.sum固定版本，无replace，最终验证使用GOWORK=off。

- 独立UTF8 PostgreSQL测试库、锁文件固定Pi依赖；`bun scripts/verify-before-push.mjs --jobs 3`完整11项任务通过，0失败。前端3322通过、485文件、77610断言；后端其余包及4个app分片、格式、类型、lint和gofmt均通过。
- 正式SDK专项app/platformintegration race、Go构建和Web生产构建通过。
- React/AntD的4个交互测试放入独立Bun子进程，避免全套SSR测试提前加载ReactDOM造成的测试环境污染。修正既有Agent工具schema快照与缺失协议版本的拒绝夹具，新增原任务查询恢复不重复生成断言。
- 早期SQL_ASCII、缺Pi依赖和磁盘不足的失败单独保留，不计为通过；最终完整日志在私有`.local/image-output-size/full-verify-resource-green.log`。

本地代码及免费组件验证通过；未推送Studio、未部署、未完成实际运行浏览器验收、未新增付费生成。
