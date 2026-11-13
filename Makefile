# 自动加载 .env 文件中的环境变量（如果存在）
-include .env
export

# run & build --------------------------------
BIN_DIR := bin
APP_BINARY := $(BIN_DIR)/api

.PHONY: run build

## run: 本地启动 API 服务（自动加载 .env）
run:
	go run ./cmd/api

## build: 编译 API 二进制文件到 bin/api
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(APP_BINARY) ./cmd/api

# database migrate --------------------------------
MYSQL_HOST ?= 127.0.0.1
MYSQL_PORT ?= 13306
MYSQL_USER ?= root
MYSQL_PASSWORD ?= $(MYSQL_ROOT_PASSWORD)
MYSQL_DATABASE ?= seckill

# golang-migrate 可执行文件路径
MIGRATE := $(HOME)/.go/bin/migrate

# golang-migrate 专用的数据库连接 URL
MIGRATE_DB_URL := mysql://$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOST):$(MYSQL_PORT))/$(MYSQL_DATABASE)?multiStatements=true
MIGRATIONS_DIR := migrations

.PHONY: migrate-new migrate-up migrate-down migrate-version

## migrate-new: 创建新的迁移文件，用法: make migrate-new name=create_users
migrate-new:
	@if [ -z "$(name)" ]; then \
		echo "Error: 请指定迁移名称，例如: make migrate-new name=init_schema"; \
		exit 1; \
	fi
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

## migrate-up: 执行所有未应用的向上迁移
migrate-up:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATE_DB_URL)" up

## migrate-down: 回滚最后一次迁移（加 1 防止误操作一次性删光所有表）
migrate-down:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATE_DB_URL)" down 1

## migrate-version: 查看当前数据库迁移版本
migrate-version:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATE_DB_URL)" version

# docker compose --------------------------------
# docker compose 命令（指定 .env 文件和 docker-compose.yml 文件路径）
COMPOSE := docker compose --env-file .env -f deploy/docker-compose.yml

.PHONY: docker-up docker-down docker-down-v docker-ps docker-stop docker-start docker-logs

## docker-up: 启动所有依赖并等待健康检查通过
docker-up:
	$(COMPOSE) up -d --wait

## docker-down: 停止并移除容器（保留命名卷数据）
docker-down:
	$(COMPOSE) down

## docker-down-v: 停止并移除容器及数据卷（会清空数据库和 Redis 数据）
docker-down-v:
	$(COMPOSE) down -v

## docker-ps: 查看依赖服务状态
docker-ps:
	$(COMPOSE) ps

## docker-stop: 停止指定服务（不传 s 则停止全部），用法: make docker-stop s=redis
docker-stop:
	$(COMPOSE) stop $(s)

## docker-start: 启动指定服务（不传 s 则启动全部），用法: make docker-start s=redis
docker-start:
	$(COMPOSE) start $(s)

## docker-logs: 查看服务日志，用法: make docker-logs s=mysql
docker-logs:
	$(COMPOSE) logs -f $(s)

# test --------------------------------
# 测试数据库名称与迁移连接串
MYSQL_TEST_DATABASE ?= seckill_test
MIGRATE_TEST_DB_URL := mysql://$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOST):$(MYSQL_PORT))/$(MYSQL_TEST_DATABASE)?multiStatements=true

.PHONY: test test-integration test-db-reset mysql-cli

## test: 运行所有单元测试（开启竞态检测，不依赖 Docker）
test:
	go test -race -v ./...

## test-integration: 对 seckill_test 执行迁移并运行集成测试（需先 make docker-up）
test-integration:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATE_TEST_DB_URL)" up
	go test -race -v -tags=integration ./...

## test-db-reset: 彻底重置测试数据库（回滚所有表并重新迁移，清理脏数据）
test-db-reset:
	echo "y" | $(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATE_TEST_DB_URL)" down -all
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATE_TEST_DB_URL)" up

## mysql-cli: 进入 Docker 容器内的 MySQL 交互式终端
mysql-cli:
	$(COMPOSE) exec mysql mysql -u$(MYSQL_USER) -p$(MYSQL_PASSWORD) $(MYSQL_DATABASE)

# lint & format --------------------------------
GOLANGCI_LINT := $(HOME)/.go/bin/golangci-lint

.PHONY: lint fmt

## lint: 运行 golangci-lint 代码静态检查
lint:
	$(GOLANGCI_LINT) run ./...

## fmt: 使用 golangci-lint 自动格式化代码并整理 import 分组
fmt:
	$(GOLANGCI_LINT) fmt ./...
