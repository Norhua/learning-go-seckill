# Go-Seckill 高并发秒杀系统

基于 Go 构建的高并发秒杀系统。

## 1. 技术选型

| 领域 | 选型 | 说明 |
| :--- | :--- | :--- |
| HTTP 框架 | Gin | |
| 数据库访问 | GORM (`gorm.io/gorm`) | |
| 数据库 | MySQL 8.0 | |
| 缓存 / 预占 | Redis 7 (`go-redis/v9`) | |
| 结构化日志 | 标准库 `log/slog` | |
| 配置管理 | `gopkg.in/yaml.v3` + 环境变量 | |
| 数据库迁移 | `golang-migrate` | |
| 测试与质量 | `testing` + `golangci-lint` | |

## 2. 目录结构

```text
implement/
├── cmd/
│   └── api/main.go              # API 进程入口：组装依赖、启动服务与优雅停机
├── configs/
│   └── config.yaml              # 默认非敏感配置
├── deploy/
│   ├── docker-compose.yml       # 本地开发依赖（MySQL 8.0 / Redis 7）
│   └── mysql/init.sql           # 容器首次初始化脚本（创建 seckill_test 测试库）
├── internal/
│   ├── config/                  # 配置加载、环境变量覆盖与合法性校验
│   ├── infra/                   # 基础设施客户端初始化（MySQL / Redis）
│   ├── logger/                  # slog 结构化日志封装与 Context 注入
│   └── server/                  # HTTP Server、路由、中间件、统一响应与健康检查
├── migrations/                  # golang-migrate 版本化 SQL 迁移文件
├── .env.example                 # 环境变量模板（含敏感配置）
├── .golangci.yml                # 静态代码检查配置
└── Makefile                     # 常用工程化命令封装
```

## 3. 环境依赖

- **Go**: 1.26+
- **Docker** & **Docker Compose**: v2+
- golang-migrate CLI
- **golangci-lint**: v2+

## 4. 快速开始

### 4.1 准备环境变量

- 复制 `.env.example` 为 `.env`，并根据实际情况修改敏感配置
- 安装 golang migrate
- 安装 golangci-lint
- 修改 Makefile 中 `MIGRATE_BIN` 与 `GOLANGCI_LINT_BIN` 的路径为实际安装路径
```bash
cp .env.example .env
nvim .env
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint
nvim Makefile
```

### 4.2 启动基础设施（MySQL & Redis）
```bash
make docker-up
```
> 该命令会自动等待容器 `healthcheck` 变为 `healthy`（默认映射端口：MySQL `13306`，Redis `16379`）。

### 4.3 执行数据库迁移
```bash
make migrate-up
```

### 4.4 启动 API 服务
```bash
make run
```

### 4.5 验证服务状态
另开一个终端执行：
```bash
# 存活检查（Liveness）
curl -i http://127.0.0.1:8080/healthz

# 就绪检查（Readiness，检查 MySQL 与 Redis 连通性）
curl -i http://127.0.0.1:8080/readyz
```

## 5. 常用 Make 命令

| 命令 | 说明 |
| :--- | :--- |
| `make run` | 本地启动 API 服务（自动加载 `.env`） |
| `make build` | 编译二进制文件到 `bin/api` |
| `make docker-up` | 启动 MySQL 和 Redis 容器并等待健康检查通过 |
| `make docker-down` | 停止并移除容器（保留命名卷数据） |
| `make docker-down-v` | 停止容器并**清空数据卷**（重置全新环境） |
| `make docker-ps` | 查看依赖容器运行与健康状态 |
| `make docker-stop s=redis` | 停止单个服务（用于故障演练） |
| `make docker-start s=redis` | 恢复启动单个服务 |
| `make migrate-new name=xxx` | 创建新的 `.up.sql` / `.down.sql` 迁移文件 |
| `make migrate-up` | 执行所有未应用的向上迁移 |
| `make migrate-down` | 回滚最后一次迁移（每次仅回滚 1 步） |
| `make migrate-version` | 查看当前数据库迁移版本 |
| `make test` | 运行所有单元测试（开启 `-race`，不依赖 Docker） |
| `make test-integration` | 对 `seckill_test` 执行迁移并运行集成测试 |
| `make lint` | 运行 `golangci-lint` 静态代码检查 |
| `make fmt` | 使用 `gofmt` / `goimports` 自动格式化代码 |
