# 旧版部署目录

当前 Go 部署方式请查看 [deploy/go/README.md](../go/README.md)。此目录中的 build.mjs 与 integration.test.mjs 转发到新版入口；旧 configure/install 工具只供维护既有 Node 版本，新的 Linux 包不携带 Node 服务。新部署使用 Go 程序、config.json、MySQL 和宝塔 HTTPS 反向代理，不再使用旧环境变量或任何商业授权配置。
