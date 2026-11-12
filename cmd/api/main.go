package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
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

	// 初始化 MySQL (最先注册 defer 保证最后关闭)
	db, err := mysql.New(ctx, cfg.MySQL)
	if err != nil {
		return fmt.Errorf("init mysql: %w", err)
	}
	defer func() {
		if err := mysql.Close(db); err != nil {
			log.ErrorContext(ctx, "failed to close mysql", slog.Any("error", err))
		} else {
			log.Info("mysql connection closed")
		}
	}()
	log.InfoContext(ctx, "mysql connected successfully")

	// 初始化 Redis (第二个注册 defer 保证倒数第二关闭)
	rdb, err := redis.New(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			log.ErrorContext(ctx, "failed to close redis", slog.Any("error", err))
		} else {
			log.Info("redis connection closed")
		}
	}()
	log.InfoContext(ctx, "redis connected successfully")

	ser := server.New(cfg.Server, log)
	ser.RegisterChecker("mysql", func(ctx context.Context) error {
		return mysql.Ping(ctx, db)
	})
	ser.RegisterChecker("redis", func(ctx context.Context) error {
		return rdb.Ping(ctx).Err()
	})

	errCh := make(chan error, 1)
	go func() {
		if err := ser.Start(); err != nil {
			errCh <- err
		}
	}()

	// 阻塞等待：要么启动/运行报错，要么收到系统信号要求停机
	select {
	case err := <-errCh:
		return fmt.Errorf("start http server: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received, starting graceful shutdown...",
			slog.Duration("timeout", cfg.Server.ShutdownTimeout),
		)
	}

	// 1. 立即标记停机状态，使 /readyz 返回 503（拒绝新流量）
	ser.MarkShuttingDown()

	// 2. 创建全新的超时 Context（注意不能用已经取消的 ctx！）等待在途请求完成
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := ser.Shutdown(shutdownCtx); err != nil {
		log.Error("http server graceful shutdown timed out, forcing exit", slog.Any("error", err))
		return fmt.Errorf("http server shutdown: %w", err)
	}
	log.Info("http server stopped gracefully")

	// 函数返回后，将按 LIFO 顺序触发 defer：先关闭 Redis，再关闭 MySQL
	return nil
}
