# syntax=docker/dockerfile:1.7
#
# 两段构建: backend (go build) -> runtime (alpine + 静态二进制).
# 前端 dist 通过 Go //go:embed all:dist 嵌进二进制 (见 modules/web/web.go),
# 所以最终镜像里没有 node / npm, 只剩一个静态可执行 + 少量运行时依赖.
#
# 镜像内不跑 wire gen 也不跑 npm run build —— 两者都要 type-check 整个项目
# 依赖 / 持有完整 webpack 模块图, 在 Colima / QEMU 仿真等内存受限场景下都会
# 被 SIGKILL (ResourceExhausted: cannot allocate memory). 由 Makefile 的
# docker-image 入口在宿主机先跑 make web && make wire 把 modules/web/dist
# 与 modules/cmd/wire/wire_gen.go 刷新好, docker build 只负责把它们打包进镜像.
#
# 直接 docker build . (绕过 Makefile) 必须事先在宿主机跑过 make web && make wire,
# 否则 dist 缺失 SPA 资源会少, wire_gen.go 落后会导致 DI 不一致.

# ---- 1) Backend ----
FROM golang:1.24-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-trimpath
# 先拉依赖, 充分利用 layer cache.
COPY modules/go.mod modules/go.sum ./modules/
RUN cd modules && go mod download
COPY modules ./modules
RUN cd modules && go build -ldflags="-s -w" -o /out/devsys ./cmd

# ---- 2) Runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata git \
    && addgroup -S devsys \
    && adduser -S -G devsys -h /home/devsys -s /sbin/nologin devsys \
    && mkdir -p /workspace /home/devsys /var/lib/devsys-workspace \
    && chown -R devsys:devsys /workspace /home/devsys /var/lib/devsys-workspace
ENV TZ=Asia/Shanghai \
    LOG_PRETTY=false \
    SERVER_HOST=0.0.0.0:8080 \
    SERVER_ROOT_PATH=/api/v1 \
    HOME=/home/devsys \
    # 流水线 workspace root 在容器化部署时必须是 host 与容器同名的 bind volume,
    # 否则 step container 看到的 /workspace 跟 controller 写入的 fs 不在一处.
    # 部署侧需要: docker run -v devsys-workspace:/var/lib/devsys-workspace ...
    PIPELINE_WORKSPACE_ROOT=/var/lib/devsys-workspace
COPY --from=go-builder /out/devsys /usr/local/bin/devsys
WORKDIR /home/devsys
EXPOSE 8080
# 默认以 root 运行, 让流水线引擎能读写挂进来的 /var/run/docker.sock.
# 想改非 root: 加 --user devsys + --group-add <宿主 docker GID>.
ENTRYPOINT ["/usr/local/bin/devsys"]
