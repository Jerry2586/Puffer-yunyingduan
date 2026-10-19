# 河豚运营中心 0.3.0 安装说明

正式服务为 Go 独立程序，内嵌后台网页。服务器不需要安装 Node.js、npm 或 Go 编译器；需要 MySQL 8.0 和宝塔 Nginx。商业授权、激活码及云 Key 校验已移除，管理员登录保留。

## 宝塔首次安装（推荐）

1. 普通 Intel/AMD 64 位 Linux 服务器下载 linux-amd64 包；ARM 服务器使用 linux-arm64 包。请核对 SHA256SUMS.txt。
2. 宝塔文件管理中创建空目录，例如 /www/server/hetun-operations。上传对应压缩包并解压，程序直接位于该目录中。不要把程序目录设为公开静态网站根目录。
3. 宝塔「数据库」创建一个独立的空 MySQL 数据库，记下库名、用户名、密码。无需导入 SQL，不要共用已有 CMS 数据库。
4. 给 hetun-operations 文件执行权限；确保运行账号可以读写程序目录。宝塔使用 Go 项目或进程管理器运行该可执行文件，工作目录设为程序所在目录，开启守护和开机启动。启动命令为 ./hetun-operations；无需编译。默认只监听 127.0.0.1:8787。
5. 宝塔创建独立域名网站并配置 HTTPS，反向代理到 http://127.0.0.1:8787，保留 Host 请求头。可使用包内 nginx-location.conf，替换冲突的 location /，保留证书配置。此项目的所有请求都交给 Go 服务，不公开程序目录中的文件。
6. 在项目启动日志中找到 /install/ 后面的临时安装路径，打开「https://你的域名/install/日志中的随机字符串」。也可以从程序目录 data/install-token.txt 读取字符串。该路径是首次安装入口，无需购买或输入授权码；不要公开分享。
7. 填写后台 HTTPS 地址、数据库连接和自选管理员账号密码。先测试连接，再完成安装。程序会建表、保存 config.json、创建 data 目录并关闭安装入口，随后进入后台登录页。安装不要求先连接 CMS。
8. 用刚创建的管理员登录，在后台连接设置中接入自己的苹果 CMS，然后保存并发布运营配置。

若端口占用，首次启动使用 ./hetun-operations --listen 127.0.0.1:8788，并把反向代理端口改为 8788；安装完成后端口保存到 config.json。配置文件默认位于可执行文件旁，路径有空格时由进程管理器正确引用。也可通过 --config /绝对路径/config.json 指定配置路径，其父目录必须可写。程序不会用安装向导覆盖已存在的配置或已初始化的数据库。

连接成功仅代表账号可连接数据库；完成安装还需要建表和写入权限。保存配置失败时修复目录权限，使用相同管理员信息重试。安装入口在未完成时重启保持有效，完成后失效。

## CMS 与客户端

CMS 采集接口用于分类和影片列表。会员、订单及播放权限需要兼容接口及包内 cms-adapter/Hetun.php。先备份自己的 CMS，再将文件安装到 application/index/controller/Hetun.php；已有同名文件时先比对。实际验证 index.php/hetun/token 和 index.php/hetun/play，以及会员、收费影片、支付回调。不同 CMS 魔改版本需单独验收。

客户端固定接口：GET /api/bootstrap?platform=mobile&os=android&version=1.0.0；platform 支持 mobile/tablet/tv/web。运营协议 hetun-operations-v1，主题协议 hetun-runtime-v1。客户端必须实现协议，由本地打包器写入运营地址。运营端不进行云打包、不替换已打包的品牌、配色和布局。

## 升级与备份

停止当前进程，备份 MySQL、config.json 和整个 data 目录，保留旧程序。解压新版到临时目录，只替换可执行文件，保留原 config.json、data、数据库及运行账号权限，再启动服务。不要删除配置后对旧库重新安装。

业务状态可通过 ./hetun-operations --backup /绝对路径/backup.json 导出，素材仍需单独备份。仅向尚未初始化的空数据库使用 --import-state /绝对路径/backup.json；程序拒绝覆盖现有状态。旧 Node 版本先用源码中的 deploy/go/migrate-node-state.mjs 迁移状态，再复制素材。迁移前保留旧服务和完整备份。

失败回退需停止新程序，恢复匹配的旧程序、数据库及数据目录。后台发布历史回退只恢复运营配置，不代替系统备份。配置含数据库凭据，备份含密码摘要，应限制文件权限。

## 其他运行方式

不使用宝塔进程管理时，可用包内 systemd 模板。该模板为预配置部署：先按 config.example.json 创建 /etc/hetun-operations/config.json、hetun-ops 账号和可写的 /var/lib/hetun-operations，再按模板安装到 /etc/systemd/system/，执行 systemctl daemon-reload 和 systemctl enable --now hetun-operations。不要同时使用两套进程管理启动同一实例。

Windows 包适合本地查看；可连接测试 MySQL 走同样安装流程，或按 config.example.json 手动设置 storage=file、删除 mysqlDSN，listen/publicOrigin 使用同一 127.0.0.1 端口，dataDir 使用绝对路径。file 模式仅用于本机，正式部署使用 MySQL。

正式发布包含 Go 单元测试、独立成品归档验证和 Linux 隔离 MySQL 安装测试；这不代替你的宝塔服务器、生产 CMS、支付和安卓真机验收。iOS 内购与二进制二次打包仍需独立实现。
