# 轮询解析播放器接口与接入

位置：左侧电脑客户端下方、系统管理上方的独立入口。规则四端共享，每个解析来源可限制手机、平板、电视、电脑适用范围。

## 操作流程
1. 添加实际解析服务，填写名称、接口地址、类型、JSON 地址字段和适用客户端。
2. 调整来源顺序、单来源超时、总时限及最大尝试数，按需要启用视频直链优先。
3. 开启轮询解析，输入影片来源地址执行真实解析测试。测试使用当前草稿，不改变已发布配置。
4. 保存草稿；发布配置后，各端播放接口才会使用新规则。
5. 所有来源失败会明确报错，不返回伪造成功。新安装和旧数据默认关闭，保持原有 CMS 播放地址行为。

## 管理接口
全部要求现有管理员登录；写操作还要求现有 Origin 与 X-Hetun-Client: studio 来源校验。返回值中 endpoint 仅供管理员配置，不会进入客户端播放结果。

| 方法 | 路径 | 输入与行为 |
| --- | --- | --- |
| GET | /api/admin/playback-resolvers | 返回 revision、draft、published、publishedVersion、publishedAt |
| PUT | /api/admin/playback-resolvers | {revision,config}，CAS 保存草稿；过期 revision 返回409 |
| POST | /api/admin/playback-resolvers/publish | {revision}，发布当前已保存草稿并递增版本 |
| POST | /api/admin/playback-resolvers/test | {config,url,platform}，实际发起解析请求，不保存；耗尽时HTTP200但ok=false |

配置结构示例（地址是格式说明，需要替换成自己的服务）：

```json
{"enabled":true,"directFirst":true,"timeoutSeconds":5,"totalSeconds":15,"maxAttempts":3,"sources":[{"id":"main","name":"主解析","kind":"json","endpoint":"https://your-parser.example/api?url={url}","urlField":"data.url","enabled":true,"platforms":["mobile","tablet","tv","web"]}]}
```

类型 json：GET 后需要HTTP200和JSON对象，urlField 支持 url、data.url 等点分对象路径，不支持数组。返回的字段须是HTTP(S)资源地址。
类型 redirect：GET 后需要301/302/303/307/308和有效 Location。不会继续请求跳转目标。
{url} 必须且仅能出现一次，放在查询参数值中；服务端自动编码影片地址。暂不支持返回HTML页面的 iframe/WebView 解析服务、POST签名协议或SDK播放器。

## APP 调用契约
继续使用 /api/v1/mobile/playback、/api/v1/tablet/playback、/api/v1/tv/playback、/api/v1/desktop/playback；兼容原有播放参数和认证方式。四端路由复用同一业务实现，desktop 在内部配置中对应 web。

服务端先执行原有 CMS 会员登录、影片与播放授权，再执行已发布解析规则。没有对游客开放任意 URL 解析接口。

成功响应增加 sourceId、kind、attempts、resolverVersion，保留 url、name。
解析失败返回HTTP502，code=PLAYBACK_RESOLUTION_FAILED，含attempts。每条尝试包含 id、name、ok、elapsedMs、message，不包含解析接口及密钥。

APP 收到地址后自行交给对应平台的播放器加载。若加载失败，应重新请求同一影片，将之前所有尝试过的 id（包括成功返回地址但加载失败的 sourceId）放入 excludeSourceIds。direct 表示已尝试CMS直链。服务端跳过这些来源并计入来源尝试上限，防止循环。配置变更导致旧ID不再存在时返回400，客户端需重启本次播放选择。

总时限约束单次服务端解析过程；多次客户端续试的累计总时限和取消由客户端自己管理。会员/CMS授权耗时也不计入解析时限。本次没有修改原生APP播放器，原生加载失败续试尚需客户端接入；服务端解析失败的自动切换已经实现。

## 限制与安全边界
- 来源最多32个，每次最多尝试1–10个，单来源1–10秒，总时限1–20秒且不小于单来源。
- enabled、客户端范围、排序、排除列表均参与调度。直链优先只按扩展名识别，不主动下载验证媒体。
- 使用既有CMS出站HTTP客户端和DNS/IP策略；默认阻止解析服务器指向内网，测试实例显式允许本机测试地址。
- 不向解析服务发送管理员Cookie、会员Cookie或Authorization；响应最大2MB，错误信息不透出上游敏感响应。
- 管理员对自己配置的第三方解析服务负责；解析时会将影片来源地址发送给该服务。
- 取得地址只表示解析成功，不表示媒体能被特定设备解码、投屏或播放。

## 验证依据
Go专项测试覆盖保存/发布分离、重启读取、CAS冲突、管理员鉴权、来源校验、实际HTTP请求顺序、URL编码、不转发会话、返回格式错误、2MB上限、跳转、超时/取消、排除续试、四端CMS播放授权和出站网络策略。
本地构建执行 vue-tsc、H5编译、完整go test、go vet，并生成平台二进制。
本次未配置真实外部解析供应商，未验证安卓/平板/电视/桌面真机媒体播放。
