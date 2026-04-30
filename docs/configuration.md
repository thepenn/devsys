# Configuration Reference / 配置参考

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

DevSys is configured entirely through environment variables. Variables can be set in the shell environment or in a `.env` file in the `modules/` directory (loaded automatically via `godotenv`).

### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_DRIVER` | `mysql` | Database driver |
| `DATABASE_DATASOURCE` | `root:password@tcp(localhost:3306)/devops?charset=utf8mb4&parseTime=True&loc=Local` | Database connection string |
| `DATABASE_MAX_CONNECTIONS` | `10` | Maximum number of open connections |
| `DATABASE_SHOW_SQL` | `false` | Enable SQL query logging |

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_HOST` | `localhost:8080` | HTTP server listen address |
| `SERVER_ROOT_PATH` | `/api/v1` | API base path prefix |

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `LOG_PRETTY` | `false` | Enable pretty-printed console output |

### Pipeline

| Variable | Default | Description |
|----------|---------|-------------|
| `PIPELINE_WORKER_COUNT` | `2` | Number of concurrent pipeline workers |
| `PIPELINE_QUEUE_CAPACITY` | `128` | Maximum pipeline queue size |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_AUTH_PROVIDER` | `gitlab` | Active OAuth provider (`github`, `gitlab`, `gitee`, `gitea`) |
| `SERVER_AUTH_SESSION_SECRET` | *(empty)* | Secret key for JWT signing |
| `SERVER_AUTH_TOKEN_TTL` | `24h` | JWT token time-to-live |
| `SERVER_AUTH_STATE_TTL` | `10m` | OAuth state parameter TTL |

### GitHub

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_GITHUB` | `false` | Enable GitHub provider |
| `SERVER_GITHUB_URL` | `https://github.com` | GitHub base URL |
| `SERVER_GITHUB_API_URL` | `https://api.github.com` | GitHub API URL |
| `SERVER_GITHUB_CLIENT` | *(empty)* | OAuth client ID |
| `SERVER_GITHUB_SECRET` | *(empty)* | OAuth client secret |
| `SERVER_GITHUB_REDIRECT` | *(empty)* | OAuth redirect URL |
| `SERVER_GITHUB_SCOPES` | `read:user repo read:org` | OAuth scopes |
| `SERVER_GITHUB_ORGS` | *(empty)* | Comma-separated allowed organizations |
| `SERVER_GITHUB_INCLUDE_FORKS` | `false` | Include forked repositories |
| `SERVER_GITHUB_SKIP_VERIFY` | `false` | Skip TLS certificate verification |

### GitLab

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_GITLAB` | `true` | Enable GitLab provider |
| `SERVER_GITLAB_URL` | `https://gitlab.com` | GitLab base URL |
| `SERVER_GITLAB_CLIENT` | *(empty)* | OAuth client ID |
| `SERVER_GITLAB_SECRET` | *(empty)* | OAuth client secret |
| `SERVER_GITLAB_REDIRECT` | *(empty)* | OAuth redirect URL |
| `SERVER_GITLAB_SCOPES` | `read_user api` | OAuth scopes |
| `SERVER_GITLAB_SKIP_VERIFY` | `false` | Skip TLS certificate verification |
| `SERVER_GITLAB_ORGS` | *(empty)* | Comma-separated allowed groups |

### Gitee

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_GITEE` | `false` | Enable Gitee provider |
| `SERVER_GITEE_URL` | `https://gitee.com` | Gitee base URL |
| `SERVER_GITEE_CLIENT` | *(empty)* | OAuth client ID |
| `SERVER_GITEE_SECRET` | *(empty)* | OAuth client secret |
| `SERVER_GITEE_REDIRECT` | *(empty)* | OAuth redirect URL |
| `SERVER_GITEE_SCOPES` | `user_info projects` | OAuth scopes |
| `SERVER_GITEE_SKIP_VERIFY` | `false` | Skip TLS certificate verification |
| `SERVER_GITEE_ORGS` | *(empty)* | Comma-separated allowed organizations |

