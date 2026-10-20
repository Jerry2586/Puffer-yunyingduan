# 广告中心接口与验收范围

协议：hetun-advertising-v1。platform 为 mobile、tablet、tv、web；四端草稿、发布版本、频控与记录独立。管理员接口沿用后台登录认证。客户端不得持有管理员凭据。

|方法|路径|用途|
|---|---|---|
|GET|/api/admin/ads?platform=mobile|读取草稿、发布版本、统计与事件|
|PUT|/api/admin/ads?platform=mobile|保存 {version,config}，版本冲突返回409|
|POST|/api/admin/ads/publish?platform=mobile|发布 {version}|
|POST|/api/admin/ads/diagnose?platform=mobile|只读检测 {os:"android",slot:"splash"}|
|GET|/api/ads/config?platform=mobile|公开已发布广告位与调用地址|
|POST|/api/ads/decision?platform=mobile|请求真实投放决策|
|POST|/api/ads/events?platform=mobile|上报决策对应事件|
|GET|/api/ads/balance?platform=mobile|会员认证后读取运营积分|
|GET|/api/ads/callback/admob|接收官方签名查询串，禁止重排或重编码|

决策请求示例：{ "slot":"splash", "os":"android", "deviceId":"client-device-00001", "adapters":["admob"], "consent":true }。只有客户端确实编译并初始化适配器且取得所需同意后才声明 adapters 与 consent。无填充按实际原因返回，不能强制展示。成功响应提供 decisionId、ad、expiresAt、eventExpiresAt。事件请求包含 decisionId、type 和可选 code；事件去重并校验顺序。激励事件本身不发奖。

广告位：splash、home_interstitial、home_banner、feed、search_banner、mine_banner、playback、rewarded。排期保存为 UTC，编辑器显示本地时间；报表以 UTC 日期统计。自定义素材使用现有管理员素材上传接口，最大12 MiB，本地保存后发布时再次校验存在性。

AdMob 激励回调按原始查询串执行 ECDSA SHA256 验签，校验事务、用户、广告单元、决策与时效，重复事务不重复发奖。客户端使用响应 ssv.customData / userId 配置原生SDK。积分是运营端独立账本，未同步苹果CMS user_points。未知供应商不允许启用，电视与电脑未实现AdMob原生适配。

本地验收：全量Go测试、vet、Vue类型检查、H5与三个目标编译通过；测试覆盖权限、版本冲突、四端隔离、重启持久、排期、频控、事件去重、兜底、伪造签名及重复回调。验签测试使用生成的签名密钥，不代表实际Google广告回调验收。浏览器验证原生文件选择、日期控件及真实配置诊断返回。

未完成外部验收：原生SDK展示、广告账户真实填充、Google生产回调、真实CMS会员奖励链路。当前本地CMS未配置。收入与渠道结算未接入，不展示虚构收益。

容量边界：当前采用单进程全状态CAS持久化；每端最多5000个有效决策、10000个频控标识、最近500条事件、90天统计，奖励账本达到20000条后拒绝继续发奖；不适合作为高并发广告分析服务。设备标识哈希存储，不能替代客户端可信身份或渠道反作弊。
