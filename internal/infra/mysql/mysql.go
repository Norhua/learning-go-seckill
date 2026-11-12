package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"go-seckill/internal/config"
)

const pingTimeout = 5 * time.Second

// New 根据配置初始化 GORM MySQL 客户端并验证连通性
func New(ctx context.Context, cfg config.MySQLConfig) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		// 先使用 Warn 级别（只打印慢 SQL 和错误），避免压测时满屏 SQL 刷爆 I/O
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sql.DB: %w", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// 启动时带超时探活，确保快速失败（Fail-Fast）
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, errors.Join(fmt.Errorf("ping mysql: %w", err), sqlDB.Close())
	}

	return db, nil
}

// Close 关闭 GORM 底层连接池，供优雅停机时调用
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB for close: %w", err)
	}
	return sqlDB.Close()
}

// Ping 检查 MySQL 连通性，供 /readyz 就绪探针使用
func Ping(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql db is nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB: %w", err)
	}
	return sqlDB.PingContext(ctx)
}
