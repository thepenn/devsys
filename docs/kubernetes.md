# Kubernetes Management Module / Kubernetes 管理模块

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

### Overview

The Kubernetes module provides multi-cluster management capabilities through a dynamic client approach. Cluster connections are derived from Kubernetes-type certificates stored in the credential management system. All K8s endpoints require admin privileges.

**Source:** `modules/service/k8s/`, `modules/routers/k8s.go`

### Architecture

```
┌───────────────────────────────┐
│        Frontend (React)        │
│  K8s Pages │ xterm.js Terminal │
└──────────────┬────────────────┘
               │ REST / WebSocket
┌──────────────▼────────────────┐
│         K8s Router             │
│   (admin middleware required)  │
└──────────────┬────────────────┘
               │
┌──────────────▼────────────────┐
│         K8s Service            │
│  ┌─────────────────────────┐  │
│  │ Certificate Store       │  │
│  │ (type: "kubernetes")    │  │
│  └────────────┬────────────┘  │
│               │ kubeconfig    │
│  ┌────────────▼────────────┐  │
│  │ Dynamic Client          │  │
│  │ + Discovery Client      │  │
│  └────────────┬────────────┘  │
└───────────────┼───────────────┘
                │
    ┌───────────┼───────────┐
    ▼           ▼           ▼
┌────────┐ ┌────────┐ ┌────────┐
│Cluster1│ │Cluster2│ │ClusterN│
└────────┘ └────────┘ └────────┘
```

### Cluster Registration

Clusters are registered by creating a certificate of type `kubernetes` via the credential management API:

```
POST /api/v1/sys/certificates
Content-Type: application/json

{
  "name": "production-cluster",
  "type": "kubernetes",
  "config": {
    "kubeconfig": "<base64-encoded kubeconfig content>"
  }
}
```

The K8s service reads all `kubernetes`-type certificates and builds client instances from the embedded kubeconfig data.

### Resource Management

The module uses the Kubernetes dynamic client with server-side API discovery. This enables management of any resource type — including Custom Resource Definitions (CRDs) — without generated client code.

**List Resources:**

```
GET /api/v1/admin/k8s/clusters/{cluster_id}/resources?resource=deployments&namespace=default&version=v1&group=apps
```

**Get Single Resource:**

```
GET /api/v1/admin/k8s/clusters/{cluster_id}/resources/object?resource=deployments&namespace=default&name=my-app&version=v1&group=apps
```

**Apply Manifest:**

```
POST /api/v1/admin/k8s/clusters/{cluster_id}/resources/apply
Content-Type: application/json

{
  "manifest": "apiVersion: apps/v1\nkind: Deployment\n..."
}
```

**Delete Resource:**

```
DELETE /api/v1/admin/k8s/clusters/{cluster_id}/resources/object
Content-Type: application/json

{
  "group": "apps",
  "version": "v1",
  "resource": "deployments",
  "namespace": "default",
  "name": "my-app"
}
```

### Workload Operations

Specialized endpoints for workload lifecycle management:

| Operation | Method | Path |
|-----------|--------|------|
| List pods | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/pods` |
| Get details | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/details` |
| View history | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/history` |
| Rollback | `POST` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/rollback` |
| Aggregate logs | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/logs` |

**Rollback Example:**

```
POST /api/v1/admin/k8s/clusters/1/workloads/Deployment/default/my-app/rollback
Content-Type: application/json

