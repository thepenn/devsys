# DevSys

DevSys 是一个自部署的 CI/CD 平台，后端由 Go 编写、前端基于 Vue 2 + Element UI。系统集成了多代码托管平台登录与仓库同步、流水线 YAML 配置、审批/定时触发、Docker 构建以及工作空间保理等功能，适合在私有环境里统一管理构建与发布流程。

## 功能亮点

- **多代码源接入**：支持 GitHub、GitLab、Gitea、Gitee，登录与仓库同步均由同一 Provider 控制，可按组织白名单同步。
- **流水线即代码**：仓库绑定 `.yaml` 配置描述步骤、镜像、命令、证书及变量，提供审批 Step、Dockerfile 回退文本、环境透传等能力。
- **多种触发方式**：手动触发、Webhook、多个 Cron 表达式同时调度，并发限制和序列化编号已处理。
- **审批与操作审计**：等待审批的 Step 在 UI 中展示同意/拒绝按钮（颜色区分），审批记录写入日志流和流程图。
- **构建产物治理**：可配置保留天数和最大记录数，清理任务会连带删除过期的工作空间目录。
- **可视化控制台**：内置 Dashboard、项目列表、构建详情（含日志、画布、变量展示、审批栏），支持浅色/深色日志背景。

## 目录结构概览

```
cmd/             // 程序入口与 wire 依赖注入
internal/        // 配置、日志、Server、工具等基础设施
model/           // GORM 实体定义（RepoPipelineConfig 等）
routers/         // HTTP API 与中间件
service/         // 业务服务（auth、pipeline、repo、migrate…）
web/             // Vue2 前端工程
```

## 环境要求

- Go **1.24+**
- Node.js **16+**（推荐 18 LTS，用于 `web/` 工程）
- MySQL **5.7+/8.0+**
- `wire` 代码生成器：`go install github.com/google/wire/cmd/wire@latest`
- （可选）`npm` 或 `pnpm`/`yarn` 按需

## 快速开始（前后端一体）

> 项目默认以前后端未分离的模式运行：`make web` 先打包前端，再执行 `make run` 即可由 Go 服务同时托管 API 与静态资源。

1. **准备配置**
   ```bash
   cp  .env.local  .env # 或直接编辑 .env
   ```
   根据下文的环境变量说明完成数据库、服务端口、鉴权 Provider 等设置。

2. **构建前端**
   ```bash
   make web
   # 等价于：cd web && npm install && npm run build:prod
   ```
   构建产物会输出到 `web/dist` 并由后端嵌入式托管。

3. **启动服务（API + 静态资源）**
   ```bash
   make run
   # 等价于：make wire && go run cmd/*.go
   ```
   默认 HTTP 监听地址由 `SERVER_HOST` 控制（`localhost:8080`），浏览器直接访问该地址即可看到完整系统。

4. **登录 & 同步仓库**
   - 浏览器访问 `http://localhost:8080/#/dashboard`。
   - 按提示完成 OAuth 登录（Provider 由 `SERVER_AUTH_PROVIDER` 决定）。
   - 触发“同步仓库”即可将白名单组织下的仓库导入系统。

## 环境变量速查

> 系统使用 [github.com/kelseyhightower/envconfig](internal/config/config.go) 读取环境变量，以下为常用配置。布尔值接受 `true/false/1/0`，多值列表使用 **逗号** 分隔。

### 核心配置

| 模块      | 变量                         | 说明/默认值 |
|-----------|------------------------------|-------------|
| 数据库    | `DATABASE_DRIVER`             | 默认为 `mysql` |
|           | `DATABASE_DATASOURCE`         | 例如 `user:pass@tcp(host:3306)/go-devops?charset=utf8mb4&parseTime=True&loc=Local` |
|           | `DATABASE_MAX_CONNECTIONS`    | 连接池大小，默认 `10` |
|           | `DATABASE_SHOW_SQL`          | 是否打印 SQL，默认 `false` |
| 日志      | `LOG_LEVEL` / `LOG_PRETTY`    | `debug/info/warn/error`，以及是否美化输出 |
| 服务      | `SERVER_HOST`                 | HTTP 监听地址，例如 `0.0.0.0:8080` |
|           | `SERVER_ROOT_PATH`           | API 前缀（默认 `/api/v1`，前端已使用该约定） |
| 流水线    | `PIPELINE_WORKER_COUNT`       | 并行 Worker 数（默认 `2`） |
|           | `PIPELINE_QUEUE_CAPACITY`     | 任务队列容量（默认 `128`） |
| 鉴权      | `SERVER_AUTH_PROVIDER`        | `github/gitlab/gitea/gitee`，默认 `gitlab` |
|           | `SERVER_AUTH_SESSION_SECRET`  | JWT/HMAC 密钥，未配置会随机生成（进程重启后失效） |
|           | `SERVER_AUTH_TOKEN_TTL`       | 登录令牌有效期（默认 `24h`） |
|           | `SERVER_AUTH_STATE_TTL`       | OAuth state 有效期（默认 `10m`） |

