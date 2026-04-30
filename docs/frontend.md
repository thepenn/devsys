# Frontend Module / 前端模块

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

### Overview

The frontend is a React single-page application (SPA) built with Ant Design 5 for the UI component library. It provides two distinct interfaces: a **Developer Portal** (`/dev`) and an **Operations Console** (`/ops`).

**Source:** `modules/web/`

### Tech Stack

| Technology | Version | Purpose |
|-----------|---------|---------|
| React | 18.x | UI framework |
| Ant Design | 5.x | Component library |
| Redux | 4.x | State management |
| react-router-dom | 6.x | Client-side routing (HashRouter) |
| axios | 1.x | HTTP client |
| xterm.js | 5.x | Terminal emulator for K8s exec |
| dayjs | 1.x | Date/time handling |
| js-yaml | 4.x | YAML parsing for pipeline configs |
| Webpack | 5.x | Module bundler |
| Less | 4.x | CSS preprocessor |
| Tailwind CSS | 3.x | Utility-first CSS |

### Build & Embedding

The frontend builds to `modules/web/dist/` and is embedded into the Go binary:

```go
// web/web.go
//go:embed dist/*
var distFS embed.FS
```

The backend serves the SPA at `/` and static assets at `/static/*`, enabling single-binary deployment without a separate web server.

**Build command:**

```bash
make web    # npm run build in modules/web/
```

**Dev server:**

```bash
make run    # npm run start in modules/web/
```

The dev server uses `http-proxy-middleware` to proxy API requests to the Go backend at `localhost:8080`.

### Routing Structure

The app uses `HashRouter` with role-based route guards:

```
/
├── /login                           Login page
├── /dev/*          [RequireDeveloper]   Developer Portal
│   ├── /dashboard                      Dashboard
│   ├── /profile                        User profile
│   └── /projects/:owner/:name         Project pages
│       ├── /pipeline                   Pipeline list
│       ├── /pipeline/:runId            Pipeline run detail
│       ├── /deployment                 (placeholder)
│       └── /monitor                    (placeholder)
├── /ops/*          [RequireAdmin]       Operations Console
│   ├── /k8s/clusters                   Kubernetes clusters
│   ├── /k8s/workloads                  Workloads
│   ├── /k8s/services                   Services
│   ├── /k8s/pods                       Pods
│   ├── /k8s/jobs                       Jobs
│   ├── /k8s/volumes                    Volumes
│   ├── /k8s/nodes                      Nodes
│   ├── /k8s/monitor                    K8s monitoring
│   ├── /projects/list                  Project list
│   ├── /projects/pipeline              Project builds
│   ├── /projects/build/:repoId/:runId  Build detail
│   ├── /projects/monitor               Project monitoring
│   ├── /messages/notification          Notifications
│   ├── /messages/alert                 Alert management
│   ├── /db/mysql                       MySQL (placeholder)
│   ├── /db/redis                       Redis (placeholder)
│   ├── /db/mongo                       MongoDB (placeholder)
│   ├── /db/postgres                    PostgreSQL (placeholder)
│   ├── /system/credentials             Credential management
│   ├── /system/roles                   Role management
│   ├── /system/audit                   Audit logs
│   └── /profile                        Admin profile
└── /                                   Landing redirect
```

### Role-Based Access

- **RequireDeveloper** — Wraps `/dev` routes. Any authenticated user can access.
- **RequireAdmin** — Wraps `/ops` routes. Only users with `admin: true` can access.
- **Landing Redirect** — Root path (`/`) redirects admins to `/ops/k8s/clusters` and regular users to `/dev/dashboard`.

### Key Pages

#### Developer Portal

| Page | Component | Description |
|------|-----------|-------------|
| Dashboard | `DashboardPage` | User's project overview |
| Profile | `ProfilePage` | User profile settings |
| Pipeline | `ProjectPipeline` | Pipeline run list for a project |
| Run Detail | `ProjectRunDetail` | Detailed view of a pipeline run with workflows, steps, and logs |

