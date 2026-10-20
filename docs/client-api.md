# 四客户端接口边界

一个 Go 服务、一个部署端口，四个明确的客户端入口。四端入口只负责确定客户端范围，随后进入同一套业务处理、认证、校验及持久化代码。

| 客户端 | 公开入口 | 管理入口 | 内部平台标识 |
|---|---|---|---|
| 手机 | `/api/v1/mobile/` | `/api/admin/clients/mobile/` | mobile |
| 平板 | `/api/v1/tablet/` | `/api/admin/clients/tablet/` | tablet |
| 电视 | `/api/v1/tv/` | `/api/admin/clients/tv/` | tv |
| 电脑 | `/api/v1/desktop/` | `/api/admin/clients/desktop/` | web |

手机、平板各支持 Android/iOS，电视支持 Android，电脑支持 Windows/Web。`desktop` 是新 URL 中的名称；现有存储和响应的 `web` 标识保持兼容。

## 职责和数据归属

- 共享：CMS 连接、影视内容来源、会员身份、系统连接设置、服务域名和服务端更新。四端会员共用 CMS 身份，同一会员令牌可以跨端使用，退出会撤销该服务会话。
- 按端隔离：运营栏目与功能配置、草稿与发布历史、广告配置及回传记录、应用版本、缓存与搜索索引。四端访问同一 CMS，但各自使用独立缓存范围。
- 主题中心：视觉骨架、颜色、图片；运营端仅发布运营字段，不覆盖客户端安装包中的视觉骨架。打包器负责本地构建与签名。
- 服务端更新仍在共享的 `/api/admin/system/update` 下；应用版本属于各客户端的 `releases`，两者互不代替。

## 路径合同

公开入口后接：

- `GET bootstrap`：已发布运营配置与该端版本提示。支持 `os`、`version`、`contract`。
- `GET catalog/categories`、`GET catalog`：真实内容来源的分类与影片目录，沿用分页、搜索及详情参数。
- `POST playback`：经 CMS 适配器校验后取得播放地址。
- `member/config`、`member/plans`、`member/profile`、`member/orders`、`member/order` 为 GET；登录、注册、退出、购买、升级、充值、结算为 POST，沿用现有 CMS 适配合同。
- `GET ads/config`、`POST ads/decision`、`POST ads/events`、`GET ads/balance`：广告合同见 advertising-api.md。

管理员入口后接：

| 接口 | 方法 | 含义 |
|---|---|---|
| statistics | GET | 当前端真实请求统计和持久业务摘要，需要管理员会话 |
| state | GET | 当前端运营状态 |
| draft | PUT | 保存运营草稿，传 theme/version |
| publish | POST | 发布当前端草稿，传 version |
| rollback | POST | 回退当前端历史，传 version/targetVersion |
| releases | GET/POST/PUT | 查询、新增、编辑当前端应用版本 |
| releases/delete | POST | 按 id/revision 删除当前端版本 |
| ads | GET/PUT | 当前端广告状态与草稿 |
| ads/publish、ads/diagnose | POST | 发布及诊断当前端广告 |
| cache | GET | 当前端真实缓存状态 |
| cache/clear | POST | 清理当前端指定 kind 的缓存；kind=all 时一次清理 config、users、catalog |
| cache/search/rebuild | POST | 当前端搜索索引重建 |

管理接口仍要求管理员会话，写入仍要求可信来源。保存和发布沿用原有版本并发控制与原子持久化。

路径确定客户端后，重复平台参数、空平台参数、无效查询串，以及与路径冲突的 `platform`、`X-Hetun-Platform`、运营 `template` 返回 400。跨端修改或删除应用版本返回 404。未列入路由白名单的路径返回 404。请求体中会员支付的 `platform` 是 CMS 支付参数，不作为设备标识处理。

## 兼容和真实能力

旧 `/api/client/bootstrap?platform=...`、`/api/catalog`、`/api/member/...`、`/api/admin/draft` 等保留并进入同一业务实现，供已安装客户端继续使用。新增后台调用使用四端明确入口。旧接口不是第二份业务系统。

AdMob 服务商签名回调继续使用共享 `/api/ads/callback/admob`；不能改写其签名查询参数。广告下发返回与请求端一致的 decision/event/balance 路径。

缓存未配置时清理返回 409，Redis 连接失败返回 503。分类可读取不代表播放、支付或原生广告 SDK 可用；会员、播放依赖 CMS 适配，商业广告依赖原生 SDK 与服务商配置。`iapReady`、`cardReady` 等状态沿用实际能力。本次测试使用隔离 CMS HTTP 夹具和临时 Redis，不代表生产站点或真机已经验收。

