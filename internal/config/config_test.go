package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setRequiredSecrets 清空可能干扰测试的宿主机/Makefile 环境变量，并统一设置必填的敏感环境变量
func setRequiredSecrets(t *testing.T) {
	t.Helper()
	envKeys := []string{
		"SERVER_HOST", "SERVER_PORT", "SERVER_READ_TIMEOUT", "SERVER_WRITE_TIMEOUT", "SERVER_IDLE_TIMEOUT", "SERVER_SHUTDOWN_TIMEOUT",
		"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_DATABASE", "MYSQL_ROOT_PASSWORD", "MYSQL_PASSWORD",
		"MYSQL_MAX_OPEN_CONNS", "MYSQL_MAX_IDLE_CONNS", "MYSQL_CONN_MAX_LIFETIME", "MYSQL_CONN_MAX_IDLE_TIME",
		"REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB", "REDIS_POOL_SIZE", "REDIS_MIN_IDLE_CONNS",
		"REDIS_DIAL_TIMEOUT", "REDIS_READ_TIMEOUT", "REDIS_WRITE_TIMEOUT",
		"LOG_LEVEL", "LOG_FORMAT",
	}
	for _, key := range envKeys {
		t.Setenv(key, "")
	}
	t.Setenv("MYSQL_PASSWORD", "test_mysql_pwd")
	t.Setenv("REDIS_PASSWORD", "test_redis_pwd")
}

// 1. 验证代码默认值生效
func TestLoad_Defaults(t *testing.T) {
	setRequiredSecrets(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default server.port=8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 5*time.Second {
		t.Errorf("expected default server.read_timeout=5s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.MySQL.Port != 13306 {
		t.Errorf("expected default mysql.port=13306, got %d", cfg.MySQL.Port)
	}
	if cfg.MySQL.Password != "test_mysql_pwd" {
		t.Errorf("expected mysql.password from env, got %q", cfg.MySQL.Password)
	}
	if cfg.Redis.Port != 16379 {
		t.Errorf("expected default redis.port=16379, got %d", cfg.Redis.Port)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "text" {
		t.Errorf("unexpected default log config: %+v", cfg.Log)
	}
}

// 2. 验证加载优先级：默认值 < YAML < 环境变量，且 YAML 中的密码会被忽略
func TestLoad_YAMLAndEnvOverride(t *testing.T) {
	setRequiredSecrets(t)

	yamlContent := `
server:
  port: 9000
  read_timeout: 8s
mysql:
  host: "10.0.0.1"
  database: "seckill_yaml"
  password: "password_in_yaml_should_be_ignored"
redis:
  db: 2
  password: "redis_yaml_pwd_should_be_ignored"
log:
  level: "debug"
  format: "json"
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("write temp yaml failed: %v", err)
	}

	// 用环境变量覆盖 YAML 中的 server.port 和 log.level
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("LOG_LEVEL", "warn")

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load(%q) unexpected error: %v", tmpFile, err)
	}

	// 环境变量覆盖了 YAML (9000 -> 9090, debug -> warn)
	if cfg.Server.Port != 9090 {
		t.Errorf("expected env override server.port=9090, got %d", cfg.Server.Port)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("expected env override log.level=warn, got %q", cfg.Log.Level)
	}

	// YAML 覆盖了代码默认值
	if cfg.Server.ReadTimeout != 8*time.Second {
		t.Errorf("expected yaml server.read_timeout=8s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.MySQL.Host != "10.0.0.1" || cfg.MySQL.Database != "seckill_yaml" {
		t.Errorf("expected yaml mysql host/db override, got host=%q db=%q", cfg.MySQL.Host, cfg.MySQL.Database)
	}
	if cfg.Redis.DB != 2 {
		t.Errorf("expected yaml redis.db=2, got %d", cfg.Redis.DB)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("expected yaml log.format=json, got %q", cfg.Log.Format)
	}

	// 未在 YAML 和环境变量中出现的字段，保留代码默认值
	if cfg.MySQL.MaxOpenConns != 50 {
		t.Errorf("expected default mysql.max_open_conns=50, got %d", cfg.MySQL.MaxOpenConns)
	}
}

// 3. 验证即使 YAML 写了 password，只要环境变量没提供密码，仍然会报错（证明 yaml:"-" 生效）
func TestLoad_MissingRequiredSecret(t *testing.T) {
	t.Setenv("MYSQL_ROOT_PASSWORD", "")
	t.Setenv("MYSQL_PASSWORD", "")
	t.Setenv("REDIS_PASSWORD", "")

	yamlContent := `
mysql:
  password: "leaked_yaml_password"
redis:
  password: "leaked_yaml_password"
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("write temp yaml failed: %v", err)
	}

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected error when MYSQL_PASSWORD and REDIS_PASSWORD are missing, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "mysql.password") {
		t.Errorf("expected error to mention 'mysql.password', got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "redis.password") {
		t.Errorf("expected error to mention 'redis.password', got: %v", errMsg)
	}
}

// 4. 验证环境变量类型格式非法时报错
func TestLoad_InvalidEnvFormat(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("SERVER_PORT", "not_a_number")

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error for invalid SERVER_PORT, got nil")
	}
	if !strings.Contains(err.Error(), "SERVER_PORT") {
		t.Errorf("expected error to mention 'SERVER_PORT', got: %v", err)
	}
}

// 5. 验证 Validate() 对非法取值的拦截与字段提示
func TestValidate_InvalidFields(t *testing.T) {
	cfg := defaultConfig()
	cfg.MySQL.Password = "pwd"
	cfg.Redis.Password = "pwd"

	// 人为构造多处非法配置
	cfg.Server.Port = 70000      // 端口越界
	cfg.Server.ReadTimeout = 0   // 超时 <= 0
	cfg.MySQL.MaxIdleConns = 100 // Idle > Open (50)
	cfg.Redis.DB = -1            // DB < 0
	cfg.Log.Level = "verbose"    // 非法日志级别
	cfg.Log.Format = "xml"       // 非法日志格式

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	errMsg := err.Error()
	expectedFields := []string{
		"server.port",
		"server.read_timeout",
		"mysql.max_idle_conns",
		"redis.db",
		"log.level",
		"log.format",
	}
	for _, field := range expectedFields {
		if !strings.Contains(errMsg, field) {
			t.Errorf("expected validation error to contain field %q, got:\n%s", field, errMsg)
		}
	}
}

// 6. 验证 Addr() 和 DSN() 辅助方法格式正确
func TestHelperMethods(t *testing.T) {
	cfg := defaultConfig()
	cfg.MySQL.Password = "secret123"

	if got := cfg.Server.Addr(); got != "0.0.0.0:8080" {
		t.Errorf("Server.Addr() = %q, want %q", got, "0.0.0.0:8080")
	}
	if got := cfg.Redis.Addr(); got != "127.0.0.1:16379" {
		t.Errorf("Redis.Addr() = %q, want %q", got, "127.0.0.1:16379")
	}
	wantDSN := "root:secret123@tcp(127.0.0.1:13306)/seckill?charset=utf8mb4&parseTime=True&loc=Local"
	if got := cfg.MySQL.DSN(); got != wantDSN {
		t.Errorf("MySQL.DSN() = %q, want %q", got, wantDSN)
	}
}
