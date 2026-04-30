# DevSys

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

**DevSys** is a self-hosted, all-in-one DevOps platform built with Go and React. It unifies Git repository management, CI/CD pipelines, Kubernetes cluster operations, and credential management into a single deployable binary.

### Features

- **Multi-Git Platform Integration** — OAuth login and repository sync for GitHub, GitLab, Gitea, and Gitee
- **CI/CD Pipelines** — YAML-defined pipelines executed in Docker containers, with queue scheduling, cron triggers, manual approval steps, and real-time log streaming
- **Kubernetes Management** — Multi-cluster support with dynamic resource CRUD, workload details/rollback, Pod logs, and interactive WebSocket terminal (xterm.js)
- **Credential Management** — Encrypted credential store (RSA) supporting Git, Docker, MySQL, LDAP, Kubernetes, and more
- **Single Binary Deployment** — Frontend assets are embedded into the Go binary via `go:embed`
- **OpenAPI Documentation** — Auto-generated API spec at `/api.json`
- **Prometheus Metrics** — Built-in `/metrics` endpoint

### Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, go-restful v3, GORM + MySQL, Google Wire (DI), zerolog |
| Frontend | React 18, Ant Design 5, Redux, react-router-dom v6, xterm.js |
| Auth | OAuth2 + JWT |
| Pipeline Runtime | Docker API |
| Kubernetes | client-go (dynamic client) |
| Monitoring | Prometheus client |

### Project Structure

```
devsys/
├── Makefile                  # Build and run commands
├── docs/                     # Documentation
│   ├── architecture.md       # Architecture overview
│   ├── getting-started.md    # Quick start guide
│   ├── configuration.md      # Configuration reference
│   ├── auth.md               # Authentication module
│   ├── pipeline.md           # CI/CD Pipeline module
│   ├── kubernetes.md         # Kubernetes management module
│   ├── api-reference.md      # API reference
│   └── frontend.md           # Frontend module
└── modules/                  # Core application code
    ├── cmd/                  # Entry point and Wire DI
    ├── internal/             # Internal packages (config, handler, server, etc.)
    ├── model/                # GORM data models
    ├── routers/              # REST API route definitions
    ├── service/              # Business logic layer
    └── web/                  # React frontend (embedded into binary)
```

### Quick Start

**Prerequisites:** Go 1.24+, Node.js 18+, MySQL 8.0+, Docker

```bash
# 1. Clone the repository
git clone https://github.com/thepenn/devsys.git
cd devsys

# 2. Configure environment
cp modules/.env.example modules/.env
# Edit modules/.env with your database and OAuth settings

# 3. Build the frontend
make web

# 4. Generate Wire injectors and start the server
make server
```

The application will be available at `http://localhost:8080`.

### Single-image deployment (Docker)

The repository ships a multi-stage `Dockerfile` that combines the React frontend (built via `npm run build`) and the Go backend (which embeds the resulting `web/dist` through `//go:embed`) into a single static binary, then bakes it into a small Alpine image. The whole platform — UI + API + pipeline engine — is one container.

```bash
# 1. Build the image. `make docker-image` runs `make web` (npm + webpack) and
#    `make wire` (DI codegen) on the host first, then `docker build .`. The
#    container build only does `go build`, so even small Colima / QEMU VMs are
#    safe (webpack and wire each peak at 2-3 GB and would OOM inside BuildKit).
#    If you call `docker build .` directly, run `make web && make wire` first.
make docker-image                           # = make web && make wire && docker build -t devsys:latest .
# Push to a private registry by overriding IMAGE_NAME/IMAGE_TAG:
#   make docker-image IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/sixx/devsys IMAGE_TAG=v1.2.3

# 2. Run (assumes an external MySQL on host port 33306)
docker run -d --name devsys \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v devsys-workspace:/var/lib/devsys-workspace \
  -e DATABASE_DATASOURCE='root:123456@tcp(host.docker.internal:33306)/devsys?charset=utf8mb4&parseTime=True&loc=Local' \
  -e SERVER_AUTH_SESSION_SECRET="$(openssl rand -base64 32)" \
  devsys:latest
```

