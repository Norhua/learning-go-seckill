//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-seckill/internal/config"
	"go-seckill/internal/infra/mysql"
	"go-seckill/internal/infra/redis"
	"go-seckill/internal/logger"
	"go-seckill/internal/server"
)

// pingTestRecord 对应 migrations/000001_init_ping_test.up.sql 中的 ping_test 表
type pingTestRecord struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Message   string    `gorm:"column:message"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (pingTestRecord) TableName() string {
	return "ping_test"
}

func TestIntegration_ReadyzAndMigration(t *testing.T) {
	// 加载环境变量中的端口与密码配置（不依赖相对路径的 config.yaml）
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config failed (请确保设置了 MYSQL_PASSWORD 和 REDIS_PASSWORD): %v", err)
	}

	// 强制隔离到测试专用的 seckill_test 库和 Redis DB 1，防止污染业务数据
	cfg.MySQL.Database = "seckill_test"
	cfg.Redis.DB = 1

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. 初始化真实 MySQL 连接
	db, err := mysql.New(ctx, cfg.MySQL)
	if err != nil {
		t.Fatalf("connect to mysql seckill_test failed: %v", err)
	}
	defer func() {
		_ = mysql.Close(db)
	}()

	// 2. 初始化真实 Redis 连接
	rdb, err := redis.New(ctx, cfg.Redis)
	if err != nil {
		t.Fatalf("connect to redis failed: %v", err)
	}
	defer func() {
		_ = rdb.Close()
	}()

	// 3. 验证 seckill_test 库已执行迁移（ping_test 表可正常插入与查询）
	record := pingTestRecord{Message: "integration_test_ping"}
	if err := db.WithContext(ctx).Create(&record).Error; err != nil {
		t.Fatalf("insert into ping_test failed (请确认已对 seckill_test 执行迁移): %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(context.Background()).Delete(&pingTestRecord{}, record.ID).Error
	})

	var got pingTestRecord
	if err := db.WithContext(ctx).First(&got, record.ID).Error; err != nil {
		t.Fatalf("query ping_test by id=%d failed: %v", record.ID, err)
	}
	if got.Message != "integration_test_ping" {
		t.Errorf("expected message='integration_test_ping', got %q", got.Message)
	}

	// 4. 组装 Server 并验证真实依赖下的 /readyz 返回 200
	var logBuf bytes.Buffer
	log := logger.NewWithOptions(logger.Options{
		Level:  "info",
		Format: "json",
		Output: &logBuf,
	})
	srv := server.New(cfg.Server, log)
	srv.RegisterChecker("mysql", func(ctx context.Context) error {
		return mysql.Ping(ctx, db)
	})
	srv.RegisterChecker("redis", func(ctx context.Context) error {
		return rdb.Ping(ctx).Err()
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected /readyz status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal /readyz body: %v", err)
	}
	if resp.Data.Status != "ready" || resp.Data.Checks["mysql"] != "ok" || resp.Data.Checks["redis"] != "ok" {
		t.Errorf("unexpected /readyz data: %+v", resp.Data)
	}
}
