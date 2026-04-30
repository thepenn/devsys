# Architecture / 架构概览

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

### High-Level Overview

DevSys follows a classic layered architecture with clear separation of concerns:

```
┌─────────────────────────────────────────────────┐
│                   React SPA                      │
│          (Ant Design + Redux + xterm.js)         │
└──────────────────────┬──────────────────────────┘
                       │ HTTP / WebSocket
┌──────────────────────▼──────────────────────────┐
│                 HTTP Server                       │
│           (go-restful + CORS)                    │
├──────────────────────────────────────────────────┤
│              Middleware Layer                      │
│     Auth │ Admin │ Metrics │ CORS                │
├──────────────────────────────────────────────────┤
│               Router Layer                        │
│  Health │ Auth │ Repos │ K8s │ System │ Web      │
├──────────────────────────────────────────────────┤
│              Service Layer                        │
│  Auth │ User │ Repo │ Pipeline │ K8s │ System    │
├──────────────────────────────────────────────────┤
│            Infrastructure Layer                   │
│   GORM/MySQL │ Docker API │ client-go │ Cache    │
└──────────────────────────────────────────────────┘
```

### Startup Flow

1. **Environment Loading** — `godotenv` loads `.env` file automatically
2. **Configuration** — `envconfig` parses environment variables into the `Config` struct
3. **Wire Dependency Injection** — Google Wire assembles the full dependency graph:
   - Database connection + auto-migration
   - In-memory TTL cache (5 min)
   - Pipeline queue (bounded capacity)
   - All services (User, Repo, Auth, Pipeline, System, K8s)
   - All middlewares (Auth, Admin, Metrics, CORS)
   - Router registration → Handler → HTTP Server
4. **Service Startup** — Pipeline service starts background workers and cron scheduler
5. **HTTP Listening** — Server begins accepting requests

### Key Design Decisions

**Single Binary Deployment**

The React frontend is built to `web/dist/` and embedded into the Go binary using `go:embed`. The `webHandler` router serves the SPA at `/` and static assets at `/static/*`. This eliminates the need for a separate web server or CDN in simple deployments.

**Wire for Dependency Injection**

Google Wire provides compile-time dependency injection, ensuring all dependencies are resolved at build time rather than runtime. The `wire.go` file defines the dependency graph, and `wire_gen.go` contains the generated code.

**go-restful for HTTP**

The `go-restful` framework provides route definitions with built-in OpenAPI spec generation. Each router module registers its routes as `WebService` instances with typed request/response models, filters (middlewares), and documentation metadata.

**Multi-Forge Authentication**

The auth service is designed to support multiple Git forge providers (GitHub, GitLab, Gitea, Gitee) through a common interface. The active provider is selected via configuration. OAuth tokens are stored per-user and used for API calls to sync repositories.

**Pipeline Execution Model**

Pipelines follow a queue-worker model:
- Manual triggers or cron schedules create pipeline records and enqueue them
- Worker goroutines dequeue and execute pipelines using Docker containers
- Each pipeline consists of Workflows → Steps → Tasks
- Steps support types: `clone`, `service`, `plugin`, `commands`, `cache`, `approval`

**Kubernetes Dynamic Client**

Instead of generating typed clients for each resource, DevSys uses the Kubernetes dynamic client with server-side discovery. This allows managing any resource type without code changes, making it suitable for CRD-heavy environments.

### Module Dependencies

```
cmd/main.go
  └─ cmd/wire/wire.go (DI assembly)
       ├─ internal/config      (configuration)
       ├─ internal/store       (database connection)
       ├─ internal/cache       (in-memory TTL cache)
       ├─ internal/handler     (HTTP handler + middleware registration)
       ├─ internal/server      (HTTP server)
       ├─ service/             (business logic)
       │    ├─ auth            (OAuth + JWT + repo sync)
       │    ├─ user            (user CRUD)
       │    ├─ repo            (repository CRUD)
       │    ├─ pipeline        (CI/CD engine)
       │    │    ├─ queue      (bounded pipeline queue)
       │    │    ├─ runtime/   (Docker execution)
       │    │    └─ spec/      (YAML pipeline spec)
       │    ├─ k8s             (Kubernetes operations)
       │    ├─ system          (RSA crypto + certificates)
       │    └─ migrate         (DB schema migration)
       ├─ routers/             (API route definitions)
       │    └─ middleware/      (auth, admin, cors, metrics)
       ├─ model/               (data models)
       └─ web/                 (embedded frontend)
```

---

<a id="中文"></a>

## 中文

### 总体概览

