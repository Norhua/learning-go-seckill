package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go-seckill/internal/config"
	"go-seckill/internal/infra/mysql"
	"go-seckill/internal/infra/redis"
	"go-seckill/internal/logger"
	"go-seckill/internal/server"
)

const (
	serviceName = "api"
	version     = "v0.1.0"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("application exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log, serviceName, version)
	slog.SetDefault(log)

	log.InfoContext(ctx, "starting server with configuration",
		slog.Any("server", cfg.Server),
		slog.Any("mysql", cfg.MySQL),
		slog.Any("redis", cfg.Redis),
		slog.Any("log", cfg.Log),
	)

	// 初始化 MySQL
	db, err := mysql.New(ctx, cfg.MySQL)
	if err != nil {
		return fmt.Errorf("init mysql: %w", err)
	}
	defer func() {
		if err := mysql.Close(db); err != nil {
			log.ErrorContext(ctx, "failed to close mysql", slog.Any("error", err))
		}
	}()
	log.InfoContext(ctx, "mysql connected successfully")

	// 初始化 Redis
	rdb, err := redis.New(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			log.ErrorContext(ctx, "failed to close redis", slog.Any("error", err))
		}
	}()
	log.InfoContext(ctx, "redis connected successfully")

	ser := server.New(cfg.Server, log)
	if err := ser.Start(); err != nil {
		return fmt.Errorf("start http server: %w", err)
	}

	return nil
}
