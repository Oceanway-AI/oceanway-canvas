# Studio 执行与恢复合同

## 入口与权威

普通生成与原生 Pi 的模型步骤共用 OAC Task、队列、租约和文本回放。服务端重新读取 Platform 发布目录，只接受文本、图片、视频的明确协议。浏览器不能指定供应商地址、凭据、工作流或本地计费。音频及未适配协议返回暂不可用。

`POST /api/tasks` 接受稳定 `commandId` 与 OAC 创作意图。同一账号同一 command 只对应一个任务；修改意图返回 409。创建响应丢失时按 command 查询原任务。先保存不可变意图，再以该任务 ID 申请 EXA；SDK 的同 task_reference 原授权合同允许恢复授权响应丢失，不产生新调用。

## 持久化与派发

schema 94 新增服务端专用 StudioExecution。原生协议、精确请求字节、Content-Type、价格版本、额度约束和原授权加密保存；任务公共 JSON 不包含这些数据。保留 `.settings-key`，不保存 Cookie。执行表和 OAC Task 通过事务进入队列。

Worker 持有效 OAC 租约，以条件写把 prepared 改为 submitted，成功后才发送唯一一次模型 POST。断线、超时、503、进程崩溃、not_found 都只进入原 ID 查询；不换模型、不补发、不构造新收费请求。即使崩溃发生在标记后而网络发送前，也保持待核对，不假定没有执行。

文本只使用 SDK 非流式 ChatCompletions / Responses / Messages；将最终结果接回 OAC 回放/SSE，保留工具调用和思考签名。上游成功正文先加密落盘，随后解析并与业务结果原子完成。空输出或格式错误保留原响应，不能自动重新收费。失败仅依据原请求的明确终态。

图片/视频冻结原生 JSON，以同一 JSON 做 PreviewMedia 和 SubmitMedia；必须验证 allowed、combination_supported、price_version 与明确 wallet_max_points。参数按所选发布变体校验，不合并其他变体。平台负责扣费与精确价格计算，本地不预占/退款。

## 素材与 Agent

引用只接受当前账号已采用的固定 resource → asset/version/file 映射；提交使用 material://asset/version，由 Platform 准入。产物通过原 EXA 查询 TaskOutputs/Original，固定版本采用、绑定确认后才宣布任务成功。签名 URL 不作结果身份。

Pi 保留原编排、审批和工具执行。后台只持原 EXA，不持浏览器 Cookie；每个付费步骤先持久化独立请求标识。审批后的创作任务走相同入口，未知结果保持原步骤等待恢复。已提交任务不经旧的自动重试、供应商取消或本地账本。

## 验证顺序

先写 HTTP/数据库红灯：三种文本协议、响应丢失后进程恢复、同 command 幂等与冲突、授权丢失恢复、跨账号隔离、租约竞争、密文/日志泄漏、未知请求不重发。再验证精确媒体报价、固定素材引用、产物恢复和 Pi 工具调用。只用隔离免费夹具；真实登录/付费验收交由协调侧。
