# OceanWay Studio

OceanWay Studio 是基于 Open AI Canvas 创作代码的工作台，提供画布、文本／图片／视频创作、素材库、Skills 和原生 Pi Agent。账号、模型、执行授权、资金、订阅权益和长期素材使用 OceanWay Platform。

项目、画布、Task、Agent、偏好与素材引用直接关联稳定的 Platform 用户 ID。当前可用入口及尚待接通的创作能力见[首发入口合同](docs/plans/oceanway-studio-release-surface.md)。

## 快速开始

需要 Go 1.25、Bun、Node、PostgreSQL 和 Redis。先在独立开发环境配置 `.env.example` 中的 `DATABASE_URL`、`REDIS_URL`、`STUDIO_PLATFORM_ORIGIN`、`STUDIO_PLATFORM_PRODUCT_KEY`、`STUDIO_PUBLIC_ORIGIN`、会话 Host 与允许来源。产品 Key 只在后端私密环境中注入。

```bash
git clone https://github.com/Oceanway-AI/oceanway-canvas.git
cd oceanway-canvas
mkdir -p .local/project-workbench-debug .local/cache
cd backend/agent-runtime/pi
npm ci --omit=dev --ignore-scripts
cd ../..
CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug go run ./cmd/migrate-schema up
CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug go run ./cmd/server
```

另一个已导入相同运行配置的终端启动前端。

```bash
cd web
bun install --frozen-lockfile
bun run dev
```

通过已在 Platform 登记的 HTTPS 开发入口访问。Vite 使用 `VITE_API_PROXY_TARGET` 代理后端请求。子路径部署设置 `VITE_APP_BASE_PATH=/studio/`，后端设置 `STUDIO_PUBLIC_BASE_PATH=/studio`。部署时两者必须匹配。

自有预构建 Web Release 默认使用根路径。子路径的 Web 需以相同源码和 `VITE_APP_BASE_PATH` 构建，单独修改后端运行变量无法改变已构建的前端路径。

## 数据与协调

开发、测试和部署统一使用 PostgreSQL。数据库结构为 Studio v98，空库直接创建创作业务表。Redis 承担限流、并发和跨实例协调。数据目录保存临时媒体、技能文件与加密设置密钥。

旧 Studio 测试数据通过 `migrate-schema reset-studio` 生成清理清单，核对后用 `--apply` 执行。该命令先释放 Studio 的 Platform 引用与执行授权，再重建业务 schema。完整操作和恢复方式见[后端数据库](docs/content/docs/backend/backend-database.mdx)。

## 部署

源码热更新使用 `docker-compose.dev.yml`，源码构建使用 `docker-compose.local.yml` 或 `docker-compose.server.yml`。配置 `.env.example` 中的服务连接和 Platform 必填项后启动相应编排。

受管发行入口为 `scripts/install-server-image.sh`、OceanWay 自有 Release、`docker-compose.deploy.yml` 和 Host Updater。Release、镜像与更新器摘要按同一版本固定。实际制品安装、升级和回退的验收状态登记于 FS-STUDIO-001／WP-STUDIO-020。

保留原生 `yingce-agent` 进程／容器。宿主机开发未设置 `YINGCE_AGENT_URL` 时由后端启动 Node，容器部署使用独立 Agent。

## 安全与验证

所有对象操作校验当前 Platform 身份和 Studio 业务归属。浏览器写请求携带身份版本与操作人，切号或旧版本请求会被拒绝。密钥、Cookie、数据库和生成媒体不入 Git。

后端数据库测试通过 `CANVAS_TEST_POSTGRES_DSN` 连接独立测试库，每个夹具创建隔离 schema。需要 `redis-server` 和 Pi 依赖。验证命令见[贡献指南](CONTRIBUTING.md)，接口合同见[HTTP API](docs/content/docs/backend/http-api.mdx)。

## 文档

- [文档入口](docs/index.md)
- [功能清单](docs/content/docs/overview/features.mdx)
- [应用实施进度](docs/plans/oceanway-studio-application-progress.md)
- [本地开发](docs/content/docs/backend/local-development.mdx)
- [安全策略](SECURITY.md)

## 许可证

本项目采用 [MIT](LICENSE) 协议。项目特定代码的版权归 Open AI Canvas 贡献者所有；第三方组件按各自附带的许可证和版权声明使用。

---

## 贡献者与团队

感谢参与产品设计、开发、测试、内容和社区建设的成员。