{
  "revision": 3
}
```

### Pod Operations

| Operation | Method | Path |
|-----------|--------|------|
| Fetch logs | `GET` | `/clusters/{id}/pods/logs?namespace=...&pod=...&container=...` |
| One-shot exec | `POST` | `/clusters/{id}/pods/{ns}/{name}/exec` |
| Interactive terminal | `GET` (WebSocket) | `/clusters/{id}/pods/{ns}/{name}/exec/stream` |
| Log stream | `GET` (WebSocket) | `/clusters/{id}/pods/{ns}/{name}/logs/stream` |

### WebSocket Terminal

The interactive terminal uses WebSocket to connect the frontend xterm.js terminal to a Kubernetes pod shell via `remotecommand`.

**Connection:**

```
ws://host/api/v1/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec/stream?shell=/bin/bash&container=main
```

**Terminal Control Frames (JSON over TextMessage):**

```json
{"type": "resize", "cols": 120, "rows": 40}
```

```json
{"type": "close"}
```

Binary WebSocket messages carry stdin/stdout data directly.

### WebSocket Log Streaming

Real-time pod log streaming via WebSocket:

```
ws://host/api/v1/admin/k8s/clusters/{id}/pods/{ns}/{name}/logs/stream?container=main&tail=100
```

### Events

```
GET /api/v1/admin/k8s/clusters/{id}/resources/events?namespace=default&kind=Pod&name=my-pod&page=1&perPage=20
```

### Full API Reference

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/k8s/clusters` | List clusters |
| `GET` | `/admin/k8s/clusters/{id}/namespaces` | List namespaces |
| `GET` | `/admin/k8s/clusters/{id}/resources` | List resources (dynamic) |
| `GET` | `/admin/k8s/clusters/{id}/resources/object` | Get single resource |
| `POST` | `/admin/k8s/clusters/{id}/resources/apply` | Apply manifest |
| `DELETE` | `/admin/k8s/clusters/{id}/resources/object` | Delete resource |
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/aggregate` | Aggregate deployment info |
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/pods` | List deployment pods |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/pods` | List workload pods |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/details` | Workload details |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/history` | Workload history |
| `POST` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/rollback` | Rollback workload |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/logs` | Workload logs |
| `GET` | `/admin/k8s/clusters/{id}/resources/events` | List events |
| `GET` | `/admin/k8s/clusters/{id}/pods/logs` | Fetch pod logs |
| `POST` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec` | One-shot exec |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec/stream` | WebSocket terminal |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/logs/stream` | WebSocket log stream |

---

<a id="中文"></a>

## 中文

### 概述

Kubernetes 模块通过动态客户端方式提供多集群管理能力。集群连接来源于凭证管理系统中存储的 Kubernetes 类型证书。所有 K8s 端点需要管理员权限。

**源码：** `modules/service/k8s/`、`modules/routers/k8s.go`

### 架构

```
┌───────────────────────────────┐
│        前端 (React)            │
│  K8s 页面 │ xterm.js 终端     │
└──────────────┬────────────────┘
               │ REST / WebSocket
┌──────────────▼────────────────┐
│         K8s 路由               │
│   (需要管理员中间件)            │
└──────────────┬────────────────┘
               │
┌──────────────▼────────────────┐
│         K8s 服务               │
│  ┌─────────────────────────┐  │
│  │ 凭证存储                 │  │
│  │ (type: "kubernetes")    │  │
│  └────────────┬────────────┘  │
│               │ kubeconfig    │
│  ┌────────────▼────────────┐  │
│  │ 动态客户端               │  │
│  │ + API 发现客户端         │  │
│  └────────────┬────────────┘  │
└───────────────┼───────────────┘
                │
    ┌───────────┼───────────┐
    ▼           ▼           ▼
┌────────┐ ┌────────┐ ┌────────┐
│ 集群 1 │ │ 集群 2 │ │ 集群 N │
└────────┘ └────────┘ └────────┘
```

### 集群注册

通过凭证管理 API 创建 `kubernetes` 类型的凭证来注册集群：

```
POST /api/v1/sys/certificates
Content-Type: application/json

{
  "name": "production-cluster",
  "type": "kubernetes",
  "config": {
    "kubeconfig": "<base64 编码的 kubeconfig 内容>"
  }
}
```

K8s 服务读取所有 `kubernetes` 类型的凭证，并从嵌入的 kubeconfig 数据构建客户端实例。

### 资源管理

模块使用 Kubernetes 动态客户端配合服务端 API 发现。这使得无需生成客户端代码即可管理任何资源类型，包括自定义资源定义 (CRD)。

**列出资源：**

```
GET /api/v1/admin/k8s/clusters/{cluster_id}/resources?resource=deployments&namespace=default&version=v1&group=apps
```

**获取单个资源：**

```
GET /api/v1/admin/k8s/clusters/{cluster_id}/resources/object?resource=deployments&namespace=default&name=my-app&version=v1&group=apps
```

