# API Reference / API 参考

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

All API endpoints are prefixed with `SERVER_ROOT_PATH` (default: `/api/v1`).

An auto-generated OpenAPI specification is available at `GET /api/v1/api.json`.

### Authentication

All endpoints except health checks and login require a valid JWT token in the `Authorization` header:

```
Authorization: Bearer <jwt-token>
```

Admin endpoints additionally require the user to have admin privileges.

### Error Response Format

```json
{
  "error": "error message description"
}
```

### Health & System

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/ping` | No | Returns `{"message":"pong"}` |
| `GET` | `/health` | No | Health check with uptime |
| `GET` | `/metrics` | No | Prometheus metrics |
| `GET` | `/api.json` | No | OpenAPI specification |

### Authentication

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/auth/{provider}/login` | No | Redirect to OAuth provider |
| `GET` | `/auth/{provider}/callback` | No | OAuth callback, returns JWT token |
| `GET` | `/auth/{provider}/me` | Yes | Get current user info |

### System / Credentials

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/sys/rsa/public-key` | No | Get RSA public key for client-side encryption |
| `GET` | `/sys/certificates` | Admin | List certificates (paginated) |
| `POST` | `/sys/certificates` | Admin | Create a certificate |
| `GET` | `/sys/certificates/{id}` | Admin | Get certificate detail |
| `PUT` | `/sys/certificates/{id}` | Admin | Update a certificate |
| `DELETE` | `/sys/certificates/{id}` | Admin | Delete a certificate |

**Query Parameters for `GET /sys/certificates`:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number (default: 1) |
| `per_page` | int | Items per page (default: 20, max: 100) |
| `type` | string | Filter by certificate type |
| `name` | string | Filter by name |

**Query Parameters for `GET /sys/certificates/{id}`:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `reveal` | bool | If `true`, return unmasked secrets |

**Certificate Types:**

| Type | Description |
|------|-------------|
| `git` | Git credentials |
| `docker` | Docker registry credentials |
| `mysql` | MySQL connection credentials |
| `ldap` | LDAP configuration |
| `kubernetes` | Kubeconfig for cluster access |

### Repositories

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/repos` | Yes | List repositories |
| `POST` | `/repos/sync` | Yes | Sync all repositories from forge |
| `POST` | `/repos/{repo_id}/sync` | Yes | Sync a single repository |

**Query Parameters for `GET /repos`:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number |
| `per_page` | int | Items per page |
| `search` | string | Search by name |
| `synced` | string | Filter: `true`/`false` |

### Pipelines

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/repos/{repo_id}/pipeline/runs` | Yes | List pipeline runs |
| `GET` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}` | Yes | Get run detail |
| `POST` | `/repos/{repo_id}/pipeline/run` | Yes | Trigger manual run |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/cancel` | Yes | Cancel a run |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/steps/{step_id}/approval` | Yes | Submit approval |
| `GET` | `/repos/{repo_id}/pipeline/config` | Yes | Get YAML config |
| `PUT` | `/repos/{repo_id}/pipeline/config` | Yes | Update YAML config |
| `GET` | `/repos/{repo_id}/pipeline/settings` | Yes | Get settings |
| `PUT` | `/repos/{repo_id}/pipeline/settings` | Yes | Update settings |

**Trigger Pipeline Request Body:**

```json
{
  "branch": "main",
  "commit": "",
  "variables": {"KEY": "value"}
}
```

**Approval Request Body:**

```json
{
  "action": "approve",
  "comment": "LGTM"
}
```

### Kubernetes (Admin Only)

All K8s endpoints are under `/admin/k8s` and require admin privileges.

#### Clusters & Namespaces

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/k8s/clusters` | List clusters |
| `GET` | `/admin/k8s/clusters/{id}/namespaces` | List namespaces |

#### Resources

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/k8s/clusters/{id}/resources` | List resources |
| `GET` | `/admin/k8s/clusters/{id}/resources/object` | Get single resource |
| `POST` | `/admin/k8s/clusters/{id}/resources/apply` | Apply manifest |
| `DELETE` | `/admin/k8s/clusters/{id}/resources/object` | Delete resource |
| `GET` | `/admin/k8s/clusters/{id}/resources/events` | List events |

**Resource Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `group` | string | API group (e.g., `apps`) |
| `version` | string | API version (e.g., `v1`) |
| `resource` | string | Resource type (e.g., `deployments`) — **required** |
| `namespace` | string | Namespace |
| `name` | string | Resource name (for single resource) |
| `labelSelector` | string | Label selector |
| `fieldSelector` | string | Field selector |

