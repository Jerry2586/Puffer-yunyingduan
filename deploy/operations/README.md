# 河豚影视中心运营服务端 0.2.0

这是可安装的独立运营网站：Linux、Node.js 22.x、Nginx、systemd。苹果CMS继续运行在原PHP站点，运营中心使用自己的域名及程序目录。服务端无需npm安装。Windows本地运行仅供验收，正式安装脚本用于Linux。

## 四个系统的职责

苹果CMS负责影片、分类、会员与订单；河豚运营中心连接CMS并发布手机/平板/电视/网页的运营配置；桌面主题中心制作并导出主题；打包器组合客户端源码、主题、品牌与服务地址生成客户端。运营后台不上传或安装主题源码包。已打包品牌与颜色不会被服务器默认主题覆盖。

运营配置包括导航与分类绑定、页面模块及排序、轮播内容与排版、功能开关、广告、公告、版本更新与排期。四端独立保存和发布，客户端每15秒拉取已发布配置。后台预览读取真实CMS内容，但外观使用内置参考主题；安装包采用打包器选择的主题。

## 独立目录

- 程序：/opt/hetun-operations/releases/0.2.0
- 配置：/etc/hetun-operations
- 状态、上传、审计与管理员凭据：/var/lib/hetun-operations
- 网站根目录：程序目录/frontend
- 服务日志：journalctl -u hetun-operations

不要把状态、签名私钥、管理员凭据、node_modules放进网站或源码目录。配置生成器禁止程序/状态/配置相互嵌套。

## 服务器安装

先准备独立域名DNS、HTTPS证书、Node22、Nginx与空闲API端口。解压发布包至程序目录，再执行预检查。下面地址均为示例，必须替换。

~~~sh
node install.mjs --domain ops.example.com --code /opt/hetun-operations/releases/0.2.0 --cert /etc/ssl/ops/fullchain.pem --key /etc/ssl/ops/privkey.pem --cmsRoots /www/wwwroot:/var/www
~~~

确认输出路径后，以root执行同一命令加 --apply。安装器校验发布文件、检查已有同名服务/配置冲突，不覆盖既有安装；创建独立服务用户与外部数据目录；安装自己的systemd服务及Nginx站点，检查Nginx后启动服务。它不更改苹果CMS站点。若新增站点校验失败，会撤下本次新增的站点及服务文件，保留配置供检查。

宝塔等环境使用实际参数 --nginxDir /www/server/panel/vhost/nginx --nginx /www/server/nginx/sbin/nginx；Node位置可用 --node 指定。脚本不会自动下载Node、申请证书或修改DNS。

也可以只生成配置并人工安装：

~~~sh
node configure.mjs --domain ops.example.com --code /opt/hetun-operations/releases/0.2.0 --cert /etc/ssl/ops/fullchain.pem --key /etc/ssl/ops/privkey.pem
~~~

## 首次安装向导

1. 服务首次启动，在外部数据目录生成随机管理员账号。服务器管理员私下读取 admin-credentials.json 中 username/password，进入运营网站登录；不要把文件发布到网页或聊天。
2. 安装向导扫描 --cmsRoots 指定目录，仅读取CMS公开站点元数据，不读取数据库密码、不执行PHP。对发现的站点实际请求分类与影片接口。唯一检测成功的站点自动选中；多个站点需要选择；也支持手工填写地址。
3. 分类及影片检测成功后设置新管理员账号与密码，完成安装。初始密码立即失效，磁盘凭据只保存盐与密码散列。
4. 生产运营API需要有效影视中心授权。配置 HETUN_LICENSE_URL 与 HETUN_LICENSE_PUBLIC_KEY_FILE，可信公钥在外部目录，由服务用户可读。云端授权签发服务需要另行提供；当前安装包没有伪造生产授权。安装向导在授权激活前可用。
5. 在“内容连接”检查并保存CMS，在四端运营配置里完成导航、模块、开关、广告、公告与更新后发布。把运营中心HTTPS地址填入打包器作为客户端后台地址。

## CMS连接与会员适配器

默认拒绝访问私有地址。与CMS同服务器而需要使用127.0.0.1或内网地址时，明确指定 --allowPrivate 1 --cmsBase http://127.0.0.1:8099，或在外部operations.env设置。PHP站点仍需正确的Host/端口；不根据文件存在就宣称对接成功。目录自动扫描受服务用户读取权限限制，权限不足可手工输入CMS公开域名。

分类和影片接口可直接连接；会员登录、付费权限播放、套餐与订单需要河豚PHP适配器。明确选择站点后运行：

~~~sh
node install-cms-adapter.mjs --cmsRoot /www/wwwroot/your-cms-site
~~~

仅写入 application/index/controller/Hetun.php，不覆盖不同版本的已有文件。相同文件重复运行不变。随后实际检测会员接口并用测试账号验证。适配器与CMS版本、支付渠道和播放规则需要真实站点联调；内容检测成功不代表会员支付已经验收。电视等端关闭会员功能后，服务端拒绝该端会员接口；客户端平台标识由X-Hetun-Platform传递，这属于产品开关而非权限安全凭证。

## 客户端接口

- GET /api/health：存活状态、版本、产品、CMS是否配置。
- GET /api/client/bootstrap?platform=mobile（或 /api/runtime）：已发布配置、版本、更新时间、15秒轮询建议；支持mobile/tablet/tv/web。
- GET /api/catalog/categories、GET /api/catalog：真实分类、影片/搜索/分页；播放地址通过权限接口获得。
- POST /api/playback、/api/member/*：PHP适配器支持的权限播放与会员功能。
- /api/admin/*：管理员会话，写入要求正确Origin及X-Hetun-Client: studio，客户端不能调用运营写接口。

网页客户端在其他域名时通过 --clientOrigins https://client.example.com 配置确切HTTPS来源。原生客户端不需要浏览器CORS。运行配置不包含打包品牌或颜色。草稿不会发给客户端，发布后才同步；多人同时修改时版本冲突返回409。

## 更新、备份与验收边界

备份外部配置、状态及CMS适配器。新版本解压到新程序目录，保持外部数据目录不变；核对校验清单，再更新systemd程序路径与Nginx frontend路径，检测后切换。首次迁移旧运营状态会保留外部快照。

本地集成测试使用明确标记的隔离CMS/签名授权夹具，验证真实HTTP流程，不代表用户生产CMS、支付渠道、PHP宿主或Linux正式安装已验收。一起看、弹幕共享等实时多人服务未由本安装任务补齐。实际CMS地址、服务器与授权云端未提供时，交付的是可运行安装包及本地验收结果。

## 本地构建

通过外部UniAppTools环境执行 node deploy/operations/build.mjs。依赖、编译、测试证据、运行数据与发布包全部在源码之外。
