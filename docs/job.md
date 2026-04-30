# Standalone Pipeline Jobs / 独立 Pipeline Job

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

### Overview

A `PipelineJob` is a pipeline definition that lives **outside any project repository**. It is the right tool when:

- The work is not tied to a specific source repo (operations scripts, periodic cleanup, broadcast deploys).
- You want a single shared YAML that can be triggered manually, on a schedule, or by another tool.
- You need to reuse the same step / approval / docker runtime as repo pipelines without inventing a new executor.

Jobs share the execution engine with repo pipelines (same queue, same Docker runtime, same approval flow). The differences:

| Aspect | Repo pipeline | Standalone Job |
|--------|---------------|----------------|
| Source of YAML | `RepoPipelineConfig.content` or referenced `PipelineTemplate` | `PipelineJob.content` (always inline) |
| Git workspace | Always a workspace dir; clone URL comes from the linked Repo | Optional. When `git_enabled=true`, the clone URL & credentials are exposed as env vars; commands run `git clone` themselves. When disabled, only an empty workspace dir is provided. |
| Trigger sources | Manual UI / API + repo cron | Manual UI / API + Job cron |
| Run history | `pipelines` rows with `owner_kind=repo` and `repo_id=<id>` | `pipelines` rows with `owner_kind=job` and `job_id=<id>` |
| Permissions | `project:read / write` + `pipeline:trigger` | `pipeline_job:read / write / trigger` |

**Source:** `modules/service/pipeline/job/`, `modules/routers/sys_pipeline_jobs.go`

### Quick start

1. Open `通用 Pipeline > 独立 Job` in the sidebar.
2. Click **新建 Job**, fill `name` (immutable identifier, used for RBAC/log correlation), display name, and description.
3. The editor opens with five tabs:
   - **YAML** — pipeline definition; same spec as repo pipelines (see [pipeline.md](pipeline.md)).
   - **Git 配置** — optional clone URL / branch / credential.
   - **默认变量** — kv pairs merged into every run; overridden per-run from the trigger modal.
   - **调度 (Cron)** — list of cron expressions (see below).
   - **运行历史** — past runs with status, click to drill into the same run-detail view used for repo builds.
4. **保存** persists, **立即运行** triggers a run immediately and navigates to the run detail page.

### YAML configuration

The YAML grammar is identical to repo pipelines — see the **YAML spec** section in [pipeline.md](pipeline.md). Minimal example:

```yaml
name: nightly-cleanup
workspace: /workspace
steps:
  trim-cache:
    image: alpine:3.20
    commands:
      - echo "Cleaning up at $(date)"
      - find /tmp -mtime +7 -delete
```

When `git_enabled=true`, the runtime injects:

| Env var | Source |
|---------|--------|
| `JOB_GIT_CLONE_URL` | `git_clone_url` |
| `JOB_GIT_USERNAME` | Decrypted username from the linked git certificate |
| `JOB_GIT_PASSWORD` | Decrypted password / token from the linked git certificate |

A typical bootstrap step:

```yaml
steps:
  fetch:
    image: alpine/git:2.45.2
    commands:
      - git clone "https://${JOB_GIT_USERNAME}:${JOB_GIT_PASSWORD}@${JOB_GIT_CLONE_URL#https://}" repo
      - cd repo && git checkout "${JOB_GIT_BRANCH:-main}"
```

### Default variables and `${VAR}` substitution

Variables are merged in this order (later wins):

```
PipelineJob.variables  ⊕  trigger options.variables  ⊕  builtin (CRON_*, JOB_GIT_*)
```

The merged map is then used for two purposes:

1. **YAML pre-processing**: `${VAR}` and `${VAR:-default}` placeholders inside the YAML are substituted before `spec.Parse`. This lets a single template-like YAML drive different behaviours per run.
2. **Step environment**: every step receives the merged map as additional env vars.

**Certificate fallback.** When a placeholder is missing from the merged map, the renderer looks up a Certificate by `name == VAR` and substitutes its primary value (`git` → `password` / token, `docker` → `repo` / registry URL). Effective resolution priority:

```
trigger options.variables  >  PipelineJob.variables  >  Certificate by name  >  ${VAR:-default}  >  ""
```

So `${gitlab-token}` defaults to the `gitlab-token` git certificate's token, `${aliyun_docker_registry}` to the docker certificate's registry URL — both can still be overridden per-run from the trigger modal. Compound certificates' other fields (e.g. docker `username` / `password`) are not auto-injected via `${VAR}`; declare them in the step with `certificates:` / `secrets:` if you need them as separate env vars. See [pipeline.md](pipeline.md) for the same rules applied to project / template pipelines.

### Cron schedules