#### Operations Console

| Page | Component | Description |
|------|-----------|-------------|
| Clusters | `K8sClusters` | Multi-cluster list and status |
| Workloads | `K8sWorkloads` | Deployment, StatefulSet, DaemonSet management |
| Pods | `K8sPods` | Pod list with status, logs, and terminal access |
| Services | `K8sServices` | Service and Ingress management |
| Jobs | `K8sJobs` | Job and CronJob management |
| Volumes | `K8sVolumes` | PV/PVC management |
| Nodes | `K8sNodes` | Node status and capacity |
| Credentials | `SystemCertificate` | Credential CRUD with encrypted storage |
| Roles | `SystemRoles` | User role management |
| Audit | `SystemAudit` | Audit log viewer |

### API Communication

HTTP requests are handled by axios with a centralized configuration:

- **Base URL:** `/api/v1` (configurable)
- **Dev proxy:** In development, requests are proxied to `localhost:8080`
- **Auth:** JWT token is included in the `Authorization` header as `Bearer <token>`

**Source:** `modules/web/src/utils/request.js`

### Terminal (xterm.js)

The K8s pod terminal uses xterm.js with the `xterm-addon-fit` addon for automatic resizing. It connects via WebSocket to the backend's exec stream endpoint.

**Features:**
- Full TTY support with ANSI color rendering
- Automatic terminal resize (sends `{"type":"resize","cols":N,"rows":N}`)
- Graceful close (sends `{"type":"close"}`)

### State Management

Redux is used for global state with middleware:
- **redux-promise** — Handles async action payloads
- **redux-logger** — Development-time action logging

Auth state is managed via a React Context (`AuthContext`) that provides `isAdmin`, `loading`, and user info to the component tree.

---

<a id="中文"></a>

## 中文

### 概述

前端是一个基于 React 的单页应用 (SPA)，使用 Ant Design 5 作为 UI 组件库。提供两个独立的界面：**开发者门户** (`/dev`) 和 **运维控制台** (`/ops`)。

**源码：** `modules/web/`

### 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| React | 18.x | UI 框架 |
| Ant Design | 5.x | 组件库 |
| Redux | 4.x | 状态管理 |
| react-router-dom | 6.x | 客户端路由 (HashRouter) |
| axios | 1.x | HTTP 客户端 |
| xterm.js | 5.x | K8s exec 终端模拟器 |
| dayjs | 1.x | 日期/时间处理 |
| js-yaml | 4.x | 流水线配置 YAML 解析 |
| Webpack | 5.x | 模块打包器 |
| Less | 4.x | CSS 预处理器 |
| Tailwind CSS | 3.x | 原子化 CSS |

### 构建与嵌入

前端构建到 `modules/web/dist/`，嵌入到 Go 二进制文件中：

```go
// web/web.go
//go:embed dist/*
var distFS embed.FS
```

后端在 `/` 提供 SPA 页面，在 `/static/*` 提供静态资源，实现单文件部署，无需额外 Web 服务器。

**构建命令：**

```bash
make web    # 在 modules/web/ 中运行 npm run build
```

**开发服务器：**

```bash
make run    # 在 modules/web/ 中运行 npm run start
```

开发服务器使用 `http-proxy-middleware` 将 API 请求代理到 `localhost:8080` 的 Go 后端。

### 路由结构

应用使用 `HashRouter` 配合基于角色的路由守卫：