### Git Provider 配置

所有 Provider 共享规则：

- `SERVER_<PROVIDER>`：开启/关闭该 Provider（例如 `SERVER_GITHUB=true`）。
- OAuth `CLIENT/SECRET/REDIRECT` 必须与平台应用保持一致。
- `SERVER_<PROVIDER>_SCOPES` 用于自定义授权范围。
- `SERVER_<PROVIDER>_ORGS`（重点）：填写一个或多个组织/团队名称，系统只同步这些组织下用户有权限访问的仓库；留空则同步账号可见的全部仓库。多个值使用逗号分隔，如 `org-a,org-b`.
- 当 `SERVER_AUTH_PROVIDER` 设置为某 Provider 时，用户登录、仓库同步、权限判断都以该 Provider 为准。

#### GitHub

| 变量 | 说明 |
|------|------|
| `SERVER_GITHUB` | 设为 `true` 启用 GitHub 登录/同步。 |
| `SERVER_GITHUB_URL` / `SERVER_GITHUB_API_URL` | Web/API 基础地址，企业版可自定义，默认 `https://github.com` / `https://api.github.com`。 |
| `SERVER_GITHUB_CLIENT` / `SERVER_GITHUB_SECRET` / `SERVER_GITHUB_REDIRECT` | OAuth 应用信息，Redirect URL 需指向 `http(s)://<host>/api/v1/auth/gitlab/callback`。 |
| `SERVER_GITHUB_SCOPES` | 默认 `read:user repo read:org`，若使用组织过滤请确保包含 `read:org`。 |
| `SERVER_GITHUB_ORGS` | 逗号分隔的组织清单，留空表示同步所有可见仓库。 |
| `SERVER_GITHUB_INCLUDE_FORKS` | `true` 时保留 Fork 仓库，默认 `false`。 |
| `SERVER_GITHUB_SKIP_VERIFY` | 跳过 TLS 校验（仅限内网自签场景）。 |

#### GitLab

| 变量 | 说明 |
|------|------|
| `SERVER_GITLAB` | 启用 GitLab（默认开启）。 |
| `SERVER_GITLAB_URL` | GitLab 实例 URL（默认 `https://gitlab.com`）。 |
| `SERVER_GITLAB_CLIENT` / `SERVER_GITLAB_SECRET` / `SERVER_GITLAB_REDIRECT` | OAuth 应用配置。 |
| `SERVER_GITLAB_SCOPES` | 默认 `read_user api`。 |
| `SERVER_GITLAB_ORGS` | Project 所属命名空间（Group/Namespace）白名单。 |
| `SERVER_GITLAB_SKIP_VERIFY` | TLS 校验开关。 |

#### Gitea

| 变量 | 说明 |
|------|------|
| `SERVER_GITEA` | 启用 Gitea Provider。 |
| `SERVER_GITEA_URL` | Gitea 基础地址。 |
| `SERVER_GITEA_CLIENT` / `SERVER_GITEA_SECRET` / `SERVER_GITEA_REDIRECT` | OAuth 信息。 |
| `SERVER_GITEA_SCOPES` | 默认 `read:user user:email repo`。 |
| `SERVER_GITEA_ORGS` | 允许同步的组织列表，留空同步全部。 |
| `SERVER_GITEA_SKIP_VERIFY` | TLS 校验开关。 |

#### Gitee