DevSys 采用经典的分层架构，各层职责清晰分离：

```
┌─────────────────────────────────────────────────┐
│                   React SPA                      │
│          (Ant Design + Redux + xterm.js)         │
└──────────────────────┬──────────────────────────┘
                       │ HTTP / WebSocket
┌──────────────────────▼──────────────────────────┐
│                 HTTP 服务器                       │
│           (go-restful + CORS)                    │
├──────────────────────────────────────────────────┤
│                 中间件层                          │
│     认证 │ 管理员 │ 指标 │ CORS                   │
├──────────────────────────────────────────────────┤
│                 路由层                            │
│  健康检查 │ 认证 │ 仓库 │ K8s │ 系统 │ 静态资源    │
├──────────────────────────────────────────────────┤
│                 服务层                            │
│  认证 │ 用户 │ 仓库 │ 流水线 │ K8s │ 系统         │
├──────────────────────────────────────────────────┤
│                基础设施层                         │
│   GORM/MySQL │ Docker API │ client-go │ 缓存     │
└──────────────────────────────────────────────────┘
```

### 启动流程

1. **环境加载** — `godotenv` 自动加载 `.env` 文件
2. **配置解析** — `envconfig` 将环境变量解析到 `Config` 结构体
3. **Wire 依赖注入** — Google Wire 组装完整依赖图：
   - 数据库连接 + 自动迁移
   - 内存 TTL 缓存 (5 分钟)
   - 流水线队列 (有界容量)
   - 所有服务 (User, Repo, Auth, Pipeline, System, K8s)
   - 所有中间件 (Auth, Admin, Metrics, CORS)
   - 路由注册 → Handler → HTTP Server
4. **服务启动** — Pipeline 服务启动后台 Worker 和 Cron 调度器
5. **HTTP 监听** — 服务器开始接受请求

### 核心设计决策

**单文件部署**

React 前端构建到 `web/dist/`，通过 `go:embed` 嵌入 Go 二进制文件。`webHandler` 路由在 `/` 提供 SPA 页面，在 `/static/*` 提供静态资源。简单部署场景下无需额外的 Web 服务器或 CDN。

**Wire 依赖注入**

Google Wire 提供编译时依赖注入，确保所有依赖在构建时而非运行时解析。`wire.go` 定义依赖图，`wire_gen.go` 包含生成的代码。

**go-restful HTTP 框架**

`go-restful` 框架提供带有内置 OpenAPI 规范生成的路由定义。每个路由模块将路由注册为 `WebService` 实例，包含类型化的请求/响应模型、过滤器（中间件）和文档元数据。

**多平台 Git 认证**

认证服务通过通用接口支持多个 Git 平台（GitHub、GitLab、Gitea、Gitee）。活跃的提供者通过配置选择。OAuth 令牌按用户存储，用于调用 API 同步仓库。

**流水线执行模型**

流水线采用队列-工作者模型：
- 手动触发或定时调度创建流水线记录并入队
- Worker 协程出队并使用 Docker 容器执行流水线
- 每个流水线由 Workflow → Step → Task 组成
- Step 支持类型：`clone`、`service`、`plugin`、`commands`、`cache`、`approval`

**Kubernetes 动态客户端**

DevSys 使用 Kubernetes 动态客户端和服务端发现，而非为每种资源生成类型化客户端。这允许在不修改代码的情况下管理任何资源类型，特别适合大量使用 CRD 的环境。

### 模块依赖关系

```
cmd/main.go
  └─ cmd/wire/wire.go (依赖注入组装)
       ├─ internal/config      (配置)
       ├─ internal/store       (数据库连接)
       ├─ internal/cache       (内存 TTL 缓存)
       ├─ internal/handler     (HTTP 处理器 + 中间件注册)
       ├─ internal/server      (HTTP 服务器)
       ├─ service/             (业务逻辑)
       │    ├─ auth            (OAuth + JWT + 仓库同步)
       │    ├─ user            (用户管理)
       │    ├─ repo            (仓库管理)
       │    ├─ pipeline        (CI/CD 引擎)
       │    │    ├─ queue      (有界流水线队列)
       │    │    ├─ runtime/   (Docker 执行)
       │    │    └─ spec/      (YAML 流水线规范)
       │    ├─ k8s             (Kubernetes 操作)
       │    ├─ system          (RSA 加密 + 凭证)
       │    └─ migrate         (数据库迁移)
       ├─ routers/             (API 路由定义)
       │    └─ middleware/      (认证、管理员、CORS、指标)
       ├─ model/               (数据模型)
       └─ web/                 (内嵌前端)
```
