# Authentication Module / 认证模块

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

### Overview

The authentication module handles user identity via OAuth2 integration with Git forge platforms and issues JWT tokens for API session management.

**Source:** `modules/service/auth/`, `modules/routers/auth_gitlab.go`

### Supported Providers

| Provider | Env Variable | Default |
|----------|-------------|---------|
| GitHub | `SERVER_GITHUB` | `false` |
| GitLab | `SERVER_GITLAB` | `true` |
| Gitea | `SERVER_GITEA` | `false` |
| Gitee | `SERVER_GITEE` | `false` |

Only **one provider** is active at a time, determined by `SERVER_AUTH_PROVIDER`.

### Authentication Flow

```
┌────────┐     ┌────────────┐     ┌──────────────┐     ┌────────┐
│ Browser │────▶│ DevSys API │────▶│ Git Forge    │────▶│ Browser│
│         │     │ /auth/     │     │ OAuth Server │     │        │
│         │     │ {provider}/│     │              │     │        │
│         │     │ login      │     │              │     │        │
└────────┘     └────────────┘     └──────┬───────┘     └────────┘
                                         │ callback
                                         ▼
                                  ┌──────────────┐
                                  │ DevSys API   │
                                  │ /auth/       │
                                  │ {provider}/  │
                                  │ callback     │
                                  └──────┬───────┘
                                         │ JWT token
                                         ▼
                                  ┌──────────────┐
                                  │ Frontend SPA │
                                  │ stores token │
                                  └──────────────┘
```

1. **Login Redirect** — `GET /api/v1/auth/{provider}/login` redirects the user to the Git forge's OAuth authorization page with a CSRF state token
2. **OAuth Callback** — `GET /api/v1/auth/{provider}/callback` receives the authorization code, exchanges it for access/refresh tokens
3. **User Upsert** — The user profile is fetched from the forge API and upserted into the local database. The first user is automatically granted admin privileges
4. **JWT Issuance** — A JWT token is generated containing `user_id`, `login`, and `admin` claims, then returned to the frontend
5. **Session Validation** — Subsequent API requests include the JWT as a `Bearer` token in the `Authorization` header. The auth middleware validates and extracts claims

### JWT Token

- **Signing:** HMAC with the session secret (`SERVER_AUTH_SESSION_SECRET`), falls back to RSA if configured
- **TTL:** Configurable via `SERVER_AUTH_TOKEN_TTL` (default: 24 hours)
- **Claims:**
  - `user_id` — internal user ID
  - `login` — username from the Git forge
  - `admin` — boolean admin flag

### Middleware

**Auth Middleware** (`routers/middleware/auth/`)

- `Authenticate` — Extracts and validates the JWT from the request. Populates the context with session claims. Does not reject unauthenticated requests (allows public endpoints).
- `RequireAuth` — Rejects requests that do not have valid session claims in context.

**Admin Middleware** (`routers/middleware/admin/`)

- Routes tagged with `AdminEnable: true` metadata require the requesting user to have `admin: true` in their claims.

### Repository Sync

After authentication, users can trigger repository synchronization:

- `POST /api/v1/repos/sync` — Fetches all accessible repositories from the forge using the stored OAuth token and upserts them into the local database
- `POST /api/v1/repos/{repo_id}/sync` — Syncs a single repository
- Organization filtering is supported via `SERVER_{PROVIDER}_ORGS`

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/auth/{provider}/login` | Redirect to OAuth authorization |
| `GET` | `/auth/{provider}/callback` | OAuth callback, returns JWT |
| `GET` | `/auth/{provider}/me` | Get current authenticated user |

---

<a id="中文"></a>

## 中文

### 概述

认证模块通过 OAuth2 集成 Git 平台实现用户身份认证，并发放 JWT 令牌用于 API 会话管理。

**源码：** `modules/service/auth/`、`modules/routers/auth_gitlab.go`

### 支持的提供者

| 提供者 | 环境变量 | 默认值 |
|--------|---------|--------|
| GitHub | `SERVER_GITHUB` | `false` |
| GitLab | `SERVER_GITLAB` | `true` |
| Gitea | `SERVER_GITEA` | `false` |
| Gitee | `SERVER_GITEE` | `false` |

同一时间仅有**一个提供者**处于活跃状态，由 `SERVER_AUTH_PROVIDER` 决定。

### 认证流程

```
┌────────┐     ┌────────────┐     ┌──────────────┐     ┌────────┐
│ 浏览器  │────▶│ DevSys API │────▶│ Git 平台     │────▶│ 浏览器  │
│         │     │ /auth/     │     │ OAuth 服务器  │     │        │
│         │     │ {provider}/│     │              │     │        │
│         │     │ login      │     │              │     │        │
└────────┘     └────────────┘     └──────┬───────┘     └────────┘
                                         │ callback
                                         ▼
                                  ┌──────────────┐
                                  │ DevSys API   │
                                  │ /auth/       │
                                  │ {provider}/  │
                                  │ callback     │
                                  └──────┬───────┘
                                         │ JWT 令牌
                                         ▼
                                  ┌──────────────┐
                                  │ 前端 SPA     │
                                  │ 存储令牌      │
                                  └──────────────┘
```

1. **登录重定向** — `GET /api/v1/auth/{provider}/login` 将用户重定向到 Git 平台的 OAuth 授权页面，携带 CSRF state 令牌
2. **OAuth 回调** — `GET /api/v1/auth/{provider}/callback` 接收授权码，交换获取 access/refresh token
3. **用户创建/更新** — 从平台 API 获取用户信息并写入本地数据库。第一个注册的用户自动获得管理员权限
4. **JWT 签发** — 生成包含 `user_id`、`login` 和 `admin` 声明的 JWT 令牌，返回给前端
5. **会话验证** — 后续 API 请求在 `Authorization` 头中携带 `Bearer` 令牌。认证中间件验证并提取声明信息

### JWT 令牌

- **签名方式：** HMAC（使用 `SERVER_AUTH_SESSION_SECRET`），如已配置则回退到 RSA
- **有效期：** 通过 `SERVER_AUTH_TOKEN_TTL` 配置（默认 24 小时）
- **声明内容：**
  - `user_id` — 内部用户 ID
  - `login` — Git 平台的用户名
  - `admin` — 管理员标志

### 中间件

**认证中间件** (`routers/middleware/auth/`)

- `Authenticate` — 从请求中提取并验证 JWT，将会话声明填充到上下文中。不拒绝未认证请求（允许公开端点访问）。
- `RequireAuth` — 拒绝上下文中没有有效会话声明的请求。

**管理员中间件** (`routers/middleware/admin/`)

- 标记了 `AdminEnable: true` 元数据的路由要求请求用户的声明中包含 `admin: true`。

### 仓库同步

认证完成后，用户可以触发仓库同步：

- `POST /api/v1/repos/sync` — 使用存储的 OAuth 令牌从平台获取所有可访问的仓库，并写入本地数据库
- `POST /api/v1/repos/{repo_id}/sync` — 同步单个仓库
- 支持通过 `SERVER_{PROVIDER}_ORGS` 进行组织过滤

### API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/auth/{provider}/login` | 重定向到 OAuth 授权 |
| `GET` | `/auth/{provider}/callback` | OAuth 回调，返回 JWT |
| `GET` | `/auth/{provider}/me` | 获取当前认证用户信息 |
