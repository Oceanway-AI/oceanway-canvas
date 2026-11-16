# Studio 大图片上传恢复实测

2026-11-16 在 W7 Studio `8274f1d0841760b3e66916d5921de53f04fc0831` 与 Platform `69c7d4c712be1c865f028d121960b00eb6420eae` 完成一份合法 4096×2816 PNG 的实际 Canvas 上传恢复。原件为 34,621,850 字节，摘要见 `summary.json`。

正常文件选择入口只创建一次上传。第一个 8 MiB 分片返回 200 和 ETag 后，浏览器关闭页面，再打开原画布，在原节点选择同一文件。客户端查询原上传，只发送分片 2–5，五片各成功一次。同一节点最后保存正式 Resource，原有节点身份、类型、任务与素材键逐项一致，上传中状态清除，显示尺寸为 4096×2816。

首次脚本在原件 ready 后立即请求 `/api/resources/:id/file`，得到 503，原始失败保留在 `studio-browser-recovery.json`。该路由默认请求 display/original，宿主会转换成 preview。当时原件已就绪，preview 的云写入和摘要核对在 14:22:09 UTC 完成。之后同一原请求返回 307。正式 Canvas 回读使用 display/thumbnail，图片实际解码为 320×220。实际点击下载按钮使用 download/original，浏览器下载 34,621,850 字节，SHA-256 与原件一致。该次正常界面观察到的旧 file 路由请求数为 0。

数据库以 `BEGIN READ ONLY` 查询精确原上传、命令、Asset、Version 与 File 后回滚。只有一条命令上传、一份版本和一份文件，元数据 ready，thumbnail 和 preview 均 ready。账户已用容量从 88,496,774 增至 123,118,624 字节，恰好增加原件大小，预留从 0 回到 0。

原 worker 的限定 job 观测共 13 条。原件 PUT 为 16.010294 秒，HTTP 200，SDK 重试数 0。后续原件 GET 为 12.300321 秒，摘要匹配。这个批次的时延与前批次分别保留，不能据此归因通用 SDK 或 CDN 故障。

原始浏览器脚本对恢复后的分片 2–5 记录了 `byteLength=0`，因为 Playwright 没有提供这些请求的 `postDataBuffer()`。这个字段表示观察器无法获得请求体大小。五个已确认 ETag、worker 精确分片读取字节、最终原件摘要和数据库字节数共同证明实际内容。

`local-and-runtime.json` 记录 500 个 Web 文件、3394 个测试和 78186 个断言通过，以及 TypeScript、ESLint、改动文件格式和生产构建通过。运行版本为 `8274f1d`，提交后验证版本 `fd7a896` 只增加测试隔离与测试格式修改。首次全量六项失败与修复范围保留。W7 镜像为同 revision 的 linux/amd64，测试数据库副本已完成 schema 97 隔离恢复演练。

`run-browser-recovery.cjs` 是首次执行的脚本，包含创建上传的操作。它对已有 create 记录拒绝再次运行。已完成的本批次使用 `studio-readback.json` 和数据库记录作后续核验。

完成浏览器、数据库核对之后，保存原始文件的外盘暂时不可用。这里的 JSON 从本 agent 内部盘会话记录中提取精确工具返回，`artifact-recovery.json` 列出对应调用。`studio-recovered-ready.png` 从 `view_image` 当时显示的图像数据恢复，属于工具显示图像副本。原截图文件哈希尚未取得，因此未声称这份副本与原 PNG 字节完全相同。原浏览器状态、签名 URL、Cookie、错误对象和下载原件均未收录，外盘恢复后可继续取得原文件。