#### Workloads

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/pods` | List pods |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/details` | Get details |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/history` | Revision history |
| `POST` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/rollback` | Rollback |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/logs` | Aggregate logs |

#### Pods

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/k8s/clusters/{id}/pods/logs` | Fetch pod logs |
| `POST` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec` | Execute command |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec/stream` | WebSocket terminal |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/logs/stream` | WebSocket log stream |

#### Deployments

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/aggregate` | Aggregate deployment |
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/pods` | List deployment pods |

### Static Assets

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | SPA index page |
| `GET` | `/favicon.ico` | Favicon |
| `GET` | `/static/{filename}` | Static assets |

---

<a id="中文"></a>

## 中文

所有 API 端点以 `SERVER_ROOT_PATH`（默认：`/api/v1`）为前缀。

自动生成的 OpenAPI 规范可通过 `GET /api/v1/api.json` 获取。

### 认证方式

除健康检查和登录外，所有端点需要在 `Authorization` 头中携带有效的 JWT 令牌：

```
Authorization: Bearer <jwt-token>
```

管理员端点额外要求用户具有管理员权限。

### 错误响应格式

```json
{
  "error": "错误信息描述"
}
```

### 健康检查 & 系统

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/ping` | 否 | 返回 `{"message":"pong"}` |
| `GET` | `/health` | 否 | 健康检查，含运行时间 |
| `GET` | `/metrics` | 否 | Prometheus 指标 |
| `GET` | `/api.json` | 否 | OpenAPI 规范 |

### 认证

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/auth/{provider}/login` | 否 | 重定向到 OAuth 提供者 |
| `GET` | `/auth/{provider}/callback` | 否 | OAuth 回调，返回 JWT 令牌 |
| `GET` | `/auth/{provider}/me` | 是 | 获取当前用户信息 |

### 系统 / 凭证

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/sys/rsa/public-key` | 否 | 获取 RSA 公钥用于客户端加密 |
| `GET` | `/sys/certificates` | 管理员 | 列出凭证（分页） |
| `POST` | `/sys/certificates` | 管理员 | 创建凭证 |
| `GET` | `/sys/certificates/{id}` | 管理员 | 获取凭证详情 |
| `PUT` | `/sys/certificates/{id}` | 管理员 | 更新凭证 |
| `DELETE` | `/sys/certificates/{id}` | 管理员 | 删除凭证 |

**`GET /sys/certificates` 查询参数：**

| 参数 | 类型 | 说明 |
|------|------|------|
| `page` | int | 页码（默认 1） |
| `per_page` | int | 每页条数（默认 20，最大 100） |
| `type` | string | 按类型筛选 |
| `name` | string | 按名称筛选 |

**凭证类型：**

| 类型 | 说明 |
|------|------|
| `git` | Git 凭证 |
| `docker` | Docker 仓库凭证 |
| `mysql` | MySQL 连接凭证 |
| `ldap` | LDAP 配置 |
| `kubernetes` | 集群访问的 Kubeconfig |

### 仓库

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/repos` | 是 | 列出仓库 |
| `POST` | `/repos/sync` | 是 | 从平台同步所有仓库 |
| `POST` | `/repos/{repo_id}/sync` | 是 | 同步单个仓库 |

### 流水线

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/repos/{repo_id}/pipeline/runs` | 是 | 列出流水线运行记录 |
| `GET` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}` | 是 | 获取运行详情 |
| `POST` | `/repos/{repo_id}/pipeline/run` | 是 | 手动触发运行 |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/cancel` | 是 | 取消运行 |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/steps/{step_id}/approval` | 是 | 提交审批 |
| `GET` | `/repos/{repo_id}/pipeline/config` | 是 | 获取 YAML 配置 |
| `PUT` | `/repos/{repo_id}/pipeline/config` | 是 | 更新 YAML 配置 |
| `GET` | `/repos/{repo_id}/pipeline/settings` | 是 | 获取设置 |
| `PUT` | `/repos/{repo_id}/pipeline/settings` | 是 | 更新设置 |

### Kubernetes（仅管理员）

所有 K8s 端点在 `/admin/k8s` 下，需要管理员权限。

#### 集群 & 命名空间

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/admin/k8s/clusters` | 列出集群 |
| `GET` | `/admin/k8s/clusters/{id}/namespaces` | 列出命名空间 |

#### 资源

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/admin/k8s/clusters/{id}/resources` | 列出资源 |
| `GET` | `/admin/k8s/clusters/{id}/resources/object` | 获取单个资源 |
| `POST` | `/admin/k8s/clusters/{id}/resources/apply` | 应用清单 |
| `DELETE` | `/admin/k8s/clusters/{id}/resources/object` | 删除资源 |
| `GET` | `/admin/k8s/clusters/{id}/resources/events` | 列出事件 |

#### 工作负载

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/pods` | 列出 Pod |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/details` | 获取详情 |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/history` | 修订历史 |
| `POST` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/rollback` | 回滚 |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/logs` | 聚合日志 |

#### Pod

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/admin/k8s/clusters/{id}/pods/logs` | 获取 Pod 日志 |
| `POST` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec` | 执行命令 |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec/stream` | WebSocket 终端 |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/logs/stream` | WebSocket 日志流 |

#### Deployment

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/aggregate` | 聚合部署信息 |
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/pods` | 列出部署的 Pod |

### 静态资源

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/` | SPA 首页 |
| `GET` | `/favicon.ico` | 图标 |
| `GET` | `/static/{filename}` | 静态资源 |
