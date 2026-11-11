package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"go-seckill/internal/config"
)

// 1. 验证 JSON 格式输出及固定字段 service、version
func TestNewWithOptions_JSONAndFixedFields(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions(Options{
		Level:   "info",
		Format:  "json",
		Service: "api",
		Version: "v0.1.0",
		Output:  &buf,
	})

	l.Info("server started", slog.Int("port", 8080))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON log, got %q, err: %v", buf.String(), err)
	}

	if entry["msg"] != "server started" {
		t.Errorf("expected msg='server started', got %v", entry["msg"])
	}
	if entry["level"] != "INFO" {
		t.Errorf("expected level='INFO', got %v", entry["level"])
	}
	if entry["service"] != "api" {
		t.Errorf("expected service='api', got %v", entry["service"])
	}
	if entry["version"] != "v0.1.0" {
		t.Errorf("expected version='v0.1.0', got %v", entry["version"])
	}
	if entry["port"] != float64(8080) {
		t.Errorf("expected port=8080, got %v", entry["port"])
	}
}

// 2. 验证 Text 格式输出
func TestNewWithOptions_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions(Options{
		Level:   "info",
		Format:  "text",
		Service: "api",
		Version: "v0.1.0",
		Output:  &buf,
	})

	l.Info("hello text")

	out := buf.String()
	for _, want := range []string{"level=INFO", "msg=\"hello text\"", "service=api", "version=v0.1.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected text log to contain %q, got: %s", want, out)
		}
	}
}

// 3. 验证日志级别过滤生效
func TestNewWithOptions_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions(Options{
		Level:  "warn",
		Format: "text",
		Output: &buf,
	})

	l.Debug("debug message")
	l.Info("info message")
	if buf.Len() != 0 {
		t.Fatalf("expected debug and info messages to be filtered out at warn level, got: %s", buf.String())
	}

	l.Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Errorf("expected warn message to be logged, got: %s", buf.String())
	}
}

// 4. 验证 Context 中的 request_id 自动注入（包括调用 l.With 派生子 Logger 后依然生效）
func TestContextHandler_RequestID(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions(Options{
		Level:   "info",
		Format:  "json",
		Service: "api",
		Version: "v0.1.0",
		Output:  &buf,
	})

	// 派生子 logger，验证 contextHandler.WithAttrs 没有丢失包装
	childLogger := l.With(slog.String("component", "order"))

	ctx := WithRequestID(context.Background(), "req-abc-123")
	childLogger.InfoContext(ctx, "order created")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal json log failed: %v", err)
	}

	if entry["request_id"] != "req-abc-123" {
		t.Errorf("expected request_id='req-abc-123', got %v", entry["request_id"])
	}
	if entry["component"] != "order" {
		t.Errorf("expected component='order', got %v", entry["component"])
	}
}

// 5. 验证 WithContext 与 FromContext 存取 Logger
func TestWithContextAndFromContext(t *testing.T) {
	var buf bytes.Buffer
	customLogger := NewWithOptions(Options{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	ctx := WithContext(context.Background(), customLogger)
	got := FromContext(ctx)
	if got != customLogger {
		t.Fatal("expected FromContext to return the logger stored by WithContext")
	}

	got.Info("from context")
	if !strings.Contains(buf.String(), "from context") {
		t.Errorf("expected log written to custom buffer, got: %s", buf.String())
	}

	// 空 context 兜底返回 slog.Default()
	if FromContext(context.Background()) == nil {
		t.Error("expected FromContext on empty ctx to return non-nil default logger")
	}
}

// 6. 验证打印 Config 时密码被自动脱敏为 [REDACTED]，绝不泄露明文密码
func TestConfigLogRedaction(t *testing.T) {
	mysqlSecret := "super_secret_mysql_pwd_999"
	redisSecret := "super_secret_redis_pwd_888"

	t.Setenv("MYSQL_PASSWORD", mysqlSecret)
	t.Setenv("REDIS_PASSWORD", redisSecret)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	var buf bytes.Buffer
	l := NewWithOptions(Options{
		Level:   "info",
		Format:  "json",
		Service: "api",
		Version: "v0.1.0",
		Output:  &buf,
	})

	l.Info("starting server with configuration",
		slog.Any("server", cfg.Server),
		slog.Any("mysql", cfg.MySQL),
		slog.Any("redis", cfg.Redis),
		slog.Any("log", cfg.Log),
	)

	out := buf.String()
	if strings.Contains(out, mysqlSecret) {
		t.Fatalf("SECURITY LEAK: mysql password leaked in log output: %s", out)
	}
	if strings.Contains(out, redisSecret) {
		t.Fatalf("SECURITY LEAK: redis password leaked in log output: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected '[REDACTED]' placeholder in log output, got: %s", out)
	}
}
