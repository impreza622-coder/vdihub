# 瘦客户端云桌面系统

一个基于 Go + Wails 的跨平台瘦客户端云桌面系统，支持 FreeRDP、SPICE、VMware Horizon 等多种远程桌面协议，后端对接 Proxmox VE 虚拟化平台。

## 功能特性

- 🖥️ **统一登录入口**: 一个账号管理所有云桌面
- 🚀 **多协议支持**: RDP (FreeRDP)、SPICE、VNC、SSH
- ⚡ **虚拟化管理**: 基于 Proxmox VE 的虚拟机开关机、状态监控
- 🔒 **安全认证**: JWT 令牌认证，支持会话管理
- 📱 **跨平台**: 支持 Windows、macOS、Linux 桌面客户端

## 系统架构

```
┌─────────────────────────────────────────────────┐
│              Wails 桌面客户端                    │
│  (React + TypeScript + Go)                      │
└─────────────────────────────────────────────────┘
                      │
                      │ HTTPS
                      ▼
┌─────────────────────────────────────────────────┐
│              Go 后端 API 服务                    │
│  (Gin + PostgreSQL + Redis)                     │
└─────────────────────────────────────────────────┘
                      │
                      │ PVE HTTP API
                      ▼
┌─────────────────────────────────────────────────┐
│           Proxmox VE 虚拟化平台                  │
└─────────────────────────────────────────────────┘
```

## 项目结构

```
shoukehuduan/
├── cmd/
│   ├── server/          # 后端服务入口
│   └── desktop/         # Wails 桌面客户端
│       └── frontend/    # React 前端
├── internal/
│   ├── auth/            # 认证服务
│   ├── config/          # 配置管理
│   ├── database/        # 数据库连接
│   ├── handlers/        # API 处理器
│   ├── middleware/      # 中间件
│   ├── pve/             # PVE API 客户端
│   ├── protocol/        # 远程桌面协议管理
│   ├── routes/          # 路由配置
│   └── session/         # 会话管理
├── pkg/
│   └── types/           # 公共类型定义
├── config.yaml.example  # 配置文件示例
├── go.mod
├── go.sum
└── wails.json
```

## 快速开始

### 1. 环境要求

- Go 1.21+
- Node.js 18+
- npm 或 yarn
- PostgreSQL 14+
- Redis 7+
- Proxmox VE 7+

### 2. 安装依赖

```bash
# 安装 Go 依赖
go mod download

# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 3. 配置后端

```bash
# 复制配置文件
cp config.yaml.example config.yaml

# 编辑配置文件
vim config.yaml
```

配置文件说明:
- `server`: 后端服务地址和端口
- `pve`: Proxmox VE 连接信息
- `database`: PostgreSQL 数据库配置
- `redis`: Redis 缓存配置
- `jwt`: JWT 认证配置

### 4. 启动后端服务

```bash
# 开发模式
go run cmd/server/main.go

# 编译运行
go build -o server cmd/server/main.go
./server
```

### 5. 启动桌面客户端

```bash
# 开发模式（自动编译前端）
cd cmd/desktop/frontend
npm install
cd ../../..
wails dev

# 编译生产版本
wails build
```

## API 文档

### 认证接口

#### 注册用户
```
POST /api/v1/auth/register
Content-Type: application/json