<table>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-sikongyue.png" alt="爱笑的毛毛虫" width="72" height="72"><br>
  <strong>爱笑的毛毛虫</strong><br>
  <sub>用户名：sikongyue</sub><br>
  <sub><a href="mailto:315515767@qq.com">315515767@qq.com</a></sub><br>
  <sub><em>正在啃 main 分支，争取下次 merge 的时候变成蝴蝶</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-delve.jpg" alt="delve-s" width="72" height="72"><br>
  <strong>delve-s</strong><br>
  <sub><a href="mailto:3013141136@qq.com">3013141136@qq.com</a></sub><br>
  <sub><em>我亦无他，惟手熟尔</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-CyrusAuyeung.jpg" alt="CyrusAuyeung" width="72" height="72"><br>
  <strong>CyrusAuyeung</strong><br>
  <sub><a href="mailto:cyrusauyeungho@gmail.com">cyrusauyeungho@gmail.com</a></sub><br>
  <sub><em>HKUST(GZ) UG</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-nz.jpg" alt="奶大佬" width="72" height="72"><br>
  <strong>奶大佬</strong><br>
  <sub><a href="mailto:1304634970@qq.com">1304634970@qq.com</a></sub><br>
  <sub><em>人生就是要不断的探索</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-dyh.jpg" alt="dyh" width="72" height="72"><br>
  <strong>dyh</strong><br>
  <sub><a href="mailto:1613203335@qq.com">1613203335@qq.com</a></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-kyori.jpg" alt="kyori" width="72" height="72"><br>
  <strong>kyori</strong><br>
  <sub><a href="mailto:1771634408@qq.com">1771634408@qq.com</a></sub><br>
  <sub><em>励志成为未来最好用的画布仓库的贡献者</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-bowen.jpg" alt="Bowen" width="72" height="72"><br>
  <strong>Bowen</strong><br>
  <sub><a href="mailto:admin@bowen.games">admin@bowen.games</a></sub><br>
  <sub><em>剑走偏峰，雷厉风行。</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-ken.jpg" alt="ken" width="72" height="72"><br>
  <strong>ken</strong><br>
  <sub><a href="mailto:2506802@qq.com">2506802@qq.com</a></sub><br>
  <sub><em>走自己的路</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-fish.png.jpg" alt="fish" width="72" height="72"><br>
  <strong>fish</strong><br>
  <sub><a href="mailto:cihai.sea@gmail.com">cihai.sea@gmail.com</a></sub><br>
  <sub><em>AI 界热于助人的拖油瓶</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-QAyong.jpg" alt="QAyong" width="72" height="72"><br>
  <strong>QAyong</strong><br>
  <sub>ID / B站：QAyong</sub><br>
  <sub><a href="mailto:2110491559@qq.com">2110491559@qq.com</a></sub><br>
  <sub><em>AI 短剧合规，资产确权</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-K37ix.jpg" alt="_K37ix." width="72" height="72"><br>
  <strong>_K37ix.</strong><br>
  <sub><a href="mailto:2773843782@qq.com">2773843782@qq.com</a></sub><br>
  <sub><em>Making things that think</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-rou.jpg" alt="Rou" width="72" height="72"><br>
  <strong>Rou</strong><br>
  <sub><a href="mailto:rou325089@163.com">rou325089@163.com</a></sub><br>
  <sub><em>上善若水</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-vv.jpg" alt="vv" width="72" height="72"><br>
  <strong>vv</strong><br>
  <sub>dy/xhs：荣灵</sub><br>
  <sub><a href="mailto:2838033228@qq.com">2838033228@qq.com</a></sub><br>
  <sub><em>就是水水</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-dominic1556.jpg" alt="Dominic1556" width="72" height="72"><br>
  <strong>Dominic1556</strong><br>
  <sub><a href="mailto:184026530@qq.com">184026530@qq.com</a></sub><br>
  <sub><em>Done is better than perfect</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-yuxi.jpg" alt="宇熙" width="72" height="72"><br>
  <strong>宇熙</strong><br>
  <sub><a href="mailto:53121904@qq.com">53121904@qq.com</a></sub><br>
  <sub><em>年轻的时候不狂，老了拿什么回忆</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-yingzi.png" alt="影子" width="72" height="72"><br>
  <strong>影子</strong><br>
  <sub><a href="mailto:305818148@qq.com">305818148@qq.com</a></sub><br>
  <sub><em>年纪大佬才明白人要顺势而为。</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-ray.jpg" alt="Ray" width="72" height="72"><br>
  <strong>Ray</strong><br>
  <sub><a href="mailto:cnraylee@qq.com">cnraylee@qq.com</a></sub><br>
  <sub><em>AI时代的全栈落地工，欢迎找我聊需求</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-bjsg.jpg" alt="不见山谷" width="72" height="72"><br>
  <strong>不见山谷</strong><br>
  <sub>VV：yu170718</sub><br>
  <sub><a href="mailto:1762202553@qq.com">1762202553@qq.com</a></sub><br>
  <sub><em>空山不见人，但闻人语响</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-yep.jpg" alt="yep" width="72" height="72"><br>
  <strong>yep</strong><br>
  <sub><a href="mailto:1239738103@qq.com">1239738103@qq.com</a></sub><br>
  <sub><em>思考，坚持</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-hamburger.jpg" alt="汉堡爸爸" width="72" height="72"><br>
  <strong>汉堡爸爸</strong><br>
  <sub>VV：jxs62888</sub><br>
  <sub><a href="mailto:309151651@qq.com">309151651@qq.com</a></sub><br>
  <sub><em>没什么大不了</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-daqzia.jpg" alt="daqzia" width="72" height="72"><br>
  <strong>daqzia</strong><br>
  <sub>VV：wangzhiwei-8234</sub><br>
  <sub><a href="mailto:wzwzcb@gmail.com">wzwzcb@gmail.com</a></sub><br>
  <sub><em>NullPointerException</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-xingmeng.jpg" alt="醒梦" width="72" height="72"><br>
  <strong>醒梦</strong><br>
  <sub>VV：love-is-heart-is</sub><br>
  <sub><a href="mailto:1948863412@qq.com">1948863412@qq.com</a></sub><br>
  <sub><em>Always believe that good things will happen</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-morrow.jpg" alt="morrow" width="72" height="72"><br>
  <strong>morrow</strong><br>
  <sub>VV：morrow0127</sub><br>
  <sub><a href="mailto:3201190041@qq.com">3201190041@qq.com</a></sub><br>
  <sub><em>最难不过坚持</em></sub>
