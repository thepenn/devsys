MODULES_DIR := modules
WEB_DIR := $(MODULES_DIR)/web

GOHOSTOS := $(shell go env GOHOSTOS)
VERSION := $(shell git describe --tags --always)

ifeq ($(GOHOSTOS), windows)
	CMD_WIRE_FILES := $(shell cd $(MODULES_DIR) && $(Git_Bash) -c "find . -name wire.go")
else
	CMD_WIRE_FILES := $(shell cd $(MODULES_DIR) && find . -name wire.go)
endif

fmt:
	@cd $(MODULES_DIR) && go fmt ./...

wire:
	cd $(MODULES_DIR) && wire gen $(CMD_WIRE_FILES)

.PHONY: server
server:
	@echo "Starting DevOps Server..."
	@if [ ! -f $(WEB_DIR)/dist/index.html ]; then \
		echo ""; \
		echo "  WARNING: $(WEB_DIR)/dist/index.html not found."; \
		echo "  The server will start but the SPA will return 404."; \
		echo "  Run 'make web' first to build the frontend bundle."; \
		echo ""; \
	fi
	@$(MAKE) wire
	cd $(MODULES_DIR) && go run cmd/*.go

.PHONY: run
run: server  ## 启动后端 (server 的别名, 兼容旧入口)

.PHONY: web-dev
web-dev:
	cd $(WEB_DIR) && npm run start

.PHONY: web
web:
	cd $(WEB_DIR) && npm run build

.PHONY: dev
dev:
	@echo "本地全栈开发请在两个终端分别执行:"
	@echo "  make run       # 启动后端 (8080)"
	@echo "  make web-dev   # 启动前端 dev server (3002)"

# -----------------------------------------------------------------------------
# Docker: 三阶段多段构建一个内嵌前端的静态二进制镜像 (详见根目录 Dockerfile).
# IMAGE_NAME / IMAGE_TAG 可被覆盖, 例如:
#   make docker-image IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/sixx/devsys IMAGE_TAG=v1.2.3
# -----------------------------------------------------------------------------
IMAGE_NAME ?= devsys
IMAGE_TAG  ?= $(VERSION)

.PHONY: docker-image
# 依赖 web + wire 是为了把 modules/web/dist 与 modules/cmd/wire/wire_gen.go 在
# 宿主机刷一遍再丢给 docker build, 镜像内部既不跑 webpack 也不跑 wire gen ——
# 两者都吃几 GB 内存, 在 buildx ARM64 / 普通 Colima VM 上必撞 OOM
# (ResourceExhausted: cannot allocate memory).
docker-image: web wire
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) -t $(IMAGE_NAME):latest .

.PHONY: docker-run
docker-run:
	docker run --rm -d \
		--name devsys \
		-p 8080:8080 \
		-v /var/run/docker.sock:/var/run/docker.sock \
		--env-file $(MODULES_DIR)/.env \
		$(IMAGE_NAME):latest
