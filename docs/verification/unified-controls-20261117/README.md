# Studio 公共三控件接入证据

范围为 WP-STUDIO-018 ST1 的公共控件宿主接线，沿前批空间上下文门禁和正式 `platformclient/v1.0.0-rc.5`。本批整合主线 `bc5ed314`，实际源码由包含本文件的提交固定；PR #11 保持草稿。最终推送前检查与 CI 运行在 Docs 的 PROC-02-091 登记。

## 设计制品与宿主行为

| 制品 | 已验收来源提交 | manifest 摘要 |
| --- | --- | --- |
| account-panel-v1 | `ea3f23f6709efc3ac85ad06dc22e757ef25c3c66` | `c153feebe74ee5764183285e21f51dee2c330af131cbf4fb7a48dc4e6b2b9e1d` |
| context-switcher-v1 | `289386183b4479b15be4f8fa56dbe64cf0df5218` | `8054b873f702c03b122b69c8ff35cd49a7b3edf2c913a06c1c4ad4e00fd8cdb4` |

原文件逐字节复用，专项测试验证 manifest 与各文件 SHA-256；宿主只接数据、主题 token、放置位置和事件边界。

- 顶部产品切换与底部姓名／空间分离；点击底部打开紧凑账户面板。空间选项通过 portal 独立呈现，列表不显示所有者／角色；团队徽标与选项复用小头像。
- 资金保留精确字符串；真实零值、不可读及部分失败分别显示，购买入口只按提供方 `can_purchase`，付款订单按 `can_read_finance`。
- 产品跳转前先完成原画布／素材持久化，再向 BFF 重新核验当前账号、空间、成员代次及登记产品路径。迟到响应、撤销入口和地址变化不能跳转。
- 菜单开启时 Delete、撤销与粘贴不传入画布；Escape 优先关闭菜单并恢复焦点，再次 Escape 可退出专注模式。
- 空间列表为只读，不写 Platform 默认值；团队选项明确禁用，不能以个人账户代替团队。ST2 接通归属和协作后再开放选择。

## 免费专项

前端命令：`bun test test/studio-shared-artifacts.test.ts test/studio-account.test.tsx test/studio-workspace-entry.test.ts`，6 个顶层测试通过、0 失败。内嵌账户 harness 7 例分别执行根路径与 `/studio/` 路径，不重复累加为额外顶层测试。覆盖组件制品、权限／精度／局部失败、迟到响应、真实组件与画布快捷键交互，以及原上下文门禁。

Go 专项使用真实 Studio handler、正式 SDK、HTTP Platform 夹具和隔离 SQLite；覆盖只读空间、Cookie 轮换、错误账号、撤销产品、精确地址、重复参数与 Console 办理目标，定向 handler 测试通过。未以本机 SQLite 代替生产 PostgreSQL 或双账号实际联验。

独立审查发现并修复权益／本人用量的局部失败未展示、菜单焦点下画布快捷键仍触发两项问题。修复后审查无剩余阻断项；最终 SHA 绑定在 PROC-02-091 登记。

## 真实页面浏览器验证

使用 Vite 实际应用及本仓注册路由，`studio_account_browser` 编译标签仅启动手动测试夹具。使用隔离 SQLite、模拟 Platform 会话／摘要／空间，不连接共享测试数据库；模拟显示账号和余额，不使用真实客户身份。建立一个免费空画布，模型执行入口明确拒绝，没有生成或付款。

| 视口与主题 | 实际验证 | 截图 |
| --- | --- | --- |
| 1440 × 900、浅色 | 侧栏两个触发位置、紧凑账户面板、方案／积分按钮 | [desktop-account.jpg](desktop-account.jpg) |
| 1440 × 900、深色 | 原 Studio 深色主题下的账户面板 | [desktop-dark.jpg](desktop-dark.jpg) |
| 1024 × 768 | 顶部产品选项独立展开 | [product-menu-1024.jpg](product-menu-1024.jpg) |
| 390 × 844 | 实际移动侧栏、账户面板、独立空间列表；Escape 分层关闭并恢复焦点 | [mobile-spaces.jpg](mobile-spaces.jpg) |
| 独立画布专注模式 | 账户入口可达；第一次 Escape 关闭账户仍留在专注模式，第二次退出专注模式 | [canvas-focus-account.jpg](canvas-focus-account.jpg) |

夹具素材列表返回 403，页面如实显示读取失败，本批不验素材功能。未验证 200% 文本缩放、正式跨站购买返回、真实团队页面或团队成员退出与重入；这些仍按工作包验收。截图和菜单测试不证明真实资金、素材和团队协作可用。

本批未部署、未修改共享环境、未触发真实模型／支付／邀请；未改变既有对象归属、数据库 schema、Task 或 Agent 执行机制。ST1 整体和 ST2—ST5 仍未完成。