Cron schedules are stored as a list of standard 5-field expressions (`minute hour day-of-month month day-of-week`). The backend uses [`github.com/gdgvda/cron`](https://github.com/gdgvda/cron) and runs in the **server timezone**.

**Examples:**

| Expression | Meaning |
|------------|---------|
| `0 2 * * *` | Every day at 02:00 |
| `*/15 * * * *` | Every 15 minutes |
| `0 9 * * 1-5` | 09:00 every weekday (Mon-Fri) |
| `0 0 1 * *` | Midnight on the 1st of every month |
| `30 3 * * 0` | 03:30 every Sunday |

**Behaviour:**

- Multiple expressions can coexist for the same Job (the array is the source of truth). Removing an entry cancels that schedule on the next save.
- Schedule changes take effect immediately — the in-memory scheduler is refreshed in the same write transaction (see `RefreshJobCronEntries` in [modules/service/pipeline/service.go](../modules/service/pipeline/service.go)).
- A cron-triggered run is identical to a manual run, except:
  - The `event` column on `pipelines` is `cron` (vs. `manual`).
  - Three extra variables are injected: `CRON_EXPRESSION`, `CRON_TRIGGERED_AT` (RFC3339 UTC), `CRON_TRIGGERED_BY` (always `cron`).
  - The display title is `定时任务 - <expr>`.
- Seconds are **not** supported (5-field format only).

### Run history and cancel

The editor's **运行历史** tab lists past runs. Click a row to open the same detail view used for repo builds (workflows, steps, log streaming, approval modal). Active runs can be cancelled from the detail view or via API:

```
POST /api/v1/pipeline-jobs/{id}/runs/{run_id}/cancel?reason=manual
```

### API endpoints

All endpoints sit under `/api/v1/pipeline-jobs`.

| Method | Path | Label |
|--------|------|-------|
| `GET` | `/pipeline-jobs` | `pipeline_job:read` |
| `POST` | `/pipeline-jobs` | `pipeline_job:write` |
| `GET` | `/pipeline-jobs/{id}` | `pipeline_job:read` |
| `PUT` | `/pipeline-jobs/{id}` | `pipeline_job:write` |
| `DELETE` | `/pipeline-jobs/{id}` | `pipeline_job:write` |
| `POST` | `/pipeline-jobs/{id}/run` | `pipeline_job:trigger` |
| `GET` | `/pipeline-jobs/{id}/runs` | `pipeline_job:read` |
| `GET` | `/pipeline-jobs/{id}/runs/{run_id}` | `pipeline_job:read` |
| `POST` | `/pipeline-jobs/{id}/runs/{run_id}/cancel` | `pipeline_job:trigger` |
| `POST` | `/pipeline-jobs/{id}/runs/{run_id}/steps/{step_id}/approval` | `pipeline_job:trigger` |

**Update payload** supports partial mutation. `cron_schedules: null` keeps existing; `cron_schedules: []` clears all schedules:

```json
PUT /api/v1/pipeline-jobs/12
{
  "content": "name: nightly\nsteps:\n  hello:\n    image: alpine\n    commands: [\"echo hi\"]\n",
  "cron_schedules": ["0 2 * * *", "0 14 * * *"],
  "variables": {"DEPLOY_ENV": "staging"}
}
```

**Manual trigger** body is optional; both `branch` and `variables` may be omitted:

```json
POST /api/v1/pipeline-jobs/12/run
{
  "branch": "main",
  "variables": {"DRY_RUN": "true"}
}
```

### Permissions

| Label | Default roles | Capability |
|-------|---------------|-----------|
| `pipeline_job:read` | developer, ops, admin | List & view jobs and run history |
| `pipeline_job:write` | ops, admin | Create / edit / delete jobs (incl. cron) |
| `pipeline_job:trigger` | developer, ops, admin | Run a job, cancel a run, submit approval |

`superadmin` automatically inherits everything via the `*` wildcard label.

### Limitations

- **Single-instance cron**: schedules run on every backend instance that holds the in-memory scheduler. For multi-instance deployments you must coordinate externally (load balancer pinning, or pick a leader). The codebase doesn't yet ship a leader-election layer.
- **No DAG / cross-job dependency**: each run is self-contained. Use `approval` steps inside a Job for sequencing within one run.
- **Workspace is not auto-cloned**: even with `git_enabled=true`, the runtime only injects credentials; commands must call `git clone` themselves. This avoids surprising behaviour for jobs that don't need a checkout at all.
- **Server-local cron timezone**: not yet configurable per-job.
- **5-field cron only**: no seconds support.

---

<a id="中文"></a>

## 中文

### 概述

`PipelineJob` 是**不依赖任何项目仓库**的 pipeline 定义，适用于：

- 与具体源代码仓库无关的工作（运维脚本、定期清理、批量部署）。
- 想要一份共享的 YAML，能被手动 / 定时 / 第三方工具触发。
- 想复用 repo 流水线那一套 step / 审批 / Docker 运行时，而不用再造一个执行器。

Job 与项目流水线共享同一个执行引擎（同一队列、同一 Docker 运行时、同一审批流），区别在于：

| 维度 | 项目流水线 | 独立 Job |
|------|-----------|---------|
| YAML 来源 | `RepoPipelineConfig.content` 或引用的 `PipelineTemplate` | `PipelineJob.content`（始终 inline）|
| Git workspace | 必有；clone URL 来自项目仓库 | 可选。`git_enabled=true` 时把 clone URL 和凭证注入 env，由 commands 自行 `git clone`；关闭时只准备一个空 workspace 目录 |
| 触发来源 | UI / API + 项目 cron | UI / API + Job cron |
| 运行历史 | `pipelines` 行 `owner_kind=repo`、`repo_id=<id>` | `pipelines` 行 `owner_kind=job`、`job_id=<id>` |
| 权限 | `project:read / write` + `pipeline:trigger` | `pipeline_job:read / write / trigger` |

**源码：** `modules/service/pipeline/job/`、`modules/routers/sys_pipeline_jobs.go`

### 快速开始

1. 侧边栏点开 `通用 Pipeline > 独立 Job`。
2. 点 **新建 Job**，填 `name`（不可改的标识，用于 RBAC / 日志关联）、显示名、描述。
3. 进入编辑器，可见五个 Tab：
   - **YAML** — pipeline 定义；语法与项目流水线相同（详见 [pipeline.md](pipeline.md)）。
   - **Git 配置** — 可选的 clone URL / 分支 / 凭证。
   - **默认变量** — 每次运行都会注入的 kv，触发弹窗里可再覆盖。
   - **调度 (Cron)** — cron 表达式列表（详见下文）。
   - **运行历史** — 历史运行列表，点进去复用与项目构建一致的运行详情页。
4. **保存** 持久化，**立即运行** 触发一次并跳转到运行详情。

### YAML 配置

YAML 语法与项目流水线完全一致 — 详见 [pipeline.md](pipeline.md) 的 YAML spec 部分。最小示例：

```yaml
name: nightly-cleanup
workspace: /workspace
steps:
  trim-cache:
    image: alpine:3.20
    commands:
      - echo "Cleaning up at $(date)"
      - find /tmp -mtime +7 -delete
```

启用 `git_enabled=true` 时，运行时会注入：

| 环境变量 | 来源 |
|----------|------|
| `JOB_GIT_CLONE_URL` | `git_clone_url` |
| `JOB_GIT_USERNAME` | 关联 git 凭证解密后的用户名 |
| `JOB_GIT_PASSWORD` | 关联 git 凭证解密后的密码 / token |

典型的 bootstrap step：

```yaml
steps:
  fetch:
    image: alpine/git:2.45.2
    commands:
      - git clone "https://${JOB_GIT_USERNAME}:${JOB_GIT_PASSWORD}@${JOB_GIT_CLONE_URL#https://}" repo
      - cd repo && git checkout "${JOB_GIT_BRANCH:-main}"
```

### 默认变量与 `${VAR}` 替换

变量按以下顺序合并（后者覆盖前者）：

```
PipelineJob.variables  ⊕  trigger options.variables  ⊕  内置 (CRON_*, JOB_GIT_*)
```

合并后的字典有两种用途：

1. **YAML 预处理**：YAML 中的 `${VAR}` 与 `${VAR:-default}` 占位符在 `spec.Parse` 之前完成替换。这样一份模板化 YAML 就能为不同运行参数化。
2. **Step 环境变量**：每个 step 都会拿到合并后的字典作为额外 env。

**凭证默认值回填**：当合并后的字典没命中某个占位符时，渲染器会去 Certificate 仓库按 `name == VAR` 查询并回填主值（`git` → `password` / token，`docker` → `repo` / registry URL）。最终优先级：

```
trigger options.variables  >  PipelineJob.variables  >  凭证仓库按名匹配  >  ${VAR:-default}  >  ""
```

也就是 `${gitlab-token}` 默认会拿 `gitlab-token` 这个 git 凭证里的 token，`${aliyun_docker_registry}` 默认拿 docker 凭证里的 registry URL — 两者都仍可在每次触发时被覆盖。复合凭证的其它字段（如 docker `username` / `password`）不通过 `${VAR}` 自动注入，要把它们用成独立 env 仍需在 step 里 `certificates:` / `secrets:` 显式声明。本节规则与项目 / 模板流水线一致，详见 [pipeline.md](pipeline.md)。

### Cron 调度

Cron 表达式以列表形式存储，使用标准 5 字段（`分 时 日 月 周`）。后端基于 [`github.com/gdgvda/cron`](https://github.com/gdgvda/cron)，运行在**服务器时区**。

**示例：**

| 表达式 | 含义 |
|--------|------|
| `0 2 * * *` | 每天 02:00 |
| `*/15 * * * *` | 每 15 分钟 |
| `0 9 * * 1-5` | 工作日 09:00（周一到周五）|
| `0 0 1 * *` | 每月 1 号 00:00 |
| `30 3 * * 0` | 每周日 03:30 |

**行为：**

- 同一个 Job 可以同时挂多条表达式（数组就是 source of truth）；移除一项保存后下一秒就取消。
- 修改即生效 — 内存中的调度器在写库的同时刷新（见 [modules/service/pipeline/service.go](../modules/service/pipeline/service.go) 的 `RefreshJobCronEntries`）。
- Cron 触发的 run 与手动触发完全一致，区别仅在：
  - `pipelines.event` 为 `cron`（手动是 `manual`）。
  - 自动注入三个额外变量：`CRON_EXPRESSION`、`CRON_TRIGGERED_AT`（RFC3339 UTC）、`CRON_TRIGGERED_BY`（始终为 `cron`）。
  - 显示标题为 `定时任务 - <表达式>`。
- **不支持** 秒级（仅 5 字段格式）。

### 运行历史与取消

编辑器的 **运行历史** Tab 列出过往运行。点击某行打开运行详情（与项目构建详情共用同一个组件，含 workflows / steps / 日志流 / 审批弹窗）。运行中状态可在详情页或 API 里取消：

```
POST /api/v1/pipeline-jobs/{id}/runs/{run_id}/cancel?reason=manual
```

### API 速查

全部挂在 `/api/v1/pipeline-jobs` 下：

| 方法 | 路径 | 标签 |
|------|------|------|
| `GET` | `/pipeline-jobs` | `pipeline_job:read` |
| `POST` | `/pipeline-jobs` | `pipeline_job:write` |
| `GET` | `/pipeline-jobs/{id}` | `pipeline_job:read` |
| `PUT` | `/pipeline-jobs/{id}` | `pipeline_job:write` |
| `DELETE` | `/pipeline-jobs/{id}` | `pipeline_job:write` |
| `POST` | `/pipeline-jobs/{id}/run` | `pipeline_job:trigger` |
| `GET` | `/pipeline-jobs/{id}/runs` | `pipeline_job:read` |
| `GET` | `/pipeline-jobs/{id}/runs/{run_id}` | `pipeline_job:read` |
| `POST` | `/pipeline-jobs/{id}/runs/{run_id}/cancel` | `pipeline_job:trigger` |
| `POST` | `/pipeline-jobs/{id}/runs/{run_id}/steps/{step_id}/approval` | `pipeline_job:trigger` |

**更新接口** 支持局部修改。`cron_schedules: null` 表示不动；`cron_schedules: []` 表示清空所有调度：

```json
PUT /api/v1/pipeline-jobs/12
{
  "content": "name: nightly\nsteps:\n  hello:\n    image: alpine\n    commands: [\"echo hi\"]\n",
  "cron_schedules": ["0 2 * * *", "0 14 * * *"],
  "variables": {"DEPLOY_ENV": "staging"}
}
```

**手动触发** body 可以为空，`branch` 与 `variables` 都可省略：

```json
POST /api/v1/pipeline-jobs/12/run
{
  "branch": "main",
  "variables": {"DRY_RUN": "true"}
}
```

### 权限

| 标签 | 默认角色 | 能力 |
|------|----------|------|
| `pipeline_job:read` | developer / ops / admin | 列出与查看 Job 及运行历史 |
| `pipeline_job:write` | ops / admin | 创建 / 编辑 / 删除 Job（含 cron）|
| `pipeline_job:trigger` | developer / ops / admin | 运行 Job、取消、提交审批 |

`superadmin` 通过通配标签 `*` 自动具备所有能力。

### 限制

- **单实例 cron**：调度器在每个后端实例内存里独立运行；多实例部署需要外部协调（负载均衡 pinning 或 leader 选举），代码层暂未自带选主层。
- **不支持 DAG / 跨 Job 依赖**：每次运行自闭环；Job 内部要排序请用 `approval` step。
- **workspace 不自动 clone**：即使 `git_enabled=true` 也只是把凭证注入 env，clone 由 commands 自行执行 — 这样不需要 checkout 的 Job 不会被强加 clone 行为。
- **服务器时区固定**：cron 时区暂不可按 Job 配置。
- **仅 5 字段 cron**：无秒级支持。