**应用清单：**

```
POST /api/v1/admin/k8s/clusters/{cluster_id}/resources/apply
Content-Type: application/json

{
  "manifest": "apiVersion: apps/v1\nkind: Deployment\n..."
}
```

**删除资源：**

```
DELETE /api/v1/admin/k8s/clusters/{cluster_id}/resources/object
Content-Type: application/json

{
  "group": "apps",
  "version": "v1",
  "resource": "deployments",
  "namespace": "default",
  "name": "my-app"
}
```

### 工作负载操作

工作负载生命周期管理的专用端点：

| 操作 | 方法 | 路径 |
|------|------|------|
| 列出 Pod | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/pods` |
| 获取详情 | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/details` |
| 查看历史 | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/history` |
| 回滚 | `POST` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/rollback` |
| 聚合日志 | `GET` | `/clusters/{id}/workloads/{kind}/{ns}/{name}/logs` |

**回滚示例：**

```
POST /api/v1/admin/k8s/clusters/1/workloads/Deployment/default/my-app/rollback
Content-Type: application/json

{
  "revision": 3
}
```

### Pod 操作

| 操作 | 方法 | 路径 |
|------|------|------|
| 获取日志 | `GET` | `/clusters/{id}/pods/logs?namespace=...&pod=...&container=...` |
| 单次执行 | `POST` | `/clusters/{id}/pods/{ns}/{name}/exec` |
| 交互式终端 | `GET` (WebSocket) | `/clusters/{id}/pods/{ns}/{name}/exec/stream` |
| 日志流 | `GET` (WebSocket) | `/clusters/{id}/pods/{ns}/{name}/logs/stream` |

### WebSocket 终端

交互式终端使用 WebSocket 将前端 xterm.js 终端通过 `remotecommand` 连接到 Kubernetes Pod Shell。

**连接：**

```
ws://host/api/v1/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec/stream?shell=/bin/bash&container=main
```

**终端控制帧（JSON 格式，TextMessage 类型）：**

```json
{"type": "resize", "cols": 120, "rows": 40}
```

```json
{"type": "close"}
```

二进制 WebSocket 消息直接传输 stdin/stdout 数据。

### WebSocket 日志流

通过 WebSocket 实时流式传输 Pod 日志：

```
ws://host/api/v1/admin/k8s/clusters/{id}/pods/{ns}/{name}/logs/stream?container=main&tail=100
```

### 事件查询

```
GET /api/v1/admin/k8s/clusters/{id}/resources/events?namespace=default&kind=Pod&name=my-pod&page=1&perPage=20
```

### 完整 API 参考

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/admin/k8s/clusters` | 列出集群 |
| `GET` | `/admin/k8s/clusters/{id}/namespaces` | 列出命名空间 |
| `GET` | `/admin/k8s/clusters/{id}/resources` | 列出资源（动态） |
| `GET` | `/admin/k8s/clusters/{id}/resources/object` | 获取单个资源 |
| `POST` | `/admin/k8s/clusters/{id}/resources/apply` | 应用清单 |
| `DELETE` | `/admin/k8s/clusters/{id}/resources/object` | 删除资源 |
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/aggregate` | 聚合部署信息 |
| `GET` | `/admin/k8s/clusters/{id}/deployments/{ns}/{name}/pods` | 列出部署的 Pod |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/pods` | 列出工作负载的 Pod |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/details` | 工作负载详情 |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/history` | 工作负载历史 |
| `POST` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/rollback` | 回滚工作负载 |
| `GET` | `/admin/k8s/clusters/{id}/workloads/{kind}/{ns}/{name}/logs` | 工作负载日志 |
| `GET` | `/admin/k8s/clusters/{id}/resources/events` | 列出事件 |
| `GET` | `/admin/k8s/clusters/{id}/pods/logs` | 获取 Pod 日志 |
| `POST` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec` | 单次执行命令 |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/exec/stream` | WebSocket 终端 |
| `GET` | `/admin/k8s/clusters/{id}/pods/{ns}/{name}/logs/stream` | WebSocket 日志流 |