### Gitea

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_GITEA` | `false` | Enable Gitea provider |
| `SERVER_GITEA_URL` | *(empty)* | Gitea base URL |
| `SERVER_GITEA_CLIENT` | *(empty)* | OAuth client ID |
| `SERVER_GITEA_SECRET` | *(empty)* | OAuth client secret |
| `SERVER_GITEA_REDIRECT` | *(empty)* | OAuth redirect URL |
| `SERVER_GITEA_SCOPES` | `read:user user:email repo` | OAuth scopes |
| `SERVER_GITEA_SKIP_VERIFY` | `false` | Skip TLS certificate verification |
| `SERVER_GITEA_ORGS` | *(empty)* | Comma-separated allowed organizations |

### Example `.env` File

```bash
# Database
DATABASE_DRIVER=mysql
DATABASE_DATASOURCE=devops_user:secure_password@tcp(db.example.com:3306)/devops?charset=utf8mb4&parseTime=True&loc=Local
DATABASE_MAX_CONNECTIONS=20

# Server
SERVER_HOST=0.0.0.0:8080

# Auth - GitLab
SERVER_AUTH_PROVIDER=gitlab
SERVER_AUTH_SESSION_SECRET=my-very-long-secret-key
SERVER_AUTH_TOKEN_TTL=12h
SERVER_GITLAB=true
SERVER_GITLAB_URL=https://gitlab.example.com
SERVER_GITLAB_CLIENT=app-client-id
SERVER_GITLAB_SECRET=app-client-secret
SERVER_GITLAB_REDIRECT=https://devops.example.com/api/v1/auth/gitlab/callback
SERVER_GITLAB_ORGS=my-org

# Pipeline
PIPELINE_WORKER_COUNT=4
PIPELINE_QUEUE_CAPACITY=256

# Logging
LOG_LEVEL=info
LOG_PRETTY=false
```

---

<a id="中文"></a>

## 中文

DevSys 完全通过环境变量配置。变量可以在 Shell 环境中设置，也可以在 `modules/` 目录下的 `.env` 文件中定义（通过 `godotenv` 自动加载）。

### 数据库

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DATABASE_DRIVER` | `mysql` | 数据库驱动 |
| `DATABASE_DATASOURCE` | `root:password@tcp(localhost:3306)/devops?charset=utf8mb4&parseTime=True&loc=Local` | 数据库连接字符串 |
| `DATABASE_MAX_CONNECTIONS` | `10` | 最大连接数 |
| `DATABASE_SHOW_SQL` | `false` | 是否开启 SQL 日志 |

### 服务器

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_HOST` | `localhost:8080` | HTTP 服务监听地址 |
| `SERVER_ROOT_PATH` | `/api/v1` | API 基础路径前缀 |

### 日志

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LOG_LEVEL` | `info` | 日志级别 (`debug`, `info`, `warn`, `error`) |
| `LOG_PRETTY` | `false` | 启用格式化控制台输出 |

### 流水线

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PIPELINE_WORKER_COUNT` | `2` | 并发 Worker 数量 |
| `PIPELINE_QUEUE_CAPACITY` | `128` | 流水线队列最大容量 |

### 认证

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_AUTH_PROVIDER` | `gitlab` | 活跃的 OAuth 提供者 (`github`, `gitlab`, `gitee`, `gitea`) |
| `SERVER_AUTH_SESSION_SECRET` | *(空)* | JWT 签名密钥 |
| `SERVER_AUTH_TOKEN_TTL` | `24h` | JWT 令牌有效期 |
| `SERVER_AUTH_STATE_TTL` | `10m` | OAuth state 参数有效期 |