Runtime dependencies:

- **MySQL** is external. The first start runs `AutoMigrate` to create / upgrade tables.
- **Docker socket** mount (`/var/run/docker.sock`) is required for the CI/CD pipeline engine (it spawns step containers and BuildKit). Without it, login / project / credential management still works but pipeline steps will fail to launch containers.
- **Workspace** defaults to `/var/lib/devsys-workspace` inside the container (set by the image's `PIPELINE_WORKSPACE_ROOT`). The path **must be a bind volume that exists at the same path on both the devsys container and the host**, otherwise step containers spawned via the host docker daemon will see an empty directory. The recommended one-liner is `-v devsys-workspace:/var/lib/devsys-workspace`. Override with `-e PIPELINE_WORKSPACE_ROOT=/your/path` if you need a different location, but then mount the same host path under the same name into the devsys container.
- **Default port** is `8080`. The image overrides `SERVER_HOST` to `0.0.0.0:8080` so it is reachable from outside the container; everything else can be tuned via `-e` / `--env-file modules/.env`.

### Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | System architecture and design |
| [Getting Started](docs/getting-started.md) | Installation and quick start guide |
| [Configuration](docs/configuration.md) | Environment variables reference |
| [Authentication](docs/auth.md) | OAuth and JWT authentication |
| [CI/CD Pipeline](docs/pipeline.md) | Pipeline engine and YAML spec |
| [Kubernetes](docs/kubernetes.md) | Kubernetes cluster management |
| [API Reference](docs/api-reference.md) | REST API endpoints |
| [Frontend](docs/frontend.md) | Frontend architecture and pages |

### License

This project is for internal use. See the repository for licensing details.

---

<a id="中文"></a>

## 中文

**DevSys** 是一个自托管的一体化 DevOps 平台，基于 Go 和 React 构建。它将 Git 仓库管理、CI/CD 流水线、Kubernetes 集群运维和凭证管理整合到一个可部署的单一二进制文件中。

### 功能特性

- **多 Git 平台集成** — 支持 GitHub、GitLab、Gitea、Gitee 的 OAuth 登录和仓库同步
- **CI/CD 流水线** — YAML 定义的流水线，在 Docker 容器中执行，支持队列调度、定时触发、人工审批和实时日志流
- **Kubernetes 管理** — 多集群支持，动态资源增删改查、工作负载详情/回滚、Pod 日志、WebSocket 交互式终端 (xterm.js)
- **凭证管理** — RSA 加密的凭证存储，支持 Git、Docker、MySQL、LDAP、Kubernetes 等类型
- **单文件部署** — 前端资源通过 `go:embed` 嵌入 Go 二进制文件
- **OpenAPI 文档** — 自动生成的 API 规范，访问 `/api.json`
- **Prometheus 监控** — 内置 `/metrics` 指标端点

### 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.24, go-restful v3, GORM + MySQL, Google Wire (依赖注入), zerolog |
| 前端 | React 18, Ant Design 5, Redux, react-router-dom v6, xterm.js |
| 认证 | OAuth2 + JWT |
| 流水线运行时 | Docker API |
| Kubernetes | client-go (动态客户端) |
| 监控 | Prometheus client |

### 项目结构

```
devsys/
├── Makefile                  # 构建和运行命令
├── docs/                     # 项目文档
│   ├── architecture.md       # 架构概览
│   ├── getting-started.md    # 快速开始
│   ├── configuration.md      # 配置参考
│   ├── auth.md               # 认证模块
│   ├── pipeline.md           # CI/CD 流水线模块
│   ├── kubernetes.md         # Kubernetes 管理模块
│   ├── api-reference.md      # API 参考
│   └── frontend.md           # 前端模块
└── modules/                  # 核心应用代码
    ├── cmd/                  # 程序入口和 Wire 依赖注入
    ├── internal/             # 内部包 (配置、处理器、服务器等)
    ├── model/                # GORM 数据模型
    ├── routers/              # REST API 路由定义
    ├── service/              # 业务逻辑层
    └── web/                  # React 前端 (嵌入到二进制文件)
```

### 快速开始

**前置要求：** Go 1.24+, Node.js 18+, MySQL 8.0+, Docker

```bash
# 1. 克隆仓库
git clone https://github.com/thepenn/devsys.git
cd devsys

# 2. 配置环境变量
cp modules/.env.example modules/.env
# 编辑 modules/.env，填入数据库和 OAuth 配置

# 3. 构建前端
make web

# 4. 生成 Wire 注入器并启动服务
make server
```

应用将在 `http://localhost:8080` 上运行。

### 单镜像部署 (Docker)

仓库根目录提供了一个多阶段 `Dockerfile`，把 React 前端（`npm run build`）和 Go 后端（通过 `//go:embed` 嵌入构建好的 `web/dist`）打成一个静态二进制，再放进一个轻量 Alpine 镜像。整个平台 —— UI + API + 流水线引擎 —— 就是一个容器。

```bash
# 1. 构建镜像. `make docker-image` 会先在宿主机跑 `make web` (npm + webpack) 与
#    `make wire` (DI 生成), 再 `docker build .`. 容器构建里只剩 `go build`, 所以
#    Colima / QEMU 这种受限 VM 也跑得动 (webpack 与 wire 单进程峰值 2-3 GB,
#    在 BuildKit 容器里会 OOM 直接 SIGKILL). 直接 `docker build .` 必须事先
#    跑 `make web && make wire`.
make docker-image                           # = make web && make wire && docker build -t devsys:latest .
# 推到私有 registry 时覆盖 IMAGE_NAME/IMAGE_TAG:
#   make docker-image IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/sixx/devsys IMAGE_TAG=v1.2.3

# 2. 运行 (假设外部 MySQL 跑在宿主 33306)
docker run -d --name devsys \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v devsys-workspace:/var/lib/devsys-workspace \
  -e DATABASE_DATASOURCE='root:123456@tcp(host.docker.internal:33306)/devsys?charset=utf8mb4&parseTime=True&loc=Local' \
  -e SERVER_AUTH_SESSION_SECRET="$(openssl rand -base64 32)" \
  devsys:latest
```

运行依赖：

- **MySQL** 外部依赖，首次启动会自动 `AutoMigrate` 建表 / 升级 schema。
- **Docker socket**（`/var/run/docker.sock`）挂载是 CI/CD 流水线引擎所需 —— 引擎会拉起步骤容器和 BuildKit。不挂载则登录 / 项目 / 凭证管理仍可用，但流水线步骤无法启动容器。
- **Workspace** 默认在容器内 `/var/lib/devsys-workspace`（镜像内置的 `PIPELINE_WORKSPACE_ROOT`）。该路径**必须是 host 与 devsys 容器同名的 bind volume**，否则 step container 通过 host docker daemon 拉起时看到的 `/workspace` 跟 controller 写入的 fs 不在一处，会出现 BuildKit 找不到 Dockerfile / 代码的问题。推荐就一行 `-v devsys-workspace:/var/lib/devsys-workspace`。需要换路径就 `-e PIPELINE_WORKSPACE_ROOT=/your/path`，并把同一个 host 路径以同名 bind 挂进 devsys 容器。
- **默认端口** 是 `8080`。镜像里把 `SERVER_HOST` 覆盖成了 `0.0.0.0:8080`，保证容器外可达；其它配置走 `-e` / `--env-file modules/.env` 注入。

### 文档目录

| 文档 | 说明 |
|------|------|
| [架构概览](docs/architecture.md) | 系统架构与设计 |
| [快速开始](docs/getting-started.md) | 安装与快速上手 |
| [配置参考](docs/configuration.md) | 环境变量说明 |
| [认证模块](docs/auth.md) | OAuth 和 JWT 认证 |
| [CI/CD 流水线](docs/pipeline.md) | 流水线引擎与 YAML 规范 |
| [Kubernetes](docs/kubernetes.md) | Kubernetes 集群管理 |
| [API 参考](docs/api-reference.md) | REST API 端点 |
| [前端模块](docs/frontend.md) | 前端架构与页面 |

### 许可证

本项目为内部使用。具体许可信息请参阅仓库说明。
