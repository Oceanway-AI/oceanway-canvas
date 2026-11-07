# Studio 素材首批实施合同

本批只使用 platformclient rc.5 的 materials v2，原件存储、容量、素材版本和签名访问由 Platform 管理。OAC Resource ID 保留，新增不可变 asset/version/file 映射，不保存签名 URL 或对象存储凭据。团队空间后续接入；首批只接受与当前账号匹配的 account owner。

## HTTP 与持久化

- `POST /api/resources/platform-uploads`：保存按用户隔离的上传意图后，用稳定 command_id 请求上传票据。同一个命令不同文件参数返回冲突；未知创建响应只可重用原命令。
- `GET /api/resources/platform-uploads/:id`、`POST .../:id/complete`：先查本地上传归属，再查询或完成同一 Platform upload_id。pending 不等于成功，丢失完成响应查询原上传，不重开上传。
- 上传票据由浏览器独立请求，`credentials: omit`、禁止重定向；不经过带登录态的业务 http 客户端。SDK 不上传原件；Studio 不落原件。
- `POST /api/resources/adopt`：按指定 asset/version 从 Platform 读取完整信息、验证账号与 file 身份；先保存 pending Resource 与映射，再以固定 command_id 绑定该 Resource，确认 active 后返回 ready。绑定响应未知时按 command_id 恢复，重试不新建 Resource 或换版本。
- `GET /api/resources/:id` 与 `POST /api/resources/access`：本地所有权校验后解析固定映射。展示 original 意图映射为 Platform preview/playback，明确实际变体；不把缩略图失败降级为原件。download/copy/browser-process 使用 original。浏览器不能请求 provider-input；执行 worker 后续按任务绑定申请。
- `GET /api/materials/assets` 与 `GET /api/materials/storage`：当前会话 SDK 只读列表与容量，不建立第二套容量计算。

本地迁移新增 Studio 上传意图与素材映射表；保留 OAC Resource 的宽高、时长等编辑元数据。稳定引用由服务端校验生成，浏览器不能传 storage provider/object key/binding ID。

## 先失败再实现的验收

1. 上传创建路由由 501 转为 SDK v2；产品权限、会话与 Cookie 更新保留。
2. 同命令不同文件拒绝；另一账号无法查询/完成；未知完成后 GET 恢复同一上传。
3. 采用固定版本、owner/file 校验、绑定恢复、重复采用幂等；失败不返回 ready。
4. 签名 URL 不写数据库；访问过期重新签发；批次逐项错误不能伪造成功或跨账号泄漏。
5. 浏览器票据不携带 Cookie/产品密钥，禁止重定向；完成 pending 轮询同一上传，账号切换终止，失败向上传播。

生成结果、任务引用与画布项目快照关联在任务批次继续；这批通过不代表收费生成或 Pi Agent 已验收。
