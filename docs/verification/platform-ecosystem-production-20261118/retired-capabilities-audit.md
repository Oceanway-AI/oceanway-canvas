# OAC 退出代码的平台复用核对

核对日期为 2026-11-18，关联 FS-STUDIO-001、WP-STUDIO-020 和 PROC-02-094。用户要求在退出旧实现时确认整个平台可吸收的能力。

本记录依据两个固定源码版本进行静态核对。旧实现取自 Studio `7e74296bb9d4b4cac9b53db4fa9d29605d840bdf`，Platform 取自 `060da1989a6e4b96df6cc9be9fb4c6ed4c524c62`。Platform 生产登记版本仍为 `1614a0f8392a7cfa66b9a178f41c17b54b47e800`，本记录没有重新执行 Platform 测试或更改生产配置。

## 已有平台覆盖

| 旧实现及可保留经验 | Platform 源码核对 | 本次处理 |
| --- | --- | --- |
| `internal/app/task_billing.go`、`internal/repository/finance_billing.go` 的不确定提交、冻结资金、幂等预留、授权上限和价格快照 | `service/platform_media_generations.go` 将价格快照、原始请求和资金预留放入同一准入事务；`platform_media_recovery.go` 只在有明确未派发证据时释放；`platform_relay_preexecution.go` 处理未执行请求。已有 PostgreSQL 用例覆盖原快照、竞争、未知派发和原单恢复 | 保留这些故障场景作为最终联验清单，Studio 删除本地扣款／退款状态机，消费 Platform 原回执。旧人工补扣入口不作为平台缺口 |
| `internal/app/task_media_recovery.go`、`resource_offload_worker.go` 的失败保存恢复、固定资源身份和延迟重试 | `service/platform_material_archive.go` 使用确定性归档 ID、租约 token、CAS、不可变原件写入和读回 SHA／大小／MIME 校验；`platform_material_recovery.go` 核对原任务、成员及已结算资金，只唤醒原归档 | 固定 ID、重入和丢回执场景已有对应实现。Studio 采用原素材恢复合同 |
| `internal/storage/errors.go` 的错误分类与敏感信息过滤 | `temporaryArchiveStoreError` 识别 S3 操作、408／429／5xx、网络和取消类错误；`platform_material_object_observation.go` 记录状态、耗时、字节和哈希对象标识，配套测试排除密钥、URL 和正文泄漏 | 复用诊断目标和异常样例，退出 Studio 专属供应商 SDK |
| `internal/assets/access.go` 的签名缓存窗口 | `service/platform_material_access.go` 使用随机 nonce 与短期签名；`deploy/cloudflare/materials-worker.mjs` 在缓存读取之前逐次验签，缓存 key 使用 origin + pathname，随后处理 Range、ETag、304 和 HEAD | 平台边缘缓存已消除签名参数造成的碎片。浏览器 URL 缓存是另一层，只有测得重复传输成本后再评估同一会话内 URL 复用 |
| `internal/app/storage_location.go` 的真实 PUT／Range／DELETE 验证 | `service/platform_material_objects.go` 校验存储配置、桶隐私和不可变写入；`platform_material_addressing_test.go`、`platform_material_object_observation_test.go` 及云验收脚本已有读写、错误和观测覆盖 | 保留凭据轮换、Range 内容错误和清理失败作为平台验收样例，既有测试是否在目标环境通过须按该次验收登记 |
| `internal/app/user_groups.go`、`finance.go` 的同一规则贯穿可见模型、报价和准入 | `service/platform_product_models.go`、`platform_capability_admission.go`、`platform_sales_pricing_policy.go`、`platform_media_generations.go` 分别维护产品目录、能力准入、销售策略和资金快照 | 统一规则与实际准入复核值得保留，旧用户组、渠道白名单和倍率表从 Studio 退出。全部供应调度等价性仍需按 Media Gateway 规范核对 |