```
/
├── /login                           登录页
├── /dev/*          [开发者守卫]       开发者门户
│   ├── /dashboard                      仪表盘
│   ├── /profile                        个人资料
│   └── /projects/:owner/:name         项目页面
│       ├── /pipeline                   流水线列表
│       ├── /pipeline/:runId            流水线运行详情
│       ├── /deployment                 (占位)
│       └── /monitor                    (占位)
├── /ops/*          [管理员守卫]       运维控制台
│   ├── /k8s/clusters                   Kubernetes 集群
│   ├── /k8s/workloads                  工作负载
│   ├── /k8s/services                   服务
│   ├── /k8s/pods                       Pod
│   ├── /k8s/jobs                       任务
│   ├── /k8s/volumes                    存储卷
│   ├── /k8s/nodes                      节点
│   ├── /k8s/monitor                    K8s 监控
│   ├── /projects/list                  项目列表
│   ├── /projects/pipeline              项目构建
│   ├── /projects/build/:repoId/:runId  构建详情
│   ├── /projects/monitor               项目监控
│   ├── /messages/notification          消息通知
│   ├── /messages/alert                 告警管理
│   ├── /db/mysql                       MySQL (占位)
│   ├── /db/redis                       Redis (占位)
│   ├── /db/mongo                       MongoDB (占位)
│   ├── /db/postgres                    PostgreSQL (占位)
│   ├── /system/credentials             凭证管理
│   ├── /system/roles                   角色管理
│   ├── /system/audit                   审计日志
│   └── /profile                        管理员资料
└── /                                   首页重定向
```

### 基于角色的访问控制

- **RequireDeveloper** — 包裹 `/dev` 路由。任何认证用户可访问。
- **RequireAdmin** — 包裹 `/ops` 路由。仅 `admin: true` 的用户可访问。
- **首页重定向** — 根路径 (`/`) 管理员跳转到 `/ops/k8s/clusters`，普通用户跳转到 `/dev/dashboard`。

### 核心页面

#### 开发者门户

| 页面 | 组件 | 说明 |
|------|------|------|
| 仪表盘 | `DashboardPage` | 用户项目概览 |
| 个人资料 | `ProfilePage` | 用户资料设置 |
| 流水线 | `ProjectPipeline` | 项目的流水线运行列表 |
| 运行详情 | `ProjectRunDetail` | 流水线运行详细视图，包含工作流、步骤和日志 |

#### 运维控制台

| 页面 | 组件 | 说明 |
|------|------|------|
| 集群 | `K8sClusters` | 多集群列表和状态 |
| 工作负载 | `K8sWorkloads` | Deployment、StatefulSet、DaemonSet 管理 |
| Pod | `K8sPods` | Pod 列表，含状态、日志和终端访问 |
| 服务 | `K8sServices` | Service 和 Ingress 管理 |
| 任务 | `K8sJobs` | Job 和 CronJob 管理 |
| 存储卷 | `K8sVolumes` | PV/PVC 管理 |
| 节点 | `K8sNodes` | 节点状态和容量 |
| 凭证 | `SystemCertificate` | 凭证 CRUD，加密存储 |
| 角色 | `SystemRoles` | 用户角色管理 |
| 审计 | `SystemAudit` | 审计日志查看 |

### API 通信

HTTP 请求通过 axios 统一配置处理：

- **基础 URL：** `/api/v1`（可配置）
- **开发代理：** 开发环境下请求代理到 `localhost:8080`
- **认证：** JWT 令牌以 `Bearer <token>` 形式包含在 `Authorization` 头中

**源码：** `modules/web/src/utils/request.js`

### 终端 (xterm.js)

K8s Pod 终端使用 xterm.js 配合 `xterm-addon-fit` 插件实现自动调整大小。通过 WebSocket 连接到后端的 exec 流端点。

**功能特性：**
- 完整 TTY 支持，支持 ANSI 颜色渲染
- 自动终端调整大小（发送 `{"type":"resize","cols":N,"rows":N}`）
- 优雅关闭（发送 `{"type":"close"}`）

### 状态管理

使用 Redux 进行全局状态管理，配合中间件：
- **redux-promise** — 处理异步 action payload
- **redux-logger** — 开发环境 action 日志

认证状态通过 React Context (`AuthContext`) 管理，向组件树提供 `isAdmin`、`loading` 和用户信息。
