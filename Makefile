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
	@$(MAKE) wire
	cd $(MODULES_DIR) && go run cmd/*.go

.PHONY: run
run:
	cd $(WEB_DIR) && npm run start

.PHONY: web
web:
	cd $(WEB_DIR) && npm run build

.PHONY: dev
dev: web run
