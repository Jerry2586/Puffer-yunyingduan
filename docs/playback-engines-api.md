# 四端播放内核管理接口与客户端接入

本模块在轮询解析播放器页面的调度策略上方。一个苹果 CMS 对接一个运营中心实例；四端共用 CMS 与会员授权，内核策略按设备类别及操作系统隔离。该模块不复制 CMS 站点配置、不改主题外观、不在线安装播放器 SDK。

## 已实现与验证边界

- Go 持久化草稿、CAS 版本、独立发布；七个系统配置：手机 Android/iOS，平板 Android/iOS（界面称 iPadOS），电视 Android，电脑 Windows/浏览器。
- 基础包能力 JSON 校验、客户端能力协商、公开配置元数据、CMS 授权后的播放策略返回、带签名凭据的客户端反馈接收。
- 候选列表不是当前安装包的 SDK 清单。登记信息来源为 admin_declared；SDK 授权值是基础包声明，运营服务不据此声称完成服务商验签。
- 反馈来源为 playback_client_report；prepared 表示客户端报告内核准备就绪，不证明完整播放、所有编码兼容或第三方认证。没有报告时显示未检测，超过24小时或策略版本变化则作为历史反馈。
- 当前 Android 基础源码仍使用 VideoView，尚未实现本协议的适配器；iOS/iPadOS、Windows 原生 SDK、打包器能力清单导出及真机切换尚未接入。不能声明已完成四端原生播放验证。

## 状态与发布

基础包未登记 → 可编辑并保存设计草稿 → 登记真实能力 → 发布校验 → 客户端协商 → CMS 授权播放 → 客户端实际执行/反馈。

内核配置与解析来源分别保存发布，分别使用 revision 和 publishedVersion。保存草稿不会改变已发布策略。PUT 与 publish 必须携带刚读到的内核 revision；过期返回409。配置反馈写入不会增加内核 revision。

启用的配置必须具备主内核，并且其基础包声明包含所选主备内核；商业内核必须声明有效授权。自动切换要求备用内核、非空条件和至少一次切换额度；手动切换需要备用内核。所需 feature 必须已经在真实客户端实现。停用配置允许保留计划中的候选，供后续开发。

## 管理接口

全部要求既有运营管理员会话；写请求继续要求可信 Origin 和 X-Hetun-Client: studio。接口不包含 SDK License Key、CMS 密码或解析来源密钥。

|方法|路径|输入/输出|
|---|---|---|
|GET|/api/admin/playback-engines|settings、候选 engines、系统 targets 及数据来源|
|PUT|/api/admin/playback-engines|{revision,config:{profiles:[...]}}，保存草稿|
|POST|/api/admin/playback-engines/publish|{revision}，校验并发布草稿|
|POST|/api/admin/playback-engines/inspect|{build}，校验能力声明；不写配置，不产生设备检测结果|

GET settings 包含 revision、publishedVersion、publishedAt、draft、published、observations。observations 最多64条，来自已授权播放签发的反馈凭据。后台登记能力和客户端反馈应分开呈现。

每个 profile 字段：platform、os、enabled、build、primary、backup、manualSwitch、autoSwitch、switchOn、resumePosition、maxSwitches、maxAttempts、sessionSeconds。

范围：maxSwitches 为0–2；maxAttempts 为1–20；sessionSeconds 为15–300。switchOn 支持 initialization、decode、stall，不接受重复项。原解析来源的网络超时、整体解析预算及 maxAttempts 继续独立生效。

## 基础包及客户端能力声明

协议为 hetun-playback-build-v1。管理导入使用 build；客户端协商使用 client；授权播放使用 playbackClient。三者采用相同结构，但来源含义不同。生产者应从真实构建及实际适配器能力生成，不能把下例复制成已实现证明。

示例仅表示协议结构，包名/版本/功能必须替换为真实已实现值：

```json
{
  "contract":"hetun-playback-build-v1",
  "platform":"mobile",
  "os":"android",
  "appId":"com.example.actualclient",
  "appVersion":"1.0.0",
  "engines":[{"id":"android-system","version":"android-api-34","license":"not_required"}],
  "features":["engine-policy","playback-report","resume-position"]
}
```

platform 为 mobile、tablet、tv、web；电脑请求路径为 desktop，内部数据仍为 web。iPadOS 的 os 使用 ios，由 tablet 路径区分。appVersion 使用三段数字，appId 绑定实际包名/Bundle ID。策略按精确应用版本匹配，升级基础包后需更新登记再发布。

候选 ID：android-system、media3、avplayer、libvlc、html5、hlsjs、aliyun、tencent、volcengine。候选系统以管理接口 engines[].systems 为准；这只是当前接入范围，不代表所有 SDK 的完整官方平台能力或电视认证。

license：开源/系统候选使用 not_required；商业候选使用 unknown、valid、invalid。valid 只是 SDK/构建方的授权声明，不代替第三方授权验证，不在该文件保存真实 License Key。

features：

