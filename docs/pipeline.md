# CI/CD Pipeline Module / CI/CD 流水线模块

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

### Overview

The pipeline module provides a complete CI/CD engine that executes YAML-defined pipelines inside Docker containers. It supports manual triggers, cron scheduling, human approval gates, and real-time log streaming.

**Source:** `modules/service/pipeline/`, `modules/routers/repo.go`

### Pipeline Sources

A pipeline run can be produced from three independent sources, all of which converge into the same `BuildAndEnqueueRun` execution path:

| Source | Owner | Trigger | YAML resolution |
|--------|-------|---------|-----------------|
| **Inline** | Repository | `POST /repos/{repo_id}/pipeline/run` | `RepoPipelineConfig.content` is parsed directly |
| **Template** | Repository | `POST /repos/{repo_id}/pipeline/run` (with `source=template`) | The published `PipelineTemplate` body is resolved with `${VAR}` substitution using `RepoPipelineConfig.template_variables` |
| **Job** | Standalone Job | `POST /pipeline-jobs/{id}/run` or cron | `PipelineJob.content` is rendered with merged variables (job defaults + opts overrides) |

See [Reusable Pipeline Templates](#reusable-pipeline-templates) and [docs/job.md](job.md) for details.

### Reusable Pipeline Templates

Templates let you define a single pipeline once and have many projects point at it. They have a draft / published two-state lifecycle:

- The **draft** is the in-flight YAML being edited.
- A **publish** action copies the draft into `published_content`. Referencing projects always consume the latest published content; until the first publish, projects that reference the template will fail to trigger with a clear error.

**Reference a template from a project:**

In the project pipeline configuration drawer, switch the source radio to "引用通用模板", select a published template and (optionally) provide variables that get substituted into `${VAR}` placeholders.

```
PUT /api/v1/repos/{repo_id}/pipeline/config
Content-Type: application/json

{
  "source": "template",
  "template_id": 12,
  "template_variables": {
    "IMAGE_TAG": "v1.4.2",
    "DEPLOY_ENV": "staging"
  }
}
```

Switch back to inline by sending `{"source":"inline","content":"<yaml>"}`. The previous template binding is cleared automatically; the inline `content` is preserved as a snapshot when switching from inline to template.

**Variable substitution** (template & job both use the same renderer):

| Syntax | Behaviour |
|--------|-----------|
| `${VAR}` | Replaced with `vars["VAR"]`; if missing, fall back to a Certificate by name (see below); finally empty string |
| `${VAR:-default}` | Replaced with `vars["VAR"]` (or Certificate fallback); `default` is used when both are missing/empty |

**Certificate fallback (default values from credentials store):**

When `${VAR}` is not provided in `template_variables` / `job.variables`, the renderer looks up a Certificate whose `name` exactly matches `VAR` and substitutes its primary value:

| Certificate type | Substituted value |
|------------------|--------------------|
| `git` | `password` (token) — handy for `git clone https://x:${gitlab-token}@…` |
| `docker` | `repo` (registry URL) — handy for `docker push ${aliyun_docker_registry}/myimage` |
| other | `""` (no substitution; treated as missing) |

**Resolution priority (highest to lowest):**

```
trigger options.variables  >  RepoPipelineConfig.template_variables / PipelineJob.variables  >  Certificate by name  >  ${VAR:-default}  >  ""
```

This means an explicit value at the project / Job level always wins. The Certificate fallback only kicks in when no one above provided a value, and `${VAR:-default}` is the final escape. Other fields of compound certificates (e.g. `docker` `username` / `password`) are not auto-injected via this path; if you need them as separate env vars in a step, declare them with the existing `certificates:` / `secrets:` step keys.

**Template management API** (mounted under `/api/v1/pipeline-templates`):

| Method | Path | Label |
|--------|------|-------|
| `GET` | `/pipeline-templates` (`?published=true` returns only publishable ones) | `pipeline_template:read` |
| `POST` | `/pipeline-templates` | `pipeline_template:write` |
| `GET` | `/pipeline-templates/{id}` | `pipeline_template:read` |
| `PUT` | `/pipeline-templates/{id}/draft` | `pipeline_template:write` |
| `POST` | `/pipeline-templates/{id}/publish` | `pipeline_template:write` |
| `POST` | `/pipeline-templates/{id}/render` (preview after substitution) | `pipeline_template:read` |
| `GET` | `/pipeline-templates/{id}/projects` (referencing repos) | `pipeline_template:read` |
| `DELETE` | `/pipeline-templates/{id}` (rejected if any project still references it) | `pipeline_template:write` |

**Trigger flow** when `source=template`:

```
trigger -> load RepoPipelineConfig -> source=template?
       yes -> templateSvc.Resolve(template_id, vars) -> spec.Parse -> BuildAndEnqueueRun
       no  -> spec.Parse(cfg.Content)               -> BuildAndEnqueueRun
```

If the template is unpublished or deleted, the trigger returns an error and writes a `PipelineError` row to surface the cause in the run history.

### kind: build (BuildKit, daemonless)

The built-in `kind: build` step replaces the typical `woodpeckerci/plugin-docker-buildx` flow with a daemonless [BuildKit](https://github.com/moby/buildkit) invocation. The engine launches `moby/buildkit` inside the step container, runs `buildctl-daemonless.sh build ...`, and pushes the resulting image — no `dockerd`, no `docker login`, no plugin-specific env wiring.

**Minimal example** — only the registry repository name is required; everything else (registry host, credentials, Dockerfile, tags, platform) has sensible defaults or comes from the bound docker certificate:

```yaml
steps:
  - name: build-and-push-image
    kind: build
    certificate: aliyun_docker_registry
    build:
      repo: sixx/devsys
```

That is enough to: pull `moby/buildkit:rootless`, generate `~/.docker/config.json` from the `aliyun_docker_registry` certificate (registry/username/password), `docker build` the workspace's `Dockerfile`, and push `<registry>/sixx/devsys:latest`.

**Full BuildSpec field reference:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `repo` | string | required | Project path under the registry (e.g. `sixx/devsys`) |
| `registry` | string | from cert (`docker.repo`) | Registry host (e.g. `registry.cn-hangzhou.aliyuncs.com`); leading `http(s)://` and trailing `/` auto-stripped |
| `username` | string | from cert | Override registry username |
| `password` | string | from cert | Override registry password |
| `dockerfile` | string | `Dockerfile` | Dockerfile path relative to `context` |
| `context` | string | `.` | Build context dir under workspace |
| `tags` | []string | `[latest]` | Tags to push; multiple values produce multiple `name=` outputs |
| `platforms` | []string | `[linux/amd64]` | One or more `os/arch` strings, comma-joined into `--opt platform=` |
| `push` | bool | `true` | Set `false` to build only (no push) |
| `build_args` | map[string]string | nil | Each entry becomes `--opt build-arg:KEY=VAL` (sorted by key) |
| `target` | string | nil | Multi-stage `--opt target=` |
| `no_cache` | bool | `false` | `--opt no-cache=true` |
| `buildkit_image` | string | `moby/buildkit:latest` | Override the BuildKit container image; if the value contains `rootless` (e.g. `moby/buildkit:rootless`) the engine switches to rootless mode (no `--privileged`, adds `seccomp=unconfined,apparmor=unconfined`) |
| `privileged` | bool | inferred from `buildkit_image` | Force `--privileged` even with a rootless image. Default behaviour: privileged for `:latest` style images, rootless for `*rootless*` images |

**Why the default is privileged + `:latest`:** matches the long-standing `woodpeckerci/plugin-docker-buildx` runtime profile, so it works on every Docker daemon (Docker Engine, Colima, Docker Desktop, etc.) out of the box. Rootless requires Linux + Docker Engine 20.10+ with kernel support for `systempaths=unconfined`; common macOS / older Linux setups outright reject that security-opt and the container never starts. Rootless mode is therefore an explicit opt-in via `buildkit_image: moby/buildkit:rootless`.

**Comparison with `woodpeckerci/plugin-docker-buildx`:**

| Aspect | `kind: build` | `plugin-docker-buildx` |
|--------|---------------|------------------------|
| Daemon | None — BuildKit ephemeral worker only | Spins up an inner `dockerd` |
| Privileged | Yes by default (rootless opt-in via image switch) | Required |
| Auth | `~/.docker/config.json` generated from cert | `PLUGIN_USERNAME / PASSWORD / REGISTRY` env |
| YAML overhead | A few fields | Three+ env wirings + dockerfile injection logic |
| Multi-arch | First-class via `platforms:` | Same, via plugin settings |
| Cache export | Roadmap | Plugin flags |

**Built-in step template:** First server start seeds a published `kind=step` template named `buildkit-image` whose body matches the minimal example above with `${IMAGE_REPO}` / `${DOCKER_CERT_NAME}` placeholders, so any project can `compose` it with three variables instead of writing the YAML from scratch.

**Frontend helper:** every YAML editor (project pipeline drawer, standalone job, template editor) has a *"插入 BuildKit 构建步骤"* button that opens a small form (cert dropdown, repo, tags, dockerfile, etc.) and appends a generated YAML snippet to the current content.

**Trigger flow:**

```
trigger -> spec.Parse -> StepKindBuild
       -> resolve registry/user/pass (build.* explicit, else first docker cert in step.secrets)
       -> write workspace/.devsys-buildkit/<step>/config.json (auths.<registry>.auth = base64(user:pass))
       -> dockerruntime.Run moby/buildkit:latest entrypoint=buildctl-daemonless.sh, Privileged=true
            args = build --frontend dockerfile.v0 --local context=... --output type=image,name=...,push=true
       -> defer: RemoveAll .devsys-buildkit dir
```

**Troubleshooting:**

- Registry / username / password missing: the engine fails the step before launching BuildKit with `kind=build 步骤 "build-and-push-image" 缺少 [registry password]: 请在 build.* 显式设置, 或在 step 上声明 certificate: <docker_cert>`. Add a `certificate:` line referencing a docker-type cert in 凭证管理.
- `cert.repo` accidentally pasted as `https://x.com/`: auto-trimmed at load time; `registry` and `auths.<registry>.auth` always use the bare host.
- Dockerfile not in workspace root: set `build.dockerfile: subdir/Dockerfile.prod`. The engine derives `--local dockerfile=` from the directory and `--opt filename=` from the basename.
- Want push disabled (CI smoke test): `build.push: false` — image is built but not uploaded.
- `Error response from daemon: invalid --security-opt: "systempaths=unconfined"` — your Docker daemon (typical on Colima / Docker Desktop / Docker < 20.10) rejects that option. The default `moby/buildkit:latest` + privileged path already avoids it; the error only appears when you explicitly opt into rootless via `buildkit_image: moby/buildkit:rootless`. Either upgrade to Linux + Docker Engine 20.10+ or stay on the default privileged image.
- Force privileged with a rootless image (rare): set `build.privileged: true` alongside `buildkit_image: moby/buildkit:rootless` — the engine adds `--privileged` and skips the rootless `SecurityOpt` set.
- `failed to read dockerfile: failed to create temp dir: stat /var/folders/.../T/: no such file or directory` (or any other ENOENT looking like a host path inside the container): the engine used to forward the controller process env (`TMPDIR`, `HOME`, `PATH`, `XPC_*`, `Apple_*`, ...) into every step container. macOS / launchd paths obviously do not exist inside Linux containers, so BuildKit's `os.MkdirTemp(os.TempDir(), ...)` crashes immediately. The runtime now strips host-only env keys via `containerSafeEnv` and force-sets `TMPDIR=/tmp` and `HOME=/tmp` for build steps. If you still see it, check whether `step.env:` has a hand-written `TMPDIR=/some/host/path` and replace it with a container-local path.
- `failed to read dockerfile: open Dockerfile: no such file or directory` with the BuildKit log showing `transferring dockerfile: 2B`: the host workspace directory is not in the Docker daemon's file-sharing list, so the bind mount silently lands on an empty directory inside the VM and the controller-written `Dockerfile` is invisible to the container. The default workspace root has been moved to `${HOME}/.devsys-workspace` (Colima / Docker Desktop / OrbStack all share `${HOME}` out of the box). If you overrode `PIPELINE_WORKSPACE_ROOT` to e.g. `/tmp/...` on Colima, switch back to the default or `colima start --mount /tmp:w`. See the "Workspace sharing across steps" section below.
- `fatal: detected dubious ownership in repository at '/workspace'`: git 2.35+ refuses to operate when the `.git` directory's UID differs from the running process UID (CVE-2022-24765). The host bind mount preserves the host user UID (e.g. `501` on macOS) while the step container typically runs as root (UID 0), so git always sees a mismatch. The engine now auto-injects `GIT_CONFIG_COUNT=1`, `GIT_CONFIG_KEY_0=safe.directory`, `GIT_CONFIG_VALUE_0=*` into every step container (only git reads them; other tools ignore). Users no longer need a manual `git config --global --add safe.directory /workspace` in their clone step. To opt out (e.g. you want strict ownership checking and provide your own config), set `GIT_CONFIG_COUNT: ""` (or your own larger config sequence) in the step's `env:`.

### Building devsys itself in a CI pipeline

When the devsys pipeline builds the devsys docker image, **clone + `kind: build` is not enough** — the repo only ships a `.gitkeep` placeholder under `modules/web/dist/` so the host-side `make docker-image` flow can pre-build the SPA via `make web`. In CI there is no Makefile to lean on, so the BuildKit step would `go build` against an empty `dist/` directory and produce an image whose `/` route returns 404. Add an explicit `web-build` step before `kind: build`:

```yaml
steps:
  - name: clone
    image: alpine/git:2.45.2
    commands:
      - git clone --depth 1 --branch main https://github.com/thepenn/devsys.git .
  - name: web-build
    image: node:20-alpine
    commands:
      - cd modules/web
      - npm ci --no-audit --no-fund
      - npm run build
  - name: build-and-push-image
    kind: build
    certificate: aliyun_docker_registry
    build:
      repo: sixx/devsys
      tags:
        - latest
        - build-${CI_PIPELINE_NUMBER}
```

Notes:
- `wire_gen.go` is committed in the repo, so CI does not need to regenerate it.
- The same engine improvements that make local `make docker-image` work — `containerSafeEnv` strips host TMPDIR, `gitSafeDirectoryEnv` injects safe.directory, workspace defaults to `${HOME}/.devsys-workspace` — apply automatically to step containers in this pipeline. No extra config required.
- `npm ci` requires `modules/web/package-lock.json` to be tracked. If you maintain a fork that drops the lockfile, switch to `npm install` (slower but works) or commit the lockfile.
- The webpack build inside `node:20-alpine` peaks at 2-3 GB RSS. If the controller VM is tight on RAM, prefer running the pipeline against a host with at least 4 GB free, or split webpack via `NODE_OPTIONS=--max-old-space-size=2048`.

### Workspace sharing across steps

Every step container in a single run bind-mounts the same host directory `<workspace_root>/<repo>/<pipeline_id>/` to `/workspace`. So:

- A `clone` step that runs `git clone … .` writes the source tree under that shared `/workspace`.
- The next step (BuildKit, plugin, or commands) sees the same files at `/workspace`.
- No per-step `volumes:` configuration is required for cross-step sharing — it is the default.

Whether the host directory is kept after the run is controlled by `RepoPipelineConfig.cleanup_enabled` (toggled in the project pipeline drawer / API). It defaults to **on**, so a post-run `ls <workspace_root>/<repo>/<pipeline_id>` will appear empty — that's the cleanup, not a missing checkout. Disable cleanup if you need to inspect the workspace after a failed run, or look at the live container while the run is in flight.

`<workspace_root>` defaults to `${HOME}/.devsys-workspace` on Linux/macOS controllers (e.g. `/Users/<you>/.devsys-workspace` on macOS) and `os.TempDir()/devsys-workspace` on Windows. The default was deliberately picked under `$HOME` because every popular macOS docker runtime auto-shares it, while neither `/var/folders/...` nor plain `/tmp` is shared out of the box on Colima:

| Runtime | Default shared paths | `${HOME}` covered |
|---------|----------------------|-------------------|
| Colima | `${HOME}`, `/tmp/colima` | yes |
| Docker Desktop | `/Users` (entire tree), `/Volumes`, `/private/tmp` | yes |
| OrbStack | `/Users` (entire tree) | yes |
| Linux native daemon | all host paths | yes |

Set the env var `PIPELINE_WORKSPACE_ROOT=/your/path` to override globally, or pass `payload.WorkspaceRoot` per trigger. The pipeline service prints a one-line `pipeline workspace root resolved workspace_root=...` at startup so the active path is visible immediately.

**Docker daemon file-sharing constraint:** when the Docker daemon runs in a VM (Colima, Docker Desktop, OrbStack, …) the host workspace path **must** be in the daemon's shared-paths list. Otherwise the controller writes to the host fs, but the bind mount target lands on a separate VM-side directory, so step containers see an empty `/workspace` while the host side has the controller-written files (and vice versa). Symptoms include BuildKit reporting `transferring dockerfile: 2B` then `open Dockerfile: no such file or directory` even though `ls <workspace>/Dockerfile` exists on the host. Customising the path requires a one-time configuration:

- **Docker Desktop (macOS / Windows):** Settings → Resources → File Sharing → add the path.
- **Colima:** restart with `colima start --mount /your/path:w`. Default already includes `${HOME}` so the new default needs no extra setup.
- **OrbStack:** any path under `/Users/<you>` is shared automatically; for others see OrbStack docs.
- **Linux:** all host paths are bindable.

**Containerised devsys deployments:** when devsys itself runs inside a docker container (see the project Dockerfile), the controller process and the host docker daemon see different filesystems. The image therefore sets `PIPELINE_WORKSPACE_ROOT=/var/lib/devsys-workspace` and the deployer **must** mount the same host path with the same in-container name, e.g. `docker run -v devsys-workspace:/var/lib/devsys-workspace ...`. Without that mount, controller writes land in the container's own fs and step containers (spawned via the host daemon) see an empty `/workspace`.

### Docker registry credentials

Container registry login (e.g. for `woodpeckerci/plugin-docker-buildx`) is a frequent source of `error authenticating: exit status 1`. The pipeline engine resolves Docker (and Git) credentials from the certificate store and exposes them to a step in three complementary ways. Pick whichever matches your style — the engine accepts all of them in the same step.

**Exposed fields per certificate type:**

| Type | Available fields | Notes |
|------|------------------|-------|
| `docker` | `username`, `password`, `repo`, `registry` (alias of `repo`) | `repo` is normalized at load time: leading `http(s)://` and trailing `/` are stripped so `docker login` doesn't choke on a pasted URL |
| `git` | `username`, `password`, `token` (alias of `password`) | `token` exists for Bearer-style flows |

A step opts into a certificate by either listing the certificate name in `secrets:` / `certificates:`, or — most commonly — using the singular `certificate:` shortcut:

```yaml
- name: build-and-push-image
  image: woodpeckerci/plugin-docker-buildx:2
  privileged: true
  certificate: aliyun_docker_registry
```

Once a step is bound to a certificate, the engine populates `<ALIAS>_USERNAME` / `<ALIAS>_PASSWORD` / `<ALIAS>_REPO` (sanitized to upper snake case) in the container env and registers `${alias.field}` substitution.

**Three ways to consume the credential:**

| Pattern | Recommended for | Example |
|---------|-----------------|---------|
| **A. Auto-injection** (lightest) | Most images that match `woodpeckerci/plugin-docker*`, `plugins/docker*`, or contain `docker-buildx` | Just declare `certificate: <name>` and skip `settings.username/password/registry` — the engine fills them from the bound docker credential automatically |
| **B. Dotted placeholder** (explicit) | When you need to mix multiple credentials, or non-docker plugins that read other env names | `${aliyun_docker_registry.docker.username}` (3-segment Drone style), `${aliyun_docker_registry.username}` (2-segment shorthand), or `${aliyun_docker_registry}` (alias-only → primary value, i.e. `repo`) |
| **C. Sanitized env name** | Drop-in compatibility with shell-style references | `${ALIYUN_DOCKER_REGISTRY_USERNAME}` — substituted from container env at runtime |

**Smart cert discovery:** if a step's `settings:` or `env:` already references `${some_name.field}` and `some_name` matches an existing certificate, it is auto-added to the step's `secrets`. You don't need to repeat `certificate: some_name` just for the placeholder to resolve.

**Complete working example** (project pipeline that builds & pushes to Aliyun ACR):

```yaml
steps:
  - name: build-and-push-image
    image: woodpeckerci/plugin-docker-buildx:2
    privileged: true
    certificate: aliyun_docker_registry
    settings:
      repo: ${aliyun_docker_registry.docker.repo}/sixx/devsys
      dockerfile: Dockerfile
      context: .
      platforms: linux/amd64
      tags: latest,build-${CI_PIPELINE_NUMBER}
```

With the `aliyun_docker_registry` docker certificate filled (`username`, `password`, `repo=registry.cn-hangzhou.aliyuncs.com`), the engine injects `PLUGIN_USERNAME / PLUGIN_PASSWORD / PLUGIN_REGISTRY` automatically; `repo` in `settings` is composed from the registry URL plus the project namespace.

**Common pitfalls:**

- The certificate `repo` field is normalized (`https://x.com/` → `x.com`) at load time, but for clarity you should still store the bare host.
- Docker-in-docker plugins require `privileged: true` on the step; otherwise the inner `dockerd` cannot start.
- Misspelling the certificate name no longer dies on a cryptic `error authenticating: exit status 1`. The engine pre-validates plugin env after substitution and fails the step with a message naming the unresolved placeholder, e.g. `步骤 "build-and-push-image" 的 plugin 设置存在未解析的占位符 [PLUGIN_USERNAME=${aliyun_docker_registr.docker.username}]; 请检查 step 是否声明了 certificate: <name>, 凭证名是否与凭证管理中一致, 类型是否匹配`.
- Multiple docker certificates on the same step: auto-injection picks the first one in alias-key order. Use the dotted form `${alias.docker.field}` to disambiguate.

### Standalone Pipeline Jobs

`PipelineJob` is a pipeline definition that lives outside any project repository. Useful for scheduled tasks, ops scripts, and shared utilities. Jobs share the same execution engine as repo pipelines but skip git clone unless explicitly enabled.

See **[docs/job.md](job.md)** for the complete reference (model, cron schedules, git options, API).

### Woodpecker syntax compatibility

The YAML grammar tracks [Woodpecker workflow syntax](https://woodpecker-ci.org/docs/usage/workflow-syntax) closely so existing `.woodpecker.yaml` files migrate with minimal change. Coverage matrix:

| Field | Parsed | Runtime enforce | Notes |
|-------|--------|-----------------|-------|
| `steps:` mapping | yes | yes | `steps: { name: { image: ..., commands: [...] } }` |
| `steps:` sequence | yes | yes | `steps: [{ name: ..., image: ... }]` |
| Step template top-level sequence (kind=step only) | yes | yes | `- name: ...` directly, no `steps:` wrapper |
| Step template top-level single mapping (kind=step) | yes | yes | A single step without sequence syntax |
| `image / commands / env / volumes / privileged / secrets / certificates` | yes | yes | Standard step fields |
| `settings` (incl. `type: approval`) | yes | yes | Approval step uses our 审批 flow |
| `when.branch` (string / list / `{include, exclude}`) | yes | yes | `*` glob via stdlib `path.Match` (no `**`) |
| `when.event` (string / list) | yes | yes | `push / pull_request / tag / cron / manual ...` |
| `when.status` (success / failure) | yes | partial | Only enforced when actual status is known |
| `when.ref` | yes | yes | Glob via `path.Match` |
| `when.repo` | yes | yes | Exact `owner/name` or `*` |
| `when` as list-of-mappings (OR-of-AND) | yes | yes | Any group matches => allowed |
| `when.cron / platform / instance` | yes | no | Parsed but ignored at runtime |
| `when.path` (incl. `include / exclude / ignore_message / on_empty`) | yes | no | Diff-based path filter not implemented |
| `when.matrix / evaluate` | yes | no | No matrix workflows; no expression engine |
| `step.pull / failure / entrypoint / directory / depends_on / detach / dns` | partial | mostly no | Parsed leniently; runtime ignores until further work |
| `services / clone / skip_clone / labels / global when / depends_on / runs_on / workspace` | partially parsed | no | Out of scope for this iteration |

#### Step template formats

For `kind=step` templates, three top-level shapes are accepted (all yield the same `[]StepSpec` after parsing):

```yaml
# 1. Standard with steps: wrapper
steps:
  clone:
    image: alpine/git:2.45.2
    commands:
      - git clone "${REPO_CLONE_URL_AUTH}" .
```

```yaml
# 2. Top-level sequence (Woodpecker-style)
- name: clone
  image: alpine/git:2.45.2
  commands:
    - |
      set -eu
      BRANCH="${CI_COMMIT_BRANCH:-${CI_DEFAULT_BRANCH:-main}}"
      git clone --depth 1 --branch "${BRANCH}" "${REPO_CLONE_URL_AUTH}" .
```

```yaml
# 3. Single step mapping (no name nesting)
name: clone
image: alpine/git:2.45.2
commands:
  - git clone "${REPO_CLONE_URL_AUTH}" .
```

For `kind=pipeline` (a complete pipeline template referenced by projects), only form 1 is valid because the full pipeline may also need `name:` / `workspace:` siblings of `steps:`.

#### `when` examples

Single mapping (all sub-conditions AND together):

```yaml
steps:
  - name: deploy
    image: alpine
    when:
      branch: main
      event: push
```

List of mappings (OR between entries, AND within each):

```yaml
steps:
  - name: prettier
    image: woodpeckerci/plugin-prettier
    when:
      - event: pull_request
        repo: owner/lib
      - event: push
        branch: main
```

Glob branch include / exclude:

```yaml
when:
  - branch:
      include: [main, release/*]
      exclude: [release/1.0.*]
```

### Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Manual Trigger│     │ Cron Schedule │     │   Webhook    │
│ (API call)   │     │ (background)  │     │  (future)    │
└──────┬───────┘     └──────┬───────┘     └──────┬───────┘
       │                    │                    │
       ▼                    ▼                    ▼
┌──────────────────────────────────────────────────────────┐
│                    Pipeline Queue                         │
│              (bounded, configurable capacity)             │
└──────────────────────────┬───────────────────────────────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
         ┌─────────┐ ┌─────────┐ ┌─────────┐
         │ Worker 1│ │ Worker 2│ │ Worker N│
         └────┬────┘ └────┬────┘ └────┬────┘
              │            │            │
              ▼            ▼            ▼
         ┌──────────────────────────────────┐
         │         Docker Runtime            │
         │   (container per step execution)  │
         └──────────────────────────────────┘
```

### Data Model

A pipeline execution follows this hierarchy:

- **Pipeline** — A single run instance, linked to a repository, branch, and commit
  - **Workflow** — A named stage within the pipeline (e.g., "build", "test", "deploy")
    - **Step** — An individual unit of work within a workflow
      - **Task** — Internal task graph with dependency tracking
      - **LogEntry** — Step-level log records (stdout, stderr, exit code, metadata, progress)

### Step Types

| Type | Description |
|------|-------------|
| `clone` | Clone the source repository |
| `commands` | Execute shell commands |
| `service` | Start a background service container |
| `plugin` | Run a plugin container |
| `cache` | Cache save/restore operations |
| `approval` | Human approval gate — pauses execution until approved/rejected |

### Pipeline Configuration

Each repository has a pipeline configuration stored as YAML in the database (`RepoPipelineConfig`).

**API Endpoints:**

- `GET /repos/{repo_id}/pipeline/config` — Retrieve current YAML configuration
- `PUT /repos/{repo_id}/pipeline/config` — Create or update YAML configuration

### Pipeline Settings

Per-repository settings that control execution behavior:

| Field | Type | Description |
|-------|------|-------------|
| `cleanup_enabled` | bool | Enable automatic cleanup of old pipeline records |
| `retention_days` | int | Number of days to retain pipeline records |
| `max_records` | int | Maximum number of pipeline records per repo (default: 10) |
| `dockerfile` | string | Custom Dockerfile for the build environment |
| `disallow_parallel` | bool | Prevent parallel pipeline execution |
| `cron_schedules` | []string | Cron expressions for scheduled runs |

### Triggering Pipelines

**Manual Trigger:**

```
POST /api/v1/repos/{repo_id}/pipeline/run
Content-Type: application/json

{
  "branch": "main",
  "commit": "",
  "variables": {
    "DEPLOY_ENV": "staging"
  }
}
```

**Cron Schedules:**

Configure cron expressions in pipeline settings. The pipeline service registers and manages cron jobs on startup.

### Approval Steps

Steps of type `approval` pause the pipeline until a designated user takes action.

**Approval configuration (within YAML):**
- `approvers` — List of usernames who can approve
- `min_approvals` — Minimum approvals required to proceed

**Approval actions:**
- `approve` — Approve the step
- `reject` — Reject the step (stops the pipeline)

**API:**

```
POST /api/v1/repos/{repo_id}/pipeline/runs/{pipeline_id}/steps/{step_id}/approval
Content-Type: application/json

{
  "action": "approve",
  "comment": "Looks good to me"
}
```

### Cancellation

Running pipelines can be cancelled:

```
POST /api/v1/repos/{repo_id}/pipeline/runs/{pipeline_id}/cancel?reason=manual
```

### Pipeline Status Values

| Status | Description |
|--------|-------------|
| `pending` | Queued, waiting for a worker |
| `running` | Currently executing |
| `success` | Completed successfully |
| `failure` | Completed with errors |
| `killed` | Cancelled by user |
| `blocked` | Waiting for approval |
| `skipped` | Skipped due to `when` conditions |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/repos/{repo_id}/pipeline/runs` | List pipeline runs for a repository |
| `GET` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}` | Get pipeline run detail (workflows, steps, logs) |
| `POST` | `/repos/{repo_id}/pipeline/run` | Trigger a manual pipeline run |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/cancel` | Cancel a running pipeline |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/steps/{step_id}/approval` | Submit approval decision |
| `GET` | `/repos/{repo_id}/pipeline/config` | Get pipeline YAML configuration |
| `PUT` | `/repos/{repo_id}/pipeline/config` | Update pipeline YAML configuration |
| `GET` | `/repos/{repo_id}/pipeline/settings` | Get pipeline settings |
| `PUT` | `/repos/{repo_id}/pipeline/settings` | Update pipeline settings |

---

<a id="中文"></a>

## 中文

### 概述

流水线模块提供完整的 CI/CD 引擎，在 Docker 容器中执行 YAML 定义的流水线。支持手动触发、定时调度、人工审批和实时日志流。

**源码：** `modules/service/pipeline/`、`modules/routers/repo.go`

### 流水线来源

一次 pipeline run 可以来自三种相互独立的来源，最终都汇入同一个 `BuildAndEnqueueRun` 执行链路：

| 来源 | Owner | 触发方式 | YAML 解析 |
|------|-------|----------|-----------|
| **Inline** | 项目 | `POST /repos/{repo_id}/pipeline/run` | 直接解析 `RepoPipelineConfig.content` |
| **Template** | 项目 | `POST /repos/{repo_id}/pipeline/run`（`source=template`）| 取已发布的 `PipelineTemplate` 内容 + 用 `RepoPipelineConfig.template_variables` 做 `${VAR}` 替换 |
| **Job** | 独立 Job | `POST /pipeline-jobs/{id}/run` 或 cron | 渲染 `PipelineJob.content`（job 默认变量 + 触发覆盖变量）|

详见下文「通用 Pipeline 模板」与 [docs/job.md](job.md)。

### 通用 Pipeline 模板

模板让你只定义一份流水线，让多个项目同时引用。模板有「草稿 / 已发布」两态：

- **草稿** 是正在编辑的 YAML。
- **发布** 操作把草稿拷贝到 `published_content`。引用方永远使用最新发布版；首次发布前，引用该模板的项目触发会失败并给出明确错误。

**项目侧引用模板：**

在「配置流水线」抽屉中把来源切到「引用通用模板」，选择已发布的模板，并按需填变量进行 `${VAR}` 替换。

```
PUT /api/v1/repos/{repo_id}/pipeline/config
Content-Type: application/json

{
  "source": "template",
  "template_id": 12,
  "template_variables": {
    "IMAGE_TAG": "v1.4.2",
    "DEPLOY_ENV": "staging"
  }
}
```

切回 inline 模式只需发 `{"source":"inline","content":"<yaml>"}`，旧的模板绑定自动清空；从 inline 切到 template 时会保留 `content` 作为快照（不丢失原 YAML），便于切回。

**变量替换语法**（模板与独立 Job 共用同一个渲染器）：

| 语法 | 行为 |
|------|------|
| `${VAR}` | 替换为 `vars["VAR"]`；缺失时按变量名 fallback 到凭证仓库（见下表）；都没命中则用空串 |
| `${VAR:-default}` | 替换为 `vars["VAR"]`（或凭证回填）；都缺失或为空时使用 `default` |

**凭证默认值回填：**

当 `${VAR}` 在 `template_variables` / `job.variables` 中没显式提供时，渲染器会去 Certificate 仓库（凭证管理）按 `name == VAR` 精确匹配，命中后按类型回填主值：

| 凭证类型 | 回填的字段 |
|----------|------------|
| `git` | `password`（token），用于 `git clone https://x:${gitlab-token}@…` |
| `docker` | `repo`（registry URL），用于 `docker push ${aliyun_docker_registry}/myimage` |
| 其它 | `""`（视为未命中） |

**优先级（从高到低）：**

```
trigger options.variables  >  RepoPipelineConfig.template_variables / PipelineJob.variables  >  凭证仓库按名匹配  >  ${VAR:-default}  >  ""
```

也就是说，项目 / Job 层显式给的值始终优先；凭证 fallback 仅在以上都未提供时启用，`${VAR:-default}` 是最后兜底。复合凭证（如 docker 的 `username` / `password`）不会通过此路径自动注入；想把这些字段当独立 env 用，需要在 step 里继续用 `certificates:` / `secrets:` 显式声明。

**模板管理 API**（挂在 `/api/v1/pipeline-templates`）：

| 方法 | 路径 | 标签 |
|------|------|------|
| `GET` | `/pipeline-templates`（`?published=true` 仅列已发布） | `pipeline_template:read` |
| `POST` | `/pipeline-templates` | `pipeline_template:write` |
| `GET` | `/pipeline-templates/{id}` | `pipeline_template:read` |
| `PUT` | `/pipeline-templates/{id}/draft` | `pipeline_template:write` |
| `POST` | `/pipeline-templates/{id}/publish` | `pipeline_template:write` |
| `POST` | `/pipeline-templates/{id}/render`（预览替换结果） | `pipeline_template:read` |
| `GET` | `/pipeline-templates/{id}/projects`（查看引用方） | `pipeline_template:read` |
| `DELETE` | `/pipeline-templates/{id}`（仍被引用时拒绝） | `pipeline_template:write` |

**`source=template` 时的触发流程：**

```
触发 -> 加载 RepoPipelineConfig -> source=template?
        yes -> templateSvc.Resolve(template_id, vars) -> spec.Parse -> BuildAndEnqueueRun
        no  -> spec.Parse(cfg.Content)               -> BuildAndEnqueueRun
```

模板未发布或已删除时，触发会失败并写入 `PipelineError` 行，让运行历史里能看到具体原因。

### kind: build (BuildKit, 无需 dockerd)

`kind: build` 是内置的镜像构建步骤，用 [BuildKit](https://github.com/moby/buildkit) 的 daemonless 模式取代常见的 `woodpeckerci/plugin-docker-buildx`。引擎在 step 容器里跑 `moby/buildkit` + `buildctl-daemonless.sh build ...` 直接推镜像 —— 不再起 `dockerd`、不再 `docker login`、也不需要 `PLUGIN_*` 三件套。

**最小写法** —— 只要 `build.repo` 必填，registry / 用户名 / 密码 / Dockerfile / tag / 平台都有合理默认或从 docker 凭证回填：

```yaml
steps:
  - name: build-and-push-image
    kind: build
    certificate: aliyun_docker_registry
    build:
      repo: sixx/devsys
```

跑起来等价于：拉 `moby/buildkit:rootless` → 用 `aliyun_docker_registry` 凭证生成 `~/.docker/config.json` → 构建 workspace 里的 `Dockerfile` → 推送 `<registry>/sixx/devsys:latest`。

**BuildSpec 字段表：**

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `repo` | string | 必填 | registry 下的项目路径，如 `sixx/devsys` |
| `registry` | string | 取自凭证 (`docker.repo`) | registry 主机；前导 `http(s)://` 与尾部 `/` 自动去掉 |
| `username` | string | 取自凭证 | 显式覆盖 |
| `password` | string | 取自凭证 | 显式覆盖 |
| `dockerfile` | string | `Dockerfile` | 相对 `context` 的 Dockerfile 路径 |
| `context` | string | `.` | 工作区下的构建上下文目录 |
| `tags` | []string | `[latest]` | 多个 tag 会变成多个 `name=` 输出 |
| `platforms` | []string | `[linux/amd64]` | 多平台构建，逗号拼成 `--opt platform=` |
| `push` | bool | `true` | 设为 `false` 仅构建不推送 |
| `build_args` | map[string]string | nil | 每项变成 `--opt build-arg:KEY=VAL`（按 key 排序） |
| `target` | string | nil | 多阶段 `--opt target=` |
| `no_cache` | bool | `false` | `--opt no-cache=true` |
| `buildkit_image` | string | `moby/buildkit:latest` | 覆盖 BuildKit 镜像；值含 `rootless`（如 `moby/buildkit:rootless`）时引擎切到 rootless 模式（不加 `--privileged`，加 `seccomp=unconfined,apparmor=unconfined`）|
| `privileged` | bool | 由 `buildkit_image` 推断 | 即便镜像是 rootless 也强制 `--privileged`。默认行为：`:latest` 类镜像走 privileged，含 `rootless` 的镜像走 rootless |

**为什么默认 privileged + `:latest`：** 跟旧的 `woodpeckerci/plugin-docker-buildx` 同构，所有 Docker daemon（Docker Engine / Colima / Docker Desktop / 旧 Docker）都开箱即用。Rootless 模式需要 Linux + Docker Engine 20.10+ 且内核支持 `systempaths=unconfined`，常见的 macOS / 旧 Linux 直接拒收 security-opt，容器根本起不来。所以 rootless 是显式 opt-in：`buildkit_image: moby/buildkit:rootless`。

**与 `woodpeckerci/plugin-docker-buildx` 对比：**

| 维度 | `kind: build` | `plugin-docker-buildx` |
|------|---------------|------------------------|
| 依赖 daemon | 无 —— BuildKit 临时 worker | 容器内现拉一个 `dockerd` |
| Privileged | 默认 privileged，rootless 显式 opt-in | 必须 |
| 鉴权 | 引擎按凭证生成 `~/.docker/config.json` | `PLUGIN_USERNAME / PASSWORD / REGISTRY` env |
| YAML 噪声 | 几个字段 | env 三件套 + dockerfile 注入逻辑 |
| 多架构 | `platforms:` 一行搞定 | 走 plugin settings |
| Cache 导出 | 后续规划 | 走 plugin flags |

**预制步骤模板：** 服务首次启动会幂等写入一条名为 `buildkit-image` 的 `kind=step` 已发布模板，内容就是上面最小示例 + `${IMAGE_REPO}` / `${DOCKER_CERT_NAME}` 占位符，项目在 compose 模式只需选模板 + 填三个变量。

**前端工具：** 项目流水线抽屉 / 独立 Job 编辑器 / 模板编辑器三处 YAML 编辑区都有「插入 BuildKit 构建步骤」按钮，打开小表单（凭证下拉、repo、tags、dockerfile 等）即可生成片段追加到当前 YAML。

**触发流程：**

```
trigger -> spec.Parse -> StepKindBuild
       -> resolve registry/user/pass (build.* 显式优先, 否则取 step.secrets 里首个 docker 凭证)
       -> 写 workspace/.devsys-buildkit/<step>/config.json (auths.<registry>.auth = base64(user:pass))
       -> dockerruntime.Run moby/buildkit:latest entrypoint=buildctl-daemonless.sh, Privileged=true
            args = build --frontend dockerfile.v0 --local context=... --output type=image,name=...,push=true
       -> defer: RemoveAll .devsys-buildkit
```

**故障排查：**

- registry / 用户名 / 密码缺失：引擎会在拉起 BuildKit 之前 fail-fast：`kind=build 步骤 "build-and-push-image" 缺少 [registry password]: 请在 build.* 显式设置, 或在 step 上声明 certificate: <docker_cert>`。补一行 `certificate:` 引用凭证管理里的 docker 类型凭证即可。
- 凭证 `repo` 字段误填 `https://x.com/`：加载时已自动 trim，`registry` 与 `auths.<registry>.auth` 都用裸主机名。
- Dockerfile 不在工作区根目录：写 `build.dockerfile: subdir/Dockerfile.prod`，引擎按目录拼 `--local dockerfile=`，按 basename 拼 `--opt filename=`。
- 想只构建不推送（CI 烟囱测试）：`build.push: false`。
- 报错 `Error response from daemon: invalid --security-opt: "systempaths=unconfined"` —— 你的 Docker daemon（常见于 Colima / Docker Desktop / Docker < 20.10）拒收该选项。默认 `moby/buildkit:latest` + privileged 路径已规避；只有显式切 `buildkit_image: moby/buildkit:rootless` 才会触发。要么升级到 Linux + Docker Engine 20.10+，要么就用默认的 privileged 镜像。
- rootless 镜像强制 privileged（罕见）：`build.privileged: true` 与 `buildkit_image: moby/buildkit:rootless` 同时声明，引擎会加 `--privileged` 并跳过 rootless 的 `SecurityOpt` 集合。
- `failed to read dockerfile: failed to create temp dir: stat /var/folders/.../T/: no such file or directory`（或其它看起来像宿主路径的 ENOENT）：旧版引擎把控制进程的 env (`TMPDIR`、`HOME`、`PATH`、`XPC_*`、`Apple_*` 等) 全量灌进了步骤容器。macOS / launchd 的路径在 Linux 容器里当然不存在，BuildKit `os.MkdirTemp(os.TempDir(), ...)` 直接挂。引擎现在会用 `containerSafeEnv` 剥掉这些宿主专属 key，并对 build 步显式设 `TMPDIR=/tmp`、`HOME=/tmp`。如果仍然报，检查 `step.env:` 里是否手抖写了 `TMPDIR=/some/host/path`，改成容器内合法路径即可。
- `failed to read dockerfile: open Dockerfile: no such file or directory`，BuildKit 日志显示 `transferring dockerfile: 2B`：宿主机 workspace 路径不在 Docker daemon 的 file-sharing 列表里，bind mount 在 VM 内静默落到一个空目录，控制进程写入的 `Dockerfile`（包括 `RepoPipelineConfig.Dockerfile` 兜底注入）容器是看不到的。默认 workspace root 已改成 `${HOME}/.devsys-workspace`（Colima / Docker Desktop / OrbStack 默认都共享 `${HOME}`，开箱即用）。如果你显式 `PIPELINE_WORKSPACE_ROOT=/tmp/...` 覆盖了，在 Colima 上 `/tmp` 默认不共享会复现该问题，要么改回默认，要么 `colima start --mount /tmp:w`。详见下面的「Workspace 共享语义」一节。
- `fatal: detected dubious ownership in repository at '/workspace'`：git 2.35+ 的 CVE-2022-24765 加固，`.git` 目录的 owner UID 跟当前进程 UID 不一致就拒绝操作。host bind mount 透传宿主用户 UID（macOS 一般是 `501`），step container 通常以 root（UID 0）跑，必然不匹配。引擎现在会自动给所有 step container 注入 `GIT_CONFIG_COUNT=1`、`GIT_CONFIG_KEY_0=safe.directory`、`GIT_CONFIG_VALUE_0=*` 三条 env（只有 git 会读，其它工具忽略），用户不用再在 clone step 手动 `git config --global --add safe.directory /workspace`。想关掉（例如生产追求严格 ownership 校验、自己注入更完整的 config 序列）：在 step 的 `env:` 里把 `GIT_CONFIG_COUNT` 设成空串或更大数字即可。

### 在 CI pipeline 里构建 devsys 镜像

devsys 自己的 pipeline 用 `kind: build` 构建 devsys 镜像时，**只 clone + build 不够** —— 仓库里 `modules/web/dist/` 只有占位 `.gitkeep`，宿主机 `make docker-image` 流程靠 `make web` 在本地预构建 SPA。CI 没有 Makefile 兜底，BuildKit 步骤 `go build` 时 dist 目录是空的，构建出来的镜像访问 `/` 会 404。在 `kind: build` 之前显式加一个 `web-build` step：

```yaml
steps:
  - name: clone
    image: alpine/git:2.45.2
    commands:
      - git clone --depth 1 --branch main https://github.com/thepenn/devsys.git .
  - name: web-build
    image: node:20-alpine
    commands:
      - cd modules/web
      - npm ci --no-audit --no-fund
      - npm run build
  - name: build-and-push-image
    kind: build
    certificate: aliyun_docker_registry
    build:
      repo: sixx/devsys
      tags:
        - latest
        - build-${CI_PIPELINE_NUMBER}
```

注意点：
- `wire_gen.go` 已经 commit 到仓库，CI 端不需要再生成。
- 跟本地 `make docker-image` 一样的引擎改进（`containerSafeEnv` 剥宿主 TMPDIR、`gitSafeDirectoryEnv` 注入 safe.directory、workspace 默认 `${HOME}/.devsys-workspace`）在这条 pipeline 的 step 容器里也自动生效，不用额外配置。
- `npm ci` 需要 `modules/web/package-lock.json` 在 git 里。如果 fork 把 lockfile 删了，要么改成 `npm install`（慢一点但能跑），要么把 lockfile 提交回去。
- `node:20-alpine` 里跑 webpack 峰值 RSS 2-3 GB。控制器 VM 内存紧张时建议跑 pipeline 的宿主有至少 4 GB 空闲，或在 step `env:` 里加 `NODE_OPTIONS=--max-old-space-size=2048` 限上限。

### Workspace 共享语义

同一次 run 里所有 step 容器都把宿主机上的 `<workspace_root>/<repo>/<pipeline_id>/` 挂到容器内 `/workspace`，所以：

- `clone` step 在 `/workspace` 里跑 `git clone … .`，源码就落在这个共享目录里。
- 后续 step（BuildKit、plugin 或 commands）读到的就是同一份 `/workspace`。
- 跨 step 共享是默认行为，**不需要再额外写 `volumes:`**。

run 结束后这份目录是否保留，由项目流水线的「清理 workspace」开关 (`RepoPipelineConfig.cleanup_enabled`) 决定：默认开，run 结束你 `ls <workspace_root>/<repo>/<pipeline_id>` 会发现是空的 —— 不是 clone 没下载，是被清理了。想保留下来排查就关掉这个开关，或者趁 run 还在跑时挂进容器看。

`<workspace_root>` 在 Linux/macOS 控制器上默认是 `${HOME}/.devsys-workspace`（macOS 上即 `/Users/<you>/.devsys-workspace`），Windows 走 `os.TempDir()/devsys-workspace`。这个默认是按"所有主流 macOS docker runtime 默认都共享 `${HOME}`"挑的：

| Runtime | 默认共享路径 | 是否含 `${HOME}` |
|---------|--------------|-----------------|
| Colima | `${HOME}`、`/tmp/colima` | 是 |
| Docker Desktop | `/Users` 整树、`/Volumes`、`/private/tmp` | 是 |
| OrbStack | `/Users` 整树 | 是 |
| Linux 原生 | 任意 host 路径 | 是 |

要全局覆盖：设环境变量 `PIPELINE_WORKSPACE_ROOT=/your/path`；要按触发覆盖：在 trigger payload 里传 `workspace_root`。pipeline 服务启动时会打印一行 `pipeline workspace root resolved workspace_root=...`，让你立刻看到当前生效的路径。

**Docker daemon file-sharing 约束**：当 Docker daemon 跑在 VM 里（Colima / Docker Desktop / OrbStack 等），宿主机 workspace 路径**必须**在 daemon 的共享目录列表里。否则 controller 写到 host fs，bind mount 在 VM 里另落一份，step container 看的是 VM 那份（空目录），host 这边 `ls` 能看到 controller 写的文件，两边对不上。典型现象：BuildKit 报 `transferring dockerfile: 2B` 然后 `open Dockerfile: no such file or directory`，但 host 上 `ls <workspace>/Dockerfile` 明明存在。自定义路径需要一次性配置：

- **Docker Desktop（macOS / Windows）**：Settings → Resources → File Sharing 里加进去。
- **Colima**：`colima start --mount /your/path:w` 重启。默认已包含 `${HOME}`，所以新默认无需额外配置。
- **OrbStack**：`/Users/<you>` 下任意路径自动共享，其它路径见 OrbStack 文档。
- **Linux**：所有宿主路径都能直接挂。

**容器化部署 devsys**：当 devsys 自己也跑在容器里（见项目 Dockerfile），controller 进程和 host docker daemon 看的是不同 fs。镜像因此把 `PIPELINE_WORKSPACE_ROOT` 显式置成 `/var/lib/devsys-workspace`，部署时**必须**用同名 bind 挂同一个 host 路径进容器，例如 `docker run -v devsys-workspace:/var/lib/devsys-workspace ...`。漏挂的话 controller 写在容器自己的 fs 里，host daemon 拉起的 step container 看到的 `/workspace` 是空的。

### Docker 镜像构建凭证

容器仓库登录（典型如 `woodpeckerci/plugin-docker-buildx`）是 `error authenticating: exit status 1` 的高发地。流水线引擎会从凭证管理里把 Docker / Git 凭证解析出来，并以三种互补方式暴露给 step；同一 step 里三种写法可以混用，引擎都能识别。

**各类凭证可用字段：**

| 类型 | 可用字段 | 说明 |
|------|----------|------|
| `docker` | `username`、`password`、`repo`、`registry`（=repo） | 加载时会自动 trim 掉 `repo` 前面的 `http(s)://` 与尾部 `/`，避免 `docker login` 因为粘贴整段 URL 而失败 |
| `git` | `username`、`password`、`token`（=password） | 提供 `token` 别名便于 Bearer 流程 |

step 通过以下任一方式绑定凭证：在 `secrets:` / `certificates:` 列出凭证名，或者直接用更短的 `certificate:` 写法：

```yaml
- name: build-and-push-image
  image: woodpeckerci/plugin-docker-buildx:2
  privileged: true
  certificate: aliyun_docker_registry
```

绑定之后，引擎会向容器 env 写入 `<ALIAS>_USERNAME` / `<ALIAS>_PASSWORD` / `<ALIAS>_REPO`（统一规范化为大写下划线），并允许 `${alias.field}` 形式的占位符在运行时被替换。

**三种推荐写法对比：**

| 写法 | 推荐场景 | 示例 |
|------|----------|------|
| **A. 自动注入**（最简） | 镜像匹配 `woodpeckerci/plugin-docker*`、`plugins/docker*` 或包含 `docker-buildx` | 只声明 `certificate: <name>`，无需写 `settings.username/password/registry`，引擎会按已绑定的 docker 凭证自动填好 |
| **B. 点路径占位符**（显式） | 同 step 想混合多个凭证；或非 docker 插件需要其它环境变量名 | `${aliyun_docker_registry.docker.username}`（Drone 三段式）、`${aliyun_docker_registry.username}`（两段简写）、`${aliyun_docker_registry}`（仅 alias，返回主字段即 `repo`） |
| **C. 规范化 env 名** | 与 shell 风格保持一致 | `${ALIYUN_DOCKER_REGISTRY_USERNAME}` —— 运行时从容器 env 中替换 |

**自动凭证发现：** 如果 step 的 `settings:` / `env:` 里已经写了 `${some_name.field}` 而 `some_name` 又恰好是已存在的凭证名，引擎会自动把它补到该 step 的 secrets 上，无需再额外写一行 `certificate: some_name`。

**完整可跑示例**（项目流水线，构建并推送到阿里云 ACR）：

```yaml
steps:
  - name: build-and-push-image
    image: woodpeckerci/plugin-docker-buildx:2
    privileged: true
    certificate: aliyun_docker_registry
    settings:
      repo: ${aliyun_docker_registry.docker.repo}/sixx/devsys
      dockerfile: Dockerfile
      context: .
      platforms: linux/amd64
      tags: latest,build-${CI_PIPELINE_NUMBER}
```

`aliyun_docker_registry` 这条 docker 凭证（`username`、`password`、`repo=registry.cn-hangzhou.aliyuncs.com`）一旦绑定，引擎就会把 `PLUGIN_USERNAME / PLUGIN_PASSWORD / PLUGIN_REGISTRY` 自动写到容器；`settings.repo` 用 registry URL 拼上项目命名空间即可。

**常见踩坑：**

- 凭证 `repo` 字段在加载时已经做了规范化（`https://x.com/` → `x.com`），但建议在凭证管理里就只填裸主机名，避免视觉混乱。
- DinD 类插件必须给 step 加上 `privileged: true`，否则容器内的 `dockerd` 起不来。
- 凭证名拼错不再卡在抓不着原因的 `error authenticating: exit status 1`。引擎会在替换完 plugin env 后做一次预校验，发现仍有 `${...}` 字面量就直接 fail-fast，错误消息直接点出残留的占位符，例如：`步骤 "build-and-push-image" 的 plugin 设置存在未解析的占位符 [PLUGIN_USERNAME=${aliyun_docker_registr.docker.username}]; 请检查 step 是否声明了 certificate: <name>, 凭证名是否与凭证管理中一致, 类型是否匹配`。
- 同一 step 上挂了多个 docker 凭证：自动注入按 alias 字典序选第一个；要明确指定就用点路径 `${alias.docker.field}`。

### 独立 Pipeline Job

`PipelineJob` 是不依赖任何项目仓库的 pipeline 定义，适用于定时任务、运维脚本、共享工具。Job 和项目流水线共用同一个执行引擎，区别在于：默认不做 git clone（除非显式启用 git）。

完整说明请见 **[docs/job.md](job.md)**（模型、cron 调度、git 选项、API）。

### Woodpecker 语法兼容性

YAML 语法对齐 [Woodpecker workflow syntax](https://woodpecker-ci.org/docs/usage/workflow-syntax)，便于把已有 `.woodpecker.yaml` 直接搬过来。覆盖矩阵：

| 字段 | 解析 | Runtime 强制 | 备注 |
|------|------|--------------|------|
| `steps:` mapping | 是 | 是 | `steps: { name: { image: ..., commands: [...] } }` |
| `steps:` 序列 | 是 | 是 | `steps: [{ name: ..., image: ... }]` |
| 步骤模板顶层序列（仅 kind=step） | 是 | 是 | 直接 `- name: ...`，不必套 `steps:` |
| 步骤模板顶层单 mapping（仅 kind=step） | 是 | 是 | 一个步骤的简写 |
| `image / commands / env / volumes / privileged / secrets / certificates` | 是 | 是 | 标准 step 字段 |
| `settings`（含 `type: approval`） | 是 | 是 | 走我们的审批流程 |
| `when.branch`（字符串 / 列表 / `{include, exclude}`） | 是 | 是 | `*` glob，基于 stdlib `path.Match`（不支持 `**`） |
| `when.event`（字符串 / 列表） | 是 | 是 | `push / pull_request / tag / cron / manual ...` |
| `when.status`（success / failure） | 是 | 部分 | 仅在已知状态时强制 |
| `when.ref` | 是 | 是 | `path.Match` glob |
| `when.repo` | 是 | 是 | 精确 `owner/name` 或 `*` |
| `when` 多组（list-of-mappings, OR-of-AND） | 是 | 是 | 任一 group 命中即通过 |
| `when.cron / platform / instance` | 是 | 否 | 解析通过，触发时忽略 |
| `when.path`（含 `include / exclude / ignore_message / on_empty`） | 是 | 否 | 暂不实现 file diff 过滤 |
| `when.matrix / evaluate` | 是 | 否 | 不支持 matrix；未引入表达式引擎 |
| `step.pull / failure / entrypoint / directory / depends_on / detach / dns` | 部分 | 否（多数） | 当前仅宽松解析 |
| `services / clone / skip_clone / labels / 全局 when / depends_on / runs_on / workspace` | 部分 | 否 | 本期不在范围 |

#### 步骤模板的三种写法

`kind=step` 模板接受三种顶层形态，解析后都得到等价的 `[]StepSpec`：

```yaml
# 1. 标准 steps: 包裹
steps:
  clone:
    image: alpine/git:2.45.2
    commands:
      - git clone "${REPO_CLONE_URL_AUTH}" .
```

```yaml
# 2. 顶层序列（Woodpecker 写法）
- name: clone
  image: alpine/git:2.45.2
  commands:
    - |
      set -eu
      BRANCH="${CI_COMMIT_BRANCH:-${CI_DEFAULT_BRANCH:-main}}"
      git clone --depth 1 --branch "${BRANCH}" "${REPO_CLONE_URL_AUTH}" .
```

```yaml
# 3. 单步骤 mapping
name: clone
image: alpine/git:2.45.2
commands:
  - git clone "${REPO_CLONE_URL_AUTH}" .
```

`kind=pipeline`（项目整体引用的完整模板）只接受第 1 种，因为可能要在 `steps:` 同级写 `name:` / `workspace:`。

#### `when` 示例

单 mapping（所有子条件 AND）：

```yaml
steps:
  - name: deploy
    image: alpine
    when:
      branch: main
      event: push
```

mapping 列表（条目之间 OR，条目内部 AND）：

```yaml
steps:
  - name: prettier
    image: woodpeckerci/plugin-prettier
    when:
      - event: pull_request
        repo: owner/lib
      - event: push
        branch: main
```

分支 glob include / exclude：

```yaml
when:
  - branch:
      include: [main, release/*]
      exclude: [release/1.0.*]
```

### 架构

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ 手动触发      │     │ 定时调度      │     │ Webhook      │
│ (API 调用)   │     │ (后台运行)    │     │ (未来支持)    │
└──────┬───────┘     └──────┬───────┘     └──────┬───────┘
       │                    │                    │
       ▼                    ▼                    ▼
┌──────────────────────────────────────────────────────────┐
│                      流水线队列                           │
│               (有界，可配置容量)                           │
└──────────────────────────┬───────────────────────────────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
         ┌─────────┐ ┌─────────┐ ┌─────────┐
         │ Worker 1│ │ Worker 2│ │ Worker N│
         └────┬────┘ └────┬────┘ └────┬────┘
              │            │            │
              ▼            ▼            ▼
         ┌──────────────────────────────────┐
         │         Docker 运行时             │
         │     (每步骤一个容器执行)            │
         └──────────────────────────────────┘
```

### 数据模型

流水线执行遵循以下层级结构：

- **Pipeline** — 单次运行实例，关联仓库、分支和提交
  - **Workflow** — 流水线中的命名阶段（如 "build"、"test"、"deploy"）
    - **Step** — 工作流中的单个执行单元
      - **Task** — 内部任务图，包含依赖追踪
      - **LogEntry** — 步骤级日志记录（stdout、stderr、退出码、元数据、进度）

### 步骤类型

| 类型 | 说明 |
|------|------|
| `clone` | 克隆源代码仓库 |
| `commands` | 执行 Shell 命令 |
| `service` | 启动后台服务容器 |
| `plugin` | 运行插件容器 |
| `cache` | 缓存保存/恢复操作 |
| `approval` | 人工审批 — 暂停执行直到审批通过/拒绝 |

### 流水线配置

每个仓库的流水线配置以 YAML 格式存储在数据库中（`RepoPipelineConfig`）。

**API 端点：**

- `GET /repos/{repo_id}/pipeline/config` — 获取当前 YAML 配置
- `PUT /repos/{repo_id}/pipeline/config` — 创建或更新 YAML 配置

### 流水线设置

每个仓库的设置，控制执行行为：

| 字段 | 类型 | 说明 |
|------|------|------|
| `cleanup_enabled` | bool | 启用自动清理旧流水线记录 |
| `retention_days` | int | 流水线记录保留天数 |
| `max_records` | int | 每仓库最大流水线记录数（默认 10） |
| `dockerfile` | string | 自定义构建环境 Dockerfile |
| `disallow_parallel` | bool | 禁止并行执行流水线 |
| `cron_schedules` | []string | 定时执行的 Cron 表达式 |

### 触发流水线

**手动触发：**

```
POST /api/v1/repos/{repo_id}/pipeline/run
Content-Type: application/json

{
  "branch": "main",
  "commit": "",
  "variables": {
    "DEPLOY_ENV": "staging"
  }
}
```

**定时调度：**

在流水线设置中配置 Cron 表达式，流水线服务在启动时注册和管理定时任务。

### 审批步骤

`approval` 类型的步骤会暂停流水线，直到指定用户执行操作。

**审批配置（YAML 中）：**
- `approvers` — 可审批的用户名列表
- `min_approvals` — 通过所需的最小审批数

**审批操作：**
- `approve` — 批准该步骤
- `reject` — 拒绝该步骤（停止流水线）

**API：**

```
POST /api/v1/repos/{repo_id}/pipeline/runs/{pipeline_id}/steps/{step_id}/approval
Content-Type: application/json

{
  "action": "approve",
  "comment": "没问题，可以继续"
}
```

### 取消流水线

可以取消正在运行的流水线：

```
POST /api/v1/repos/{repo_id}/pipeline/runs/{pipeline_id}/cancel?reason=manual
```

### 流水线状态值

| 状态 | 说明 |
|------|------|
| `pending` | 已入队，等待 Worker |
| `running` | 正在执行 |
| `success` | 执行成功 |
| `failure` | 执行失败 |
| `killed` | 被用户取消 |
| `blocked` | 等待审批 |
| `skipped` | 因 `when` 条件跳过 |

### API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/repos/{repo_id}/pipeline/runs` | 列出仓库的流水线运行记录 |
| `GET` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}` | 获取流水线运行详情（工作流、步骤、日志） |
| `POST` | `/repos/{repo_id}/pipeline/run` | 手动触发流水线 |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/cancel` | 取消运行中的流水线 |
| `POST` | `/repos/{repo_id}/pipeline/runs/{pipeline_id}/steps/{step_id}/approval` | 提交审批决定 |
| `GET` | `/repos/{repo_id}/pipeline/config` | 获取流水线 YAML 配置 |
| `PUT` | `/repos/{repo_id}/pipeline/config` | 更新流水线 YAML 配置 |
| `GET` | `/repos/{repo_id}/pipeline/settings` | 获取流水线设置 |
| `PUT` | `/repos/{repo_id}/pipeline/settings` | 更新流水线设置 |
