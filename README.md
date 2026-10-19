# 河豚运营中心

独立运营服务端使用 Go，内嵌现有 Vue/uni-app 管理界面。正式版本为 **0.3.1**，沿用后台外观。服务器安装后不需要 Node.js 或 Go 编译器。

苹果 CMS 提供内容、会员、订单和播放权限；运营端管理手机、平板、电视、网页的内容栏目、轮播、广告、公告、功能开关及更新配置。已删除商业授权、激活页面、云 Key 校验和许可客户端。管理员登录、请求来源检查、会员购买与播放权限继续保留。

## 固定职责

1. 离线主题中心制作 hetun-skeleton v2 主题包。
2. 开发者用原生源码编译实现协议的基础 APK。
3. 本地打包器组合协议基础 APK、主题、运营地址并签名。
4. Go 运营端连接 CMS，保存、发布及回退各设备的运营内容。

运营端不接收主题源码包、不更换客户端品牌/颜色/布局、不运行云打包。统一接口不表示任意 APK 都能适配；基础 APK 必须实现协议。iOS 内购、IPA/Windows 二进制二次打包、复杂主题效果仍需独立实现与验收。

## 目录

| 目录 | 用途 |
|---|---|
| go-server/ | Go HTTP 服务、MySQL/本机文件存储、CMS 与会员接口、Go 回归测试 |
| src/ | 现有管理界面及运营内容预览组件 |
| deploy/go/ | 独立程序构建、宝塔部署模板、成品测试、旧 Node 数据迁移工具 |
| server/ | 旧 Node 兼容实现与迁移验证辅助；CMS PHP 适配器源文件 |
| deploy/operations/ | 旧部署辅助工具及兼容构建/测试入口；新构建转至 Go |

依赖、状态、凭据、日志与构建产物放在源码目录之外。

## 本地构建

构建机需要 Go 1.25 或更新版本、Node.js 22、锁定 npm 依赖及 tar。生产服务器只需解压的程序和可连接的 MySQL，首次网页安装自动生成配置。

先把 package.json 与 package-lock.json 复制到源码之外的依赖目录，在那里执行 npm ci；设置 HETUN_UNI_DEPS 为该目录中的 node_modules，HETUN_OPERATIONS_BUILD_ROOT 为外部构建目录。

~~~powershell
$env:HETUN_UNI_DEPS = 'C:/HetunBuild/dependencies/node_modules'
$env:HETUN_OPERATIONS_BUILD_ROOT = 'C:/HetunBuild/operations'
node deploy/go/build.mjs
$env:HETUN_OPERATIONS_BUILD_INFO = 'C:/HetunBuild/operations/latest-build.json'
node --test deploy/go/integration.test.mjs deploy/operations/contract.test.mjs
~~~

Linux/macOS 使用同名环境变量和命令，改为本机绝对路径。构建依次执行前端类型检查、H5 编译、Go 测试和 vet，然后嵌入真实前端，生成 Linux amd64/arm64 以及 Windows amd64 程序、SHA-256 文件清单和压缩包。latest-build.json 记录实际产物路径。源码中的 go-server/web 仅为未构建提示页，正式查看请运行已嵌入前端的构建程序。

## 宝塔部署

详细步骤见 [Go 部署说明](deploy/go/README.md)。安装包解压只有一个程序：创建空目录和独立空 MySQL 数据库，在宝塔 Go 项目中选择程序、端口填 8787 并启动；日志显示持久随机后台入口。打开该地址，在网页填写数据库和管理员即可安装。程序自动生成配置及数据目录，登录后再接入 CMS。

正式环境使用 MySQL；本机查看可用 file 模式。MySQL 业务状态与 dataDir/assets 都需要备份。旧版本用 deploy/go/migrate-node-state.mjs 转换业务状态，导入空库，素材另行复制。迁移不携带云授权。

## 协议与验收

公开 bootstrap 契约为 hetun-operations-v1，保留 /api/client/bootstrap、/api/bootstrap、/api/runtime。按 mobile/tablet/tv/web 独立发布。运营白名单不传递主题视觉字段，草稿版本冲突返回 409；发布历史最多 20 份，回退生成递增的新版本。

Go 回归涵盖初始化、管理员、请求来源、CMS HTTP、四端运营配置、发布与回退、持久化、会员、播放及支付表单过滤。成品测试核对归档和实际进程；CI 运行这些检查，不自动发布或部署。隔离 CMS 夹具不是生产 CMS；CI 额外使用真实隔离 MySQL 验证首次安装和重启。生产会员支付、CMS、宝塔实站与真机仍需验收；发布安装包不代表已经部署生产服务。
