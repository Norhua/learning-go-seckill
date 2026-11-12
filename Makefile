# 自动加载 .env 文件中的环境变量（如果存在）
-include .env
export

MYSQL_HOST ?= 127.0.0.1
MYSQL_PORT ?= 13306
MYSQL_USER ?= root
MYSQL_PASSWORD ?= $(MYSQL_ROOT_PASSWORD)
MYSQL_DATABASE ?= seckill

# run: 本地启动 API 服务（自动加载 .env）
.PHONY: run

run:
	go run ./cmd/api

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