上表表示源码中存在对应机制。生产故障恢复、性能和所有供应类型的等价性需要真实环境结果支持。

## 值得 Platform 接续评估的三项

### 存储级故障退避

旧 `internal/app/task_media_fallback.go` 在同一 provider／endpoint／bucket 五分钟内三次失败后暂停直写五分钟。旧 `resource_offload_worker.go` 对同批次不可用目标停止重复写入，并按 1／5／15／30／60 分钟及两小时安排重试。

Platform 当前归档按每项 `min(300, 2 * attempts)` 秒退避，并以租约 token 防止多进程重复提交。在本次核对的素材 worker 中未找到针对整个存储目标的共享故障退避。大量归档同时遇到同一存储故障时，单任务重试仍可能产生集中请求。

建议由 Platform 素材交付工作包评估共享退避及受控探测。需覆盖两个 worker 同时发现故障、恢复后逐步放行、其他健康目标继续处理、进程重启后策略一致、原任务／原件 ID 和资金不变。旧内存计数实现只提供行为参考。验收记录必须包含故障期间请求数和恢复时间。

### 临时磁盘预算与处理背压

旧 `task_media_fallback.go` 使用 10 GiB 总预算和 5 GiB 最小剩余空间，按载荷大小设置上传时间。Platform `platform_material_temp.go` 在创建后立即 unlink 临时文件，进程退出自动释放文件描述符；派生临时目录另有租约和清理。当前核对范围未发现跨任务预留临时磁盘容量的准入机制。

建议在平台现有归档和派生 worker 中评估可配置的临时磁盘预算、单任务字节上限、队列背压和取消释放。原数值不能直接作为生产参数。验收包括低磁盘、大文件并发、worker 被终止、容量恢复后继续原任务，以及用户之间相互隔离。最终 ready 仍以对象存储中的原件完整验证为依据。

### 跨存储位置迁移的原件定位

旧 `internal/app/resource_oss.go` 与 `storage_location.go` 将 provider／endpoint／bucket 和配置身份绑定到资源，并在配置变化后保留历史位置。Platform `model/platform_material.go` 保存对象 key，`service/platform_material_objects.go` 的实例使用统一运行配置中的 bucket。本次检索素材源码和相关脚本未找到逐文件存储位置版本或跨桶迁移切换协议。

建议在计划实际跨桶／区域迁移时，将存储位置版本或整库迁移清单纳入 Platform 运维设计。验收包括旧版本可读、SHA／大小核验、迁移中断重入、配置回退和凭据轮换，业务 Asset／Version／Binding ID 保持稳定。既有单桶配置下属于演进准备项。

## 继续沿产品或现行平台边界保留

Studio 的 Pi／Task 租约、审批依赖摘要、取消来源、文本回放、作品回填、Asset／Resource 引用和项目快照属于创作运行机制。`.settings-key` 继续用于持久执行授权加密，存储设置清理后仍需验证进程重启解密。系统 Logo、Live2D 和 Agent 外观等本地系统素材保留受控访问。

旧本地账号、角色、充值、兑换、返佣、订阅、价格倍率、供应商凭据及用户 OSS 设置已经有明确的统一归属。可复用的故障场景、幂等约束和测试样例随固定 Git 版本保留，退出代码不承担继续运行的职责。

## 追溯与接续

删除后的源码可用 `git show <上述Studio提交>:backend/<路径>` 精确恢复。Platform 对照文件按上述 Platform 提交读取。查重检索包括“素材”“存储熔断”“历史存储”“磁盘预算”，已有规范 PLAT-02-064／065／066、DEV-X-01-015，Studio 退出沿 FS-STUDIO-001／WP-STUDIO-020。

三项建议归入提供方评估清单，本次只形成源码核对与验收建议。最终 Studio 退出验收仍需完成旧任务账务、渠道分组、剩余查询与正式 Release 联验。