|能力|必须真实实现|
|---|---|
|engine-policy|识别并执行已协商的内核策略|
|playback-report|处理真实内核回调，提交已授权播放凭据|
|engine-fallback|按条件切换内核、去重组合、遵守次数和时间预算|
|manual-switch|播放器允许用户在可用内核间切换|
|resume-position|保存播放位置并在支持 seek 的新内核恢复|

发布要求 engine-policy 与 playback-report；其他 feature 按开启的选项要求。现有 Android 基础包不能仅修改声明就声称完成以上能力。

## 客户端接口

前缀为 /api/v1/mobile、/api/v1/tablet、/api/v1/tv、/api/v1/desktop。所有路径使用同一处理器；路径与 platform/X-Hetun-Platform 冲突返回400。

|方法|后缀|行为|
|---|---|---|
|GET|playback/config?os=ios|返回已发布策略元数据、版本和协商路径，不返回媒体地址|
|POST|playback/negotiate|{client:能力声明}；仅协商，不代表播放授权，不签发反馈凭据|
|POST|playback|沿用 id/sid/nid/会员会话，可增加 playbackClient 和已有 excludeSourceIds|
|POST|playback/report|提交由已授权播放响应签发的 ticket、engine、outcome、reason、license|

bootstrap 新增 playback 元数据。该字段在取得已有 bootstrap 缓存后按当前内核发布状态附加，因此内核发布不依赖清理旧 bootstrap 缓存。字段 enabled 只说明策略是否启用，不代表客户端已支持。

协商取“已发布基础包声明 ∩ 客户端适配器报告”中可用的所选内核；应用ID/版本必须一致，主内核缺失、自动或手动切换需要的备用内核缺失、或 feature 不全返回409。不会返回任意其他候选作为可执行内核。

授权播放先走既有 CMS 权益校验，然后协商内核，再解析资源；响应新增 enginePolicy。status=negotiated 时还包含 engineReportTicket，有效期10分钟。协商或 bootstrap 接口无法生成 ticket。旧客户端不提交 playbackClient 时，沿用已授权媒体地址并明确返回 legacy_client；后台设置不能给它安装新 SDK。

已协商 enginePolicy 包含 primary、backup、manualSwitch、autoSwitch、switchOn、resumePosition、maxSwitches、maxAttempts、sessionSeconds、reportPath。SDK 初始化、渲染、解码及切换由客户端适配器执行；Go 服务不是媒体解码器。

## 客户端播放恢复流程

1. 使用策略指定的主内核加载已授权地址，记录本次播放开始时间及已尝试的 sourceId+engineId 组合。
2. 内核初始化/解码/持续卡顿符合 switchOn 时，在备用内核已集成且可用、次数/时间预算允许时切换；根据 resumePosition 恢复进度。
3. 地址失效/解析失败归入来源失败，提交 excludeSourceIds 再请求 playback；不能把权限拒绝或商业授权错误当成可绕过的资源失败。
4. 每个组合最多一次；来源轮询和内核轮询共用客户端本次播放的 maxAttempts 与 sessionSeconds 上限；只在预算内恢复，达到上限时明确失败。
5. 解析服务每请求的超时/次数上限由 Go 执行；跨请求、内核与来源组合的整次播放预算由客户端适配器负责。现有客户端未接入时，不能声称已端到端强制该预算。

## 播放反馈与可信程度

```json
{"ticket":"授权播放返回的凭据","engine":"avplayer","outcome":"prepared","reason":"","license":"not_required"}
```

outcome 支持 prepared/failed；失败 reason 必须为 initialization、decode、stall、network、license 或 other。成功 reason 为空；商业内核未报告有效授权时不能提交 prepared。engine 必须属于该凭据签发时协商出的主备集合。反馈不接收媒体URL或任意异常文本，避免误存授权资源地址和密钥。

凭据使用当前管理员凭据摘要与当前 CMS 站点绑定的 HMAC-SHA256 签名，包含随机ID、平台、应用、版本、可报告内核、策略版本与到期时间。改密码/更换CMS、跨端、到期、篡改或策略重新发布后，旧凭据不能作为当前检测。重复相同反馈在保留记录内幂等，复用同凭据改结果返回409。

该凭据证明报告者拿到过该范围的授权播放响应，不证明设备硬件真实性或商业SDK服务商验签；响应明确 deviceVerified:false。原会员权益与 CMS 播放授权保持唯一业务边界。

## SDK 接入参考

官方依据（只作为接入资料，不代表已在工程集成）：

- Android Media3：https://developer.android.com/media/media3/exoplayer/hello-world
- Apple AVPlayer：https://developer.apple.com/documentation/avfoundation/avplayer
- hls.js：https://github.com/video-dev/hls.js
- libVLC：https://www.videolan.org/vlc/libvlc.html
- 阿里云 SDK 授权：https://www.alibabacloud.com/help/en/apsara-video-sdk/license-authorization-and-management
- 腾讯云播放器：https://cloud.tencent.com/document/product/881/20193
- 火山引擎播放器：https://docs.volcengine.com/docs/video_on_demand/SDKOverview-1
