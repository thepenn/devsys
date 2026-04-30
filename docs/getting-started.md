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

Terminal 1 — Start the backend (regenerates Wire + `go run cmd/*.go`, port 8080):

```bash
make server          # or: make run (alias)
```

Terminal 2 — Start the frontend dev server (CRA dev server, port 3002, proxies API to `localhost:8080`):

```bash
make web-dev
```

`make dev` prints these two commands as a hint — it doesn't actually start anything.

#### Production Mode (single binary, embedded SPA)

```bash
# Build the frontend bundle once into modules/web/dist/
make web

# Start the backend; web.go embeds dist via //go:embed and serves the SPA at /
make server
```

Access the application at `http://localhost:8080` — both API (`/api/v1/...`) and SPA (`/`) come from the same port.

#### Single-image Deployment (Docker)

For production hosts that just want one container:

```bash
# One-shot: pre-build SPA + Wire on the host, then docker build
make docker-image                     # IMAGE_NAME=devsys IMAGE_TAG=$(git describe --tags --always)
# Override for a private registry:
#   make docker-image IMAGE_NAME=registry.example.com/team/devsys IMAGE_TAG=v1.2.3

# Run with mounted docker socket + persistent workspace
make docker-run
```

See [README.md](../README.md#single-image-deployment-docker) for the full `docker run` example with required env vars.

### Makefile Targets

| Target | Description |
|--------|-------------|
| `make server` | Regenerate Wire injectors and start the backend (`go run cmd/*.go`); warns if `web/dist/index.html` is missing. |
| `make run` | Alias for `make server` (legacy entry kept for habit). |
| `make web-dev` | Start the React dev server (`npm run start`, port 3002). |
| `make web` | Build the production SPA bundle into `modules/web/dist/`. |
| `make wire` | Regenerate `modules/cmd/wire/wire_gen.go`. |
| `make fmt` | Run `go fmt ./...` over the backend. |
| `make dev` | Print the two-terminal dev workflow (does not start anything). |
| `make docker-image` | Pre-build SPA (`make web`) + Wire (`make wire`) on the host, then `docker build` the single-binary image. Override with `IMAGE_NAME=` / `IMAGE_TAG=`. |
| `make docker-run` | `docker run` the latest image with `/var/run/docker.sock` mounted and `modules/.env` loaded. |

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

终端 1 — 启动后端（自动 Wire 生成 + `go run cmd/*.go`，端口 8080）：

```bash
make server          # 或: make run (别名)
```

终端 2 — 启动前端开发服务器（CRA dev server，端口 3002，API 代理到 `localhost:8080`）：

```bash
make web-dev
```

`make dev` 现在只是打印这两条命令的帮助信息，不实际启动任何东西。

#### 生产模式（单二进制，前端嵌入）

```bash
# 把前端 bundle 一次性构建到 modules/web/dist/
make web

# 启动后端；web.go 通过 //go:embed 把 dist 嵌进二进制, 在 / 路径直接返回 SPA
make server
```

访问 `http://localhost:8080` 打开应用 —— API（`/api/v1/...`）和 SPA（`/`）共用同一个端口。

#### 单镜像部署（Docker）

只想跑一个容器的生产部署：

```bash
# 一键: 宿主机预构建 SPA + Wire, 然后 docker build
make docker-image                     # IMAGE_NAME=devsys IMAGE_TAG=$(git describe --tags --always)
# 推私有 registry 时覆盖:
#   make docker-image IMAGE_NAME=registry.example.com/team/devsys IMAGE_TAG=v1.2.3

# 挂 docker socket + 持久化 workspace 跑起来
make docker-run
```

完整的 `docker run` 示例（含必填 env）见 [README.md](../README.md#单镜像部署-docker)。

### Makefile 命令

| 目标 | 说明 |
|------|------|
| `make server` | 重新生成 Wire 注入器 + 启动后端（`go run cmd/*.go`）；如果 `web/dist/index.html` 缺失会打印警告。 |
| `make run` | `make server` 的别名（兼容旧入口）。 |
| `make web-dev` | 启动 React 开发服务器（`npm run start`，端口 3002）。 |
| `make web` | 构建生产 SPA bundle 到 `modules/web/dist/`。 |
| `make wire` | 重新生成 `modules/cmd/wire/wire_gen.go`。 |
| `make fmt` | 后端跑 `go fmt ./...`。 |
| `make dev` | 打印开发模式的双终端工作流（不实际启动任何东西）。 |
| `make docker-image` | 宿主机预构建 SPA（`make web`）+ Wire（`make wire`），然后 `docker build` 出单二进制镜像。`IMAGE_NAME=` / `IMAGE_TAG=` 可覆盖。 |
| `make docker-run` | `docker run` 最新镜像，自动挂 `/var/run/docker.sock` 并加载 `modules/.env`。 |

### 验证安装

```bash
# 健康检查
curl http://localhost:8080/api/v1/ping

# 预期响应：
# {"message":"pong"}

# OpenAPI 规范
curl http://localhost:8080/api/v1/api.json
```
