<div align="center">

<img src="cmd/desktop/build/appicon.png" width="110" alt="Thin Client Cloud Desktop"/>

# 瘦客户端云桌面系统

**基于 Go + Wails 的跨平台瘦客户端 · 一个入口，连接所有云桌面**

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Wails](https://img.shields.io/badge/Wails-v2-DC382D?logo=&logoColor=white)](https://wails.io)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=white)](https://react.dev)
[![Ant Design](https://img.shields.io/badge/Ant_Design-5-1677FF?logo=antdesign&logoColor=white)](https://ant.design)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)](#)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**由 [山东飞力达软件有限公司](https://sdfeilida.com) 开源维护**

</div>

---

## 💬 交流与支持

> 🔥 **欢迎加入交流群，获取技术支持、最新版本动态与部署经验分享！**

<div align="center">

| :heavy_plus_sign: 加入微信群 | :heavy_plus_sign: 加入 QQ 群 |
|:---:|:---:|
| **微信号：`dongguo58888`**（备注"云桌面"） | **QQ 群号：`140330037`** |

</div>

---

## 📖 简介

瘦客户端云桌面系统为企业和教育场景提供统一的云桌面接入方案：

- **桌面客户端**（Wails + React）：一个窗口管理并连接所有云桌面，支持 kiosk 一体机模式
- **后端服务**（Go + Gin）：对接 [Proxmox VE](https://www.proxmox.com/) 虚拟化平台，统一管理用户认证、虚拟机与连接分发
- **多协议接入**：自动识别虚拟机类型，选择最合适的远程桌面协议并检测本机客户端依赖

## ✨ 功能特性

- 🖥️ **统一登录入口** — 一个账号管理所有云桌面，支持记住密码 / 自动登录
- 🚀 **多协议支持** — RDP（FreeRDP / mstsc）、SPICE、VNC、SSH、VMware Horizon
- ⚡ **虚拟化管理** — 基于 Proxmox VE 的开关机、重启、状态监控
- 🔒 **安全认证** — JWT 令牌 + Redis 会话管理
- 🧩 **依赖自动检测** — 自动检测本机协议客户端并给出安装指引
- 🖥️ **Kiosk 一体机模式** — 全屏锁定、开机自启、自动连接指定桌面
- 🌓 **明暗主题** — 界面支持亮色 / 暗色切换
- 📱 **跨平台** — Windows / macOS / Linux 桌面客户端

## 🏗️ 系统架构

```
┌─────────────────────────────────────────────┐
│            Wails 桌面客户端                  │
│        (React + TypeScript + Go)            │
└────────────────────┬────────────────────────┘
                     │ HTTPS (JWT)
                     ▼
┌─────────────────────────────────────────────┐
│            Go 后端 API 服务                  │
│           (Gin + PostgreSQL + Redis)        │
└────────────────────┬────────────────────────┘
                     │ PVE HTTP API
                     ▼
┌─────────────────────────────────────────────┐
│          Proxmox VE 虚拟化平台               │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐     │
│   │ VM: RDP │  │VM: SPICE│  │ VM: VNC │ ... │
│   └─────────┘  └─────────┘  └─────────┘     │
└─────────────────────────────────────────────┘
```

## 🖥️ 协议支持

| 协议 | 适用场景 | Windows | macOS | Linux |
|------|---------|---------|-------|-------|
| RDP | Windows 虚拟机 | mstsc（系统自带）/ FreeRDP | Microsoft Remote Desktop | xfreerdp / rdesktop |
| SPICE | Linux 虚拟机（推荐） | virt-viewer | Virt Viewer | remote-viewer |
| VNC | 通用（未装增强工具） | VNC Viewer | 系统内置 | tigervncviewer |
| SSH | 命令行运维 | 系统 ssh | 系统 ssh | 系统 ssh |
| Horizon | VMware Horizon 桌面 | Horizon Client | Horizon Client | Horizon Client |

客户端启动连接时会**自动检测**本机依赖；「协议设置」页可查看支持状态与安装指引。

## 🚀 快速开始

### 环境要求

| 组件 | 版本要求 |
|------|---------|
| Go | 1.25+ |
| Node.js | 18+ |
| PostgreSQL | 14+ |
| Redis | 7+ |
| Proxmox VE | 7+ |
| [Wails CLI](https://wails.io/docs/gettingstarted/installation) | v2 最新版 |

### 1. 启动后端服务

```bash
# 克隆项目
git clone https://github.com/your-org/shoukehuduan.git
cd shoukehuduan

# 安装 Go 依赖
go mod download

# 配置
cp config.yaml.example config.yaml
# 编辑 config.yaml：填写 PVE 地址、数据库、Redis、JWT 密钥
vim config.yaml

# 启动
go run cmd/server/main.go
```

默认监听 `:8080`，接口前缀 `/api/v1`。

### 2. 启动桌面客户端

```bash
# 安装 Wails CLI（一次性）
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 开发模式（根目录运行，自动编译前端并热重载）
wails dev

# 编译生产版本
wails build
```

### 3.（可选）一键打包 Windows 安装包

```bash
cd cmd/desktop
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

脚本会自动：自增版本号 → `wails build` → 生成 NSIS 安装包（`dist/瘦客户端云桌面_x.x.x_Setup.exe`）。

## ⚙️ 配置说明

复制 `config.yaml.example` 为 `config.yaml` 后按需修改：

| 配置段 | 说明 |
|--------|------|
| `server` | 后端监听地址与端口 |
| `pve` | Proxmox VE 地址、账号、节点，`verify_ssl` 控制证书校验 |
| `database` | PostgreSQL 连接信息 |
| `redis` | Redis 连接信息（用于会话存储） |
| `jwt` | JWT 签名密钥与过期时间（**生产环境务必更换默认密钥**） |

## 📁 项目结构

```
shoukehuduan/
├── cmd/
│   ├── server/            # 后端服务入口
│   └── desktop/           # Wails 桌面客户端
│       ├── frontend/      # React 前端 (Vite + Ant Design)
│       ├── build.ps1      # Windows 一键构建打包脚本
│       └── installer.nsi  # NSIS 安装包脚本
├── internal/
│   ├── auth/              # 用户认证（JWT 签发/校验）
│   ├── session/           # 会话管理（Redis）
│   ├── pve/               # Proxmox VE API 客户端
│   ├── protocol/          # 远程桌面协议管理（RDP/SPICE/VNC/SSH/Horizon）
│   ├── handlers/          # HTTP 处理器
│   ├── routes/            # 路由注册
│   ├── middleware/        # JWT 中间件
│   ├── config/            # 配置加载
│   ├── database/          # PostgreSQL 连接
│   └── tray/              # 系统托盘
├── pkg/types/             # 公共类型定义
├── tools/genico/          # 图标生成工具
└── config.yaml.example    # 配置文件模板
```

## 🔌 API 概览

<details>
<summary><b>认证接口</b></summary>

```bash
# 注册
POST /api/v1/auth/register
{ "username": "test", "password": "123456", "email": "test@example.com" }

# 登录 → 返回 JWT token
POST /api/v1/auth/login
{ "username": "test", "password": "123456" }
```

</details>

<details>
<summary><b>虚拟机接口（需 Authorization: Bearer &lt;token&gt;）</b></summary>

```bash
GET  /api/v1/vms                  # 虚拟机列表
POST /api/v1/vms/{vmid}/start     # 开机
POST /api/v1/vms/{vmid}/stop      # 关机
POST /api/v1/vms/{vmid}/reboot    # 重启
POST /api/v1/vms/{vmid}/connect   # 获取连接信息（协议/地址/端口/凭据）
```

</details>

## 🛠️ 开发指南

- **新增远程桌面协议**：在 `internal/protocol/` 添加实现 → 在 `pkg/types/types.go` 定义协议常量 → 更新 PVE 协议检测与客户端启动逻辑
- **新增 API 接口**：在 `internal/handlers/` 添加处理器 → `internal/routes/routes.go` 注册路由 → 桌面端 `bindings.ts` 增加绑定
- **前端开发**：`cmd/desktop/frontend/` 下标准 Vite + React 工程，支持热重载

## 📄 License

本项目基于 [MIT License](LICENSE) 开源。

Copyright © 2026 山东飞力达软件有限公司 (Shandong Feilida Software Co., Ltd.)

## 💬 社区

| 微信 | QQ 群 |
|------|-------|
| `dongguo58888`（备注"云桌面"） | **140330037** |

问题反馈 / 功能建议 / 部署交流，欢迎加入上方社群，也欢迎提交 [Issue](../../issues) 与 [Pull Request](../../pulls)！

---

<div align="center">

**如果这个项目对你有帮助，欢迎点一个 Star ⭐**

</div>