### GitHub

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_GITHUB` | `false` | 启用 GitHub 提供者 |
| `SERVER_GITHUB_URL` | `https://github.com` | GitHub 基础 URL |
| `SERVER_GITHUB_API_URL` | `https://api.github.com` | GitHub API URL |
| `SERVER_GITHUB_CLIENT` | *(空)* | OAuth 客户端 ID |
| `SERVER_GITHUB_SECRET` | *(空)* | OAuth 客户端密钥 |
| `SERVER_GITHUB_REDIRECT` | *(空)* | OAuth 回调 URL |
| `SERVER_GITHUB_SCOPES` | `read:user repo read:org` | OAuth 授权范围 |
| `SERVER_GITHUB_ORGS` | *(空)* | 允许的组织（逗号分隔） |
| `SERVER_GITHUB_INCLUDE_FORKS` | `false` | 是否包含 Fork 仓库 |
| `SERVER_GITHUB_SKIP_VERIFY` | `false` | 跳过 TLS 证书验证 |

### GitLab

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_GITLAB` | `true` | 启用 GitLab 提供者 |
| `SERVER_GITLAB_URL` | `https://gitlab.com` | GitLab 基础 URL |
| `SERVER_GITLAB_CLIENT` | *(空)* | OAuth 客户端 ID |
| `SERVER_GITLAB_SECRET` | *(空)* | OAuth 客户端密钥 |
| `SERVER_GITLAB_REDIRECT` | *(空)* | OAuth 回调 URL |
| `SERVER_GITLAB_SCOPES` | `read_user api` | OAuth 授权范围 |
| `SERVER_GITLAB_SKIP_VERIFY` | `false` | 跳过 TLS 证书验证 |
| `SERVER_GITLAB_ORGS` | *(空)* | 允许的群组（逗号分隔） |

### Gitee

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_GITEE` | `false` | 启用 Gitee 提供者 |
| `SERVER_GITEE_URL` | `https://gitee.com` | Gitee 基础 URL |
| `SERVER_GITEE_CLIENT` | *(空)* | OAuth 客户端 ID |
| `SERVER_GITEE_SECRET` | *(空)* | OAuth 客户端密钥 |
| `SERVER_GITEE_REDIRECT` | *(空)* | OAuth 回调 URL |
| `SERVER_GITEE_SCOPES` | `user_info projects` | OAuth 授权范围 |
| `SERVER_GITEE_SKIP_VERIFY` | `false` | 跳过 TLS 证书验证 |
| `SERVER_GITEE_ORGS` | *(空)* | 允许的组织（逗号分隔） |

### Gitea

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_GITEA` | `false` | 启用 Gitea 提供者 |
| `SERVER_GITEA_URL` | *(空)* | Gitea 基础 URL |
| `SERVER_GITEA_CLIENT` | *(空)* | OAuth 客户端 ID |
| `SERVER_GITEA_SECRET` | *(空)* | OAuth 客户端密钥 |
| `SERVER_GITEA_REDIRECT` | *(空)* | OAuth 回调 URL |
| `SERVER_GITEA_SCOPES` | `read:user user:email repo` | OAuth 授权范围 |
| `SERVER_GITEA_SKIP_VERIFY` | `false` | 跳过 TLS 证书验证 |
| `SERVER_GITEA_ORGS` | *(空)* | 允许的组织（逗号分隔） |

### 示例 `.env` 文件

```bash
# 数据库
DATABASE_DRIVER=mysql
DATABASE_DATASOURCE=devops_user:secure_password@tcp(db.example.com:3306)/devops?charset=utf8mb4&parseTime=True&loc=Local
DATABASE_MAX_CONNECTIONS=20

# 服务器
SERVER_HOST=0.0.0.0:8080

# 认证 - GitLab
SERVER_AUTH_PROVIDER=gitlab
SERVER_AUTH_SESSION_SECRET=my-very-long-secret-key
SERVER_AUTH_TOKEN_TTL=12h
SERVER_GITLAB=true
SERVER_GITLAB_URL=https://gitlab.example.com
SERVER_GITLAB_CLIENT=app-client-id
SERVER_GITLAB_SECRET=app-client-secret
SERVER_GITLAB_REDIRECT=https://devops.example.com/api/v1/auth/gitlab/callback
SERVER_GITLAB_ORGS=my-org

# 流水线
PIPELINE_WORKER_COUNT=4
PIPELINE_QUEUE_CAPACITY=256

# 日志
LOG_LEVEL=info
LOG_PRETTY=false
```
