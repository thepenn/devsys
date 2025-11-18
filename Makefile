GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)

ifeq ($(GOHOSTOS), windows)
	CMD_WIRE_FILES=$(shell $(Git_Bash) -c "find . -name wire.go")
else
	CMD_WIRE_FILES=$(shell find . -name wire.go)
endif


fmt:
	@go fmt ./...

wire:
	wire gen $(CMD_WIRE_FILES)

.PHONY: server
server:
	@echo "Starting DevOps Server..."
	make wire && go run cmd/*.go

.PHONY: run
run:
	cd web && npm run start

.PHONY: web
web:
	cd web && npm run build

.PHONY: dev
dev: web run
