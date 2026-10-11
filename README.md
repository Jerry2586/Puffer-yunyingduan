# 河豚运营中心

河豚影视系统的独立运营服务端，当前运营安装包版本为 **0.2.0**。苹果 CMS 提供内容、分类、会员与订单，运营中心负责连接 CMS 并发布客户端运行配置。

## 系统流程

1. 在服务器安装苹果 CMS。
2. 将运营中心安装到独立目录和域名，通过首次安装向导检测并连接 CMS。
3. 桌面主题中心制作主题包，交给河豚打包器。
4. 打包器组合客户端源码、主题、品牌、签名与运营中心地址。
5. 运营中心分别保存和发布手机、平板、电视、网页的导航、模块、轮播、功能开关、广告、公告和更新配置。

运营中心不安装主题源码包。已打包客户端的品牌与配色由所选主题决定，运营预览使用内置参考外观；公开客户端接口只返回已发布运营配置。

## 源码目录

| 目录 | 用途 |
| --- | --- |
| `server/` | Node.js 服务端、CMS 连接、运营配置、会员接口和授权校验 |
| `src/` | 运营工作台和共用 uni-app 组件，包含运营预览所需的客户端界面 |
| `deploy/operations/` | 运营端构建、Linux 安装、CMS 适配器与集成测试 |
| `.github/workflows/` | 源码推送后的构建和集成检查 |

`src/` 的共用主题组件由工作台与预览引用，因此保留在此仓库。打包器和桌面主题中心的软件交付分别在它们自己的仓库。依赖、Git 数据库、凭据、日志、运行数据、构建和测试产物应放在源码目录外。

## 从源码构建

需要 Node.js 22.x、npm 10.x 和可用的 `tar`。根目录 `package.json` 为共用 uni-app 依赖清单；运营包版本由 `deploy/operations/build.mjs` 定义。

以下路径仅为示例，替换成自己的绝对路径，`HETUN_UNI_DEPS` 与 `HETUN_OPERATIONS_BUILD_ROOT` 必须在源码目录外。

### Linux / macOS

```sh
mkdir -p /tmp/hetun-ops-dependencies
cp package.json package-lock.json /tmp/hetun-ops-dependencies/
npm ci --prefix /tmp/hetun-ops-dependencies --no-audit --no-fund
export HETUN_UNI_DEPS=/tmp/hetun-ops-dependencies/node_modules
export HETUN_OPERATIONS_BUILD_ROOT=/tmp/hetun-ops-build
node deploy/operations/build.mjs
export HETUN_OPERATIONS_BUILD_INFO="$HETUN_OPERATIONS_BUILD_ROOT/latest-build.json"
node --test deploy/operations/integration.test.mjs
```

### Windows PowerShell

```powershell
$deps = Join-Path $env:LOCALAPPDATA "HetunBuild/dependencies"
$build = Join-Path $env:LOCALAPPDATA "HetunBuild/operations"
New-Item -ItemType Directory -Path $deps -Force | Out-Null
Copy-Item -LiteralPath package.json,package-lock.json -Destination $deps
npm ci --prefix $deps --no-audit --no-fund
$env:HETUN_UNI_DEPS = Join-Path $deps "node_modules"
$env:HETUN_OPERATIONS_BUILD_ROOT = $build
node deploy/operations/build.mjs
$env:HETUN_OPERATIONS_BUILD_INFO = Join-Path $build "latest-build.json"
node --test deploy/operations/integration.test.mjs
```

构建会执行类型检查和运营 H5 编译，生成服务端、前端、安装脚本、SHA-256 文件清单及 Linux `.tar.gz` 归档。构建信息写入外部 `latest-build.json`，不写入源码目录。已生成的运营服务端不需要安装 npm 依赖。

## 安装与运行

完整安装说明见 [deploy/operations/README.md](deploy/operations/README.md)，包含独立域名、Nginx、systemd、首次管理员初始化、CMS 自动发现、授权和会员适配器。源码 `server/index.mjs` 的运营运行模式为 `HETUN_PRODUCT_MODE=operations-center`；`HETUN_STATE_DIR` 必须是项目外绝对路径。正式安装建议使用构建包的 `install.mjs`，先生成计划，再按部署文档明确执行。

管理员初始随机凭据由服务首次启动写入外部状态目录的 `admin-credentials.json`，安装时必须修改。不要上传该文件或将其暴露给网页。

生产运营 API 需要有效影视中心签名授权；云端签发服务和可信公钥需另行配置。明确指定的 loopback 本地验收模式不能替代生产授权。

## 自动检查及验收范围

GitHub Actions 在 `main` 推送、拉取请求或手工运行时，外置安装锁定依赖、构建运营包并执行五组隔离集成测试，涵盖安装、鉴权、CMS 内容连接、四端配置发布、冲突与持久化。工作流不创建标签或 Release，也不部署服务器。

隔离测试会启动本机 HTTP CMS 与签名授权测试服务，结束后关闭；其结果不代表生产 CMS、PHP 支付、Linux 实站安装或云端授权已经验收。一起看、弹幕等多人实时服务尚不属于本运营安装包的已验收范围。本仓库上传的是源码，不包含 APK、IPA、管理员凭据或运行数据。
