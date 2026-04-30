# Getting Started / 快速开始

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

### Prerequisites

| Software | Version | Purpose |
|----------|---------|---------|
| Go | 1.24+ | Backend compilation |
| Node.js | 18+ | Frontend build |
| MySQL | 8.0+ | Data storage |
| Docker | 20.10+ | Pipeline execution runtime |
| Wire | latest | Dependency injection code generation |

### Installation

#### 1. Clone the Repository

```bash
git clone https://github.com/thepenn/devsys.git
cd devsys
```

#### 2. Install Go Dependencies

```bash
cd modules
go mod download
```

#### 3. Install Wire (if not already installed)

```bash
go install github.com/google/wire/cmd/wire@latest
```

#### 4. Install Frontend Dependencies

```bash
cd modules/web
npm install
```

#### 5. Configure Environment

Create a `.env` file in the `modules/` directory:

```bash
# Database
DATABASE_DRIVER=mysql
DATABASE_DATASOURCE=root:password@tcp(localhost:3306)/devops?charset=utf8mb4&parseTime=True&loc=Local
DATABASE_MAX_CONNECTIONS=10
DATABASE_SHOW_SQL=false

# Server
SERVER_HOST=localhost:8080
SERVER_ROOT_PATH=/api/v1

# Logging
LOG_LEVEL=info
LOG_PRETTY=true

# Pipeline
PIPELINE_WORKER_COUNT=2
PIPELINE_QUEUE_CAPACITY=128

# Auth (choose one provider)
SERVER_AUTH_PROVIDER=gitlab

# GitLab OAuth (example)
SERVER_GITLAB=true
SERVER_GITLAB_URL=https://gitlab.com
SERVER_GITLAB_CLIENT=your-client-id
SERVER_GITLAB_SECRET=your-client-secret
SERVER_GITLAB_REDIRECT=http://localhost:8080/api/v1/auth/gitlab/callback
```

See the full [Configuration Reference](configuration.md) for all available options.

#### 6. Prepare the Database

Create a MySQL database:

```sql
CREATE DATABASE devops CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

Schema migration runs automatically on startup via GORM `AutoMigrate`.

### Running

#### Development Mode (separate backend + frontend)

Terminal 1 — Start the backend:

```bash
make server
```

Terminal 2 — Start the frontend dev server:

```bash
make run
```

The frontend dev server proxies API requests to the backend at `localhost:8080`.

#### Production Mode (single binary)

```bash
# Build the frontend
make web

# Build and run (Wire + Go)
make server
```

Access the application at `http://localhost:8080`.

### Makefile Targets

| Target | Command | Description |
|--------|---------|-------------|
| `make server` | Wire generate + `go run` | Start the backend server |
| `make run` | `npm run start` | Start the frontend dev server |
| `make web` | `npm run build` | Build the frontend for production |
| `make wire` | `wire gen` | Regenerate Wire injectors |
| `make fmt` | `go fmt ./...` | Format Go source code |
| `make dev` | `web` + `run` | Build frontend and start dev server |

### Verifying the Setup

```bash
# Health check
curl http://localhost:8080/api/v1/ping

# Expected response:
# {"message":"pong"}

# OpenAPI spec
curl http://localhost:8080/api/v1/api.json
```

---

<a id="中文"></a>

## 中文

### 前置要求

| 软件 | 版本 | 用途 |
|------|------|------|
| Go | 1.24+ | 后端编译 |
| Node.js | 18+ | 前端构建 |
| MySQL | 8.0+ | 数据存储 |
| Docker | 20.10+ | 流水线执行运行时 |
| Wire | 最新版 | 依赖注入代码生成 |

### 安装步骤

#### 1. 克隆仓库

```bash
git clone https://github.com/thepenn/devsys.git
cd devsys
```

#### 2. 安装 Go 依赖

```bash
cd modules
go mod download
```

#### 3. 安装 Wire（如尚未安装）

```bash
go install github.com/google/wire/cmd/wire@latest
```

#### 4. 安装前端依赖

```bash
cd modules/web
npm install
```

#### 5. 配置环境变量

在 `modules/` 目录下创建 `.env` 文件：

```bash
# 数据库
DATABASE_DRIVER=mysql
DATABASE_DATASOURCE=root:password@tcp(localhost:3306)/devops?charset=utf8mb4&parseTime=True&loc=Local
DATABASE_MAX_CONNECTIONS=10
DATABASE_SHOW_SQL=false

# 服务器
SERVER_HOST=localhost:8080
SERVER_ROOT_PATH=/api/v1

# 日志
LOG_LEVEL=info
LOG_PRETTY=true

# 流水线
PIPELINE_WORKER_COUNT=2
PIPELINE_QUEUE_CAPACITY=128

# 认证（选择一个提供者）
SERVER_AUTH_PROVIDER=gitlab

# GitLab OAuth（示例）
SERVER_GITLAB=true
SERVER_GITLAB_URL=https://gitlab.com
SERVER_GITLAB_CLIENT=your-client-id
SERVER_GITLAB_SECRET=your-client-secret
SERVER_GITLAB_REDIRECT=http://localhost:8080/api/v1/auth/gitlab/callback
```

完整配置项请参阅 [配置参考](configuration.md)。

#### 6. 准备数据库

创建 MySQL 数据库：

```sql
CREATE DATABASE devops CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

数据库表结构在启动时通过 GORM `AutoMigrate` 自动迁移。

### 运行方式

#### 开发模式（前后端分离）

终端 1 — 启动后端：

```bash
make server
```

终端 2 — 启动前端开发服务器：

```bash
make run
```

前端开发服务器会将 API 请求代理到 `localhost:8080` 的后端。

#### 生产模式（单文件部署）

```bash
# 构建前端
make web

# 构建并运行（Wire + Go）
make server
```

访问 `http://localhost:8080` 打开应用。

### Makefile 命令

| 目标 | 命令 | 说明 |
|------|------|------|
| `make server` | Wire 生成 + `go run` | 启动后端服务 |
| `make run` | `npm run start` | 启动前端开发服务器 |
| `make web` | `npm run build` | 构建生产环境前端 |
| `make wire` | `wire gen` | 重新生成 Wire 注入器 |
| `make fmt` | `go fmt ./...` | 格式化 Go 源代码 |
| `make dev` | `web` + `run` | 构建前端并启动开发服务器 |

### 验证安装

```bash
# 健康检查
curl http://localhost:8080/api/v1/ping

# 预期响应：
# {"message":"pong"}

# OpenAPI 规范
curl http://localhost:8080/api/v1/api.json
```