</td>
</tr>
<tr>
<td align="center" valign="top" width="33%">
  <img src="assets/user-chunqiu.jpg" alt="春秋" width="72" height="72"><br>
  <strong>春秋</strong><br>
  <sub>Agent 记忆核心贡献者 · VV：A13068788886</sub><br>
  <sub><a href="mailto:558328@qq.com">558328@qq.com</a></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-song.jpg" alt="亿媒网老宋" width="72" height="72"><br>
  <strong>亿媒网老宋</strong><br>
  <sub><a href="https://em8.top">亿媒网 em8.top</a> · <a href="mailto:61731514@qq.com">61731514@qq.com</a></sub><br>
  <sub><em>全国10W+媒体、自媒体，助力企业品牌传播</em></sub>
</td>
<td align="center" valign="top" width="33%">
  <img src="assets/user-Wyatt_K.png" alt="Wyatt_K" width="72" height="72"><br>
  <strong>Wyatt_K</strong><br>
  <sub><em>千里之行，始于足下。</em></sub>
</td>
</tr>
</table>

## 3D 预演台赞助用户

感谢以下用户赞助 3D 预演台的开发。

<table>
<tr>
<td align="center" width="20%"><a href="https://github.com/TripsCoder"><img src="https://github.com/TripsCoder.png?size=128" width="64" height="64" alt="TripsCoder"><br><sub><strong>@TripsCoder</strong></sub></a></td>
<td align="center" width="20%"><a href="https://github.com/sugtex"><img src="https://github.com/sugtex.png?size=128" width="64" height="64" alt="sugtex"><br><sub><strong>@sugtex</strong></sub></a></td>
<td align="center" width="20%"><a href="https://github.com/Leyi813"><img src="https://github.com/Leyi813.png?size=128" width="64" height="64" alt="Leyi813"><br><sub><strong>@Leyi813</strong></sub></a></td>
<td align="center" width="20%"><a href="https://github.com/daoge05178"><img src="https://github.com/daoge05178.png?size=128" width="64" height="64" alt="daoge05178"><br><sub><strong>@daoge05178</strong></sub></a></td>
<td align="center" width="20%"><a href="https://github.com/AetherNo2332"><img src="https://github.com/AetherNo2332.png?size=128" width="64" height="64" alt="AetherNo2332"><br><sub><strong>@AetherNo2332</strong></sub></a></td>
</tr>
</table>

<table>
<tr>
<td align="center" width="20%"><strong>vv</strong><br><sub>2838033228@qq.com</sub></td>
<td align="center" width="20%"><strong>克里斯</strong><br><sub><a href="https://artbox.top">artbox.top</a></sub></td>
<td align="center" width="20%"><strong>宇熙</strong><br><sub>53121904@qq.com</sub></td>
<td align="center" width="20%"><strong>落羽</strong><br><sub>luoyyv66@gmail.com</sub></td>
<td align="center" width="20%"><strong>今夕何在</strong><br><sub>809321478@qq.com</sub></td>
</tr>
<tr>
<td align="center" width="20%"><strong>Alpha-M·Break</strong><br><sub>15739564793@qq.com</sub></td>
<td align="center" width="20%"><strong>不吃鸭梨</strong><br><sub>elio02519@gmail.com</sub></td>
<td align="center" width="20%"><strong>九月</strong><br><sub>563641352@qq.com</sub></td>
<td align="center" width="20%"><strong>A</strong><br><sub>jiaboxuan1205@gmail.com</sub></td>
<td align="center" width="20%"><strong>fochaobaby</strong><br><sub>leoxie.design@gmail.com</sub></td>
</tr>
</table>