| 变量 | 说明 |
|------|------|
| `SERVER_GITEE` | 启用 Gitee Provider。 |
| `SERVER_GITEE_URL` | 默认 `https://gitee.com`。 |
| `SERVER_GITEE_CLIENT` / `SERVER_GITEE_SECRET` / `SERVER_GITEE_REDIRECT` | OAuth 配置。 |
| `SERVER_GITEE_SCOPES` | 默认 `user_info projects`。 |
| `SERVER_GITEE_ORGS` | 组织白名单。 |
| `SERVER_GITEE_SKIP_VERIFY` | TLS 校验开关。 |

> **提示**：切换 Provider 后记得清理旧会话并重新登录，否则前端仍会沿用前一次的token。

## 流水线与仓库配置

仓库级设置存储在 `repo_pipeline_configs` 表，模型定义见 [model/repo_pipeline_config.go](model/repo_pipeline_config.go)。主要字段：

- `Content`：流水线 YAML 字符串。
- `Dockerfile`：当仓库根目录缺少 `Dockerfile` 时，构建器会使用此字段保存的模板。
- `CleanupEnabled`：是否自动清理历史记录。
- `RetentionDays` / `MaxRecords`：同时限制“保留天数”和“最大构建条数”。
- `DisallowParallel`：禁止同一仓库并发执行。
- `CronSchedules`：数组形式的 Cron 表达式，可同时配置多个调度（例如 `["*/2 * * * *", "0 3 * * *"]`）。

### 流水线 YAML 速览

```yaml
kind: pipeline
workspace: /tmp/devsys
steps:
  - name: default-env
    image: registry.cn-hangzhou.aliyuncs.com/sixx/busybox
    commands:
      - env | sort
  - name: git-clone
    image: registry.cn-hangzhou.aliyuncs.com/sixx/git
    certificate: [github-test-callback]
    commands:
      - git clone --verbose ${REPO_CLONE_URL_AUTH} .
    env:
      COMMIT_ID: $(git rev-parse HEAD)
  - name: docker-build-push
    image: registry.cn-hangzhou.aliyuncs.com/sixx/plugin-docker-buildx:latest
    certificate: [acr_aliyuncs]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    settings:
      repo: ${acr_aliyuncs.docker.repo}/sixx/devops
      tags: ${env.COMMIT_ID}
      dry_run: false
      password: ${acr_aliyuncs.docker.password}
      registry: ${acr_aliyuncs.docker.repo}
      username: ${acr_aliyuncs.docker.username}
      platforms: linux/amd64
      dockerfile: Dockerfile
    privileged: true
  - name: wait-for-approval
    image: alpine:latest
    settings:
      type: approval
      message: 请审批生产环境部署
      approvers: [kuzane]
      approval_timeout: 86400
      approval_strategy: any
  - name: default-env
    image: registry.cn-hangzhou.aliyuncs.com/sixx/busybox
    commands:
      - echo "deploy"
```

**注意点**

- `approval` Step 会暂停流水线，直到 UI 中的审批人点击“同意/拒绝”。
- 任何命令的日志在后端统一脱敏：包含 `password`/`token` 的变量值会被替换成 `***`.
- 如果仓库目录存在 `Dockerfile`，构建阶段会优先使用仓库文件；否则使用 `RepoPipelineConfig.Dockerfile` 中保存的模板。
- Cron 定时触发使用 `github.com/gdgvda/cron`，同一仓库多条 Cron 会各自入队，并在管线层序列化为唯一编号。
- 步骤语义兼容 Drone / Woodpecker 插件生态：在 `steps` 的 `image` + `settings` 中直接引用官方/社区插件（如 `plugins/docker`、`woodpeckerci/plugin-docker-buildx`）即可无缝使用已有 YAML 片段。


## 常见问题

- **无法同步仓库**：确认 `SERVER_<PROVIDER>_ORGS` 是否包含目标组织，GitHub 需勾选 `read:org`。若留空仍无仓库，检查访问 token 是否过期。
- **审批按钮不显示**：仅当当前登录用户在 Step `approvers` 列表中时才会渲染按钮；管理员可从构建详情的“更多操作”中取消流水线。
- **前端静态页仍可访问**：关闭后端仅影响 API，可在 Web 服务器上配置健康检查或反向代理错误页来阻止缓存页面。

如需更多细节，请浏览对应源文件（服务逻辑位于 [service](service/) 目录，API Router 位于 [routers](routers/)）。欢迎根据业务需求继续扩展。