{
  "username": "test",
  "password": "123456",
  "email": "test@example.com"
}
```

#### 用户登录
```
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "test",
  "password": "123456"
}
```

响应:
```json
{
  "code": 0,
  "message": "登录成功",
  "data": {
    "token": "jwt-token-here",
    "user": {
      "id": 1,
      "username": "test",
      "role": "user"
    }
  }
}
```

### 虚拟机管理接口

所有虚拟机接口需要在请求头中携带认证令牌:
```
Authorization: Bearer <jwt-token>
```

#### 获取虚拟机列表
```
GET /api/v1/vms
```

#### 启动虚拟机
```
POST /api/v1/vms/{vmid}/start
```

#### 停止虚拟机
```
POST /api/v1/vms/{vmid}/stop
```

#### 重启虚拟机
```
POST /api/v1/vms/{vmid}/reboot
```

#### 获取连接信息
```
POST /api/v1/vms/{vmid}/connect
```

响应:
```json
{
  "code": 0,
  "message": "获取连接信息成功",
  "data": {
    "session_id": "session-xxx",
    "protocol": "spice",
    "host": "pve.example.com",
    "port": 6081,
    "password": "spice-password",
    "tls_port": 6082,
    "expires_at": "2026-07-29T15:00:00Z"
  }
}
```

## 远程桌面协议集成

### RDP (FreeRDP)
- Windows 虚拟机自动使用 RDP 协议
- 默认端口: 3389
- 跨平台支持:
  - Windows: 系统自带 mstsc.exe
  - macOS: Microsoft Remote Desktop
  - Linux: FreeRDP (xfreerdp)、rdesktop、remmina
- 自动生成 .rdp 配置文件
- 自动检测系统 RDP 客户端依赖

### SPICE
- Linux 虚拟机默认使用 SPICE 协议
- 需要 PVE 宿主机安装 SPICE 支持
- 支持 TLS 加密连接
- 跨平台支持:
  - Windows: virt-viewer (remote-viewer.exe)
  - macOS: Virt Viewer
  - Linux: virt-viewer (remote-viewer)
- 自动生成 .vv 连接文件
- 自动检测系统 SPICE 客户端依赖

### VNC
- 通用远程桌面协议
- 适用于未安装增强工具的虚拟机
- 跨平台支持:
  - Windows: VNC Viewer
  - macOS: 系统内置 VNC 支持
  - Linux: tigervncviewer、vncviewer
- 自动检测系统 VNC 客户端依赖

### SSH
- 远程命令行访问
- 使用系统 ssh 客户端

## 客户端依赖检测

桌面客户端会自动检测以下协议依赖是否安装：

| 协议 | Windows | macOS | Linux |
|------|---------|-------|-------|
| RDP | mstsc.exe (系统自带) | Microsoft Remote Desktop | xfreerdp / rdesktop |
| SPICE | remote-viewer.exe / virt-viewer.exe | Virt Viewer | remote-viewer / virt-viewer |
| VNC | vncviewer.exe | 系统内置 | tigervncviewer |

在「协议设置」页面可以：
- 查看当前系统各协议的支持状态
- 获取各协议客户端的安装指引
- 一键重新检测依赖状态

## 开发说明

### 添加新的远程桌面协议

1. 在 `internal/protocol/` 目录添加新的协议实现
2. 在 `pkg/types/types.go` 中添加新的协议类型常量
3. 更新 PVE 客户端的协议检测逻辑
4. 更新桌面客户端的协议启动方法

### 添加新的 API 接口

1. 在 `internal/handlers/` 添加新的处理器函数
2. 在 `internal/routes/routes.go` 注册新路由
3. 在桌面客户端的 Service 类中添加对应方法

## 生产部署

### 后端服务
```bash
# 编译
CGO_ENABLED=0 go build -ldflags="-s -w" -o server cmd/server/main.go

# 使用 systemd 管理
cat > /etc/systemd/system/thinclient.service << 'EOF'
[Unit]
Description=Thin Client Backend Service
After=network.target postgresql.service redis.service

[Service]
Type=simple
ExecStart=/path/to/server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl enable thinclient
systemctl start thinclient
```

### 桌面客户端
```bash
# Windows 版本
wails build -platform windows

# macOS 版本
wails build -platform darwin

# Linux 版本
wails build -platform linux
```

## 技术栈

- **后端**: Go, Gin, GORM, PostgreSQL, Redis
- **前端**: React, TypeScript, Ant Design
- **桌面框架**: Wails v2
- **虚拟化**: Proxmox VE
- **远程协议**: RDP, SPICE, VNC

## License

MIT License
