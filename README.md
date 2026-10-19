# 河豚运营中心

独立运营服务端使用 Go，内嵌现有 Vue/uni-app 管理界面。正式版本为 **0.3.1**，沿用后台外观。服务器安装后不需要 Node.js 或 Go 编译器。

苹果 CMS 提供内容、会员、订单和播放权限；运营端管理手机、平板、电视、网页的内容栏目、轮播、广告、公告、功能开关及更新配置。已删除商业授权、激活页面、云 Key 校验和许可客户端。管理员登录、请求来源检查、会员购买与播放权限继续保留。

## 宝塔安装教程（从这里开始）

### 1. 下载安装包

打开 [正式版 v0.3.1 下载页面](https://github.com/Jerry2586/Puffer-yunyingduan/releases/tag/v0.3.1)，展开 **Assets**，按服务器架构选择：

| 服务器 | 下载文件 |
|---|---|
| 常见 Intel / AMD（x86_64）Linux 服务器 | `Hetun-Operations-0.3.1-linux-amd64.tar.gz` |
| ARM64（aarch64）Linux 服务器 | `Hetun-Operations-0.3.1-linux-arm64.tar.gz` |

不要下载 Source code；那是开发源码。安装包解压后只有一个 **hetun-operations** 可执行文件，网页、安装向导和接口都已内置。

### 2. 创建文件夹和数据库

1. 在宝塔文件管理中新建空文件夹，例如 `/www/wwwroot/hetun`，上传安装包并解压。
2. 在宝塔「数据库」中新建一个独立的空 MySQL 数据库，记下数据库名、用户名和密码。**无需导入 SQL 文件。**
3. 确保程序文件有执行权限，运行用户 **www** 对程序所在目录有读写权限。

### 3. 在宝塔添加 Go 项目

进入宝塔 **「Go 项目 → 添加项目」**，填写：

| 设置项 | 填写内容 |
|---|---|
| 项目名称 | `hetun`（也可以自定义） |
| 可执行文件 | 选择刚解压的 `hetun-operations` |
| 项目端口 | **8787** |
| 执行命令 | 使用默认程序命令，无需添加参数 |
| 环境变量 | 留空 |
| 运行用户 | **www** |
| 开机自启 | 勾选 |

保存并启动项目。服务器上无需额外安装 Node.js、Go 编译器或手写配置文件。

### 4. 从启动日志打开安装页面

打开项目的运行日志，找到以下内容（随机路径以你自己的日志为准）：

~~~text
端口：8787
后台访问路径：/8f2b6d1a9c4e7035
后台地址示例：http://服务器IP:8787/8f2b6d1a9c4e7035
启动成功
~~~

用浏览器打开日志中的后台地址，把「服务器IP」换成自己的服务器公网 IP。通过 IP 直接访问时，在服务器防火墙和云服务器安全组中放行 **TCP 8787**。也可以在宝塔项目中绑定域名并配置 HTTPS。

**随机的是后台路径，默认端口固定为 8787。** 每台首次安装会生成自己的后台路径，重启和升级后保持不变；忘记地址时查看启动日志即可。

### 5. 填写数据库并完成安装

1. 打开的网页就是安装向导，检查自动填写的网站来源地址。
2. 填写自己的 MySQL 地址、端口、数据库名、用户名和密码。
3. 设置自己的管理员账号和密码，测试数据库连接后完成安装。
4. 在同一个随机后台地址登录，随后接入自己的苹果 CMS。

这里是安装初始化，**不需要商业授权、Key 或激活码**。数据库账号需要建表和写入权限。

程序运行后会在可执行文件旁自动生成 `config.json` 和 `data` 目录，用于保存数据库连接、随机入口和运行数据。不要删除它们，也不要把程序目录作为可公开下载文件的静态网站目录。

### 升级与常见问题

- **升级：**停止程序，备份 MySQL、`config.json` 和完整 `data` 目录，只替换可执行文件后重新启动。不要删除配置再对旧数据库重新安装。
- **8787 被占用：**启动命令增加 `--listen 0.0.0.0:8788`，同时把宝塔项目端口和防火墙放行端口改为 8788。首次安装后监听地址保存到配置。
- **无法保存配置：**检查 www 用户对程序所在目录的写权限。
- **安装后根地址打不开后台：**使用日志中的完整随机路径；安装完成后，根路径和 `/index.html` 不提供后台页面。
- **客户端接入：**客户端使用固定的 `/api/` 接口，运营地址无需拼接后台随机路径。

更多 CMS 接入、迁移和高级配置见 [完整 Go 部署说明](deploy/go/README.md)。

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

## 协议与验收

公开 bootstrap 契约为 hetun-operations-v1，保留 /api/client/bootstrap、/api/bootstrap、/api/runtime。按 mobile/tablet/tv/web 独立发布。运营白名单不传递主题视觉字段，草稿版本冲突返回 409；发布历史最多 20 份，回退生成递增的新版本。

Go 回归涵盖初始化、管理员、请求来源、CMS HTTP、四端运营配置、发布与回退、持久化、会员、播放及支付表单过滤。成品测试核对归档和实际进程；CI 运行这些检查，不自动发布或部署。隔离 CMS 夹具不是生产 CMS；CI 额外使用真实隔离 MySQL 验证首次安装和重启。生产会员支付、CMS、宝塔实站与真机仍需验收；发布安装包不代表已经部署生产服务。
