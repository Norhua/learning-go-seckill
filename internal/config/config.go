package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Redis  RedisConfig  `yaml:"redis"`
	Log    LogConfig    `yaml:"log"`
}
type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type MySQLConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	Username        string        `yaml:"username"`
	Password        string        `yaml:"-"` // 敏感信息：禁止从 YAML 读取，只从环境变量注入
	Database        string        `yaml:"database"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time"`
}

// DSN 生成供 GORM mysql.Open 使用的连接串
func (m MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		m.Username, m.Password, m.Host, m.Port, m.Database,
	)
}

type RedisConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	Password     string        `yaml:"-"` // 敏感信息：禁止从 YAML 读取，只从环境变量注入
	DB           int           `yaml:"db"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		MySQL: MySQLConfig{
			Host:            "127.0.0.1",
			Port:            13306,
			Username:        "root",
			Database:        "seckill",
			MaxOpenConns:    50,
			MaxIdleConns:    25,
			ConnMaxLifetime: time.Hour,
			ConnMaxIdleTime: 10 * time.Minute,
		},
		Redis: RedisConfig{
			Host:         "127.0.0.1",
			Port:         16379,
			DB:           0,
			PoolSize:     50,
			MinIdleConns: 10,
			DialTimeout:  3 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Load 按「代码默认值 → YAML 文件 → 环境变量覆盖 → Validate」顺序加载配置。
// 当 path 为空字符串时，跳过 YAML 文件读取（方便纯环境变量启动或单测）。
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file %q: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("unmarshal config file %q: %w", path, err)
		}
	}

	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}
func applyEnvOverrides(cfg *Config) error {
	var errs []error

	// 1. Server
	setStringFromEnv("SERVER_HOST", &cfg.Server.Host)
	errs = append(errs,
		setIntFromEnv("SERVER_PORT", &cfg.Server.Port),
		setDurationFromEnv("SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout),
		setDurationFromEnv("SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout),
		setDurationFromEnv("SERVER_IDLE_TIMEOUT", &cfg.Server.IdleTimeout),
		setDurationFromEnv("SERVER_SHUTDOWN_TIMEOUT", &cfg.Server.ShutdownTimeout),
	)

	// 2. MySQL
	setStringFromEnv("MYSQL_HOST", &cfg.MySQL.Host)
	setStringFromEnv("MYSQL_USER", &cfg.MySQL.Username)
	setStringFromEnv("MYSQL_DATABASE", &cfg.MySQL.Database)
	// 优先读 MYSQL_PASSWORD；若未设置，兼容读取 docker-compose 用的 MYSQL_ROOT_PASSWORD
	setStringFromEnv("MYSQL_ROOT_PASSWORD", &cfg.MySQL.Password)
	setStringFromEnv("MYSQL_PASSWORD", &cfg.MySQL.Password)
	errs = append(errs,
		setIntFromEnv("MYSQL_PORT", &cfg.MySQL.Port),
		setIntFromEnv("MYSQL_MAX_OPEN_CONNS", &cfg.MySQL.MaxOpenConns),
		setIntFromEnv("MYSQL_MAX_IDLE_CONNS", &cfg.MySQL.MaxIdleConns),
		setDurationFromEnv("MYSQL_CONN_MAX_LIFETIME", &cfg.MySQL.ConnMaxLifetime),
		setDurationFromEnv("MYSQL_CONN_MAX_IDLE_TIME", &cfg.MySQL.ConnMaxIdleTime),
	)

	// 3. Redis
	setStringFromEnv("REDIS_HOST", &cfg.Redis.Host)
	setStringFromEnv("REDIS_PASSWORD", &cfg.Redis.Password)
	errs = append(errs,
		setIntFromEnv("REDIS_PORT", &cfg.Redis.Port),
		setIntFromEnv("REDIS_DB", &cfg.Redis.DB),
		setIntFromEnv("REDIS_POOL_SIZE", &cfg.Redis.PoolSize),
		setIntFromEnv("REDIS_MIN_IDLE_CONNS", &cfg.Redis.MinIdleConns),
		setDurationFromEnv("REDIS_DIAL_TIMEOUT", &cfg.Redis.DialTimeout),
		setDurationFromEnv("REDIS_READ_TIMEOUT", &cfg.Redis.ReadTimeout),
		setDurationFromEnv("REDIS_WRITE_TIMEOUT", &cfg.Redis.WriteTimeout),
	)

	// 4. Log
	setStringFromEnv("LOG_LEVEL", &cfg.Log.Level)
	setStringFromEnv("LOG_FORMAT", &cfg.Log.Format)

	return errors.Join(errs...)
}

// setStringFromEnv 当环境变量存在且非空时，覆盖目标字符串
func setStringFromEnv(name string, target *string) {
	if v, ok := os.LookupEnv(name); ok {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			*target = trimmed
		}
	}
}

// setIntFromEnv 当环境变量存在且非空时，解析为 int 并覆盖目标值
func setIntFromEnv(name string, target *int) error {
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return fmt.Errorf("invalid env %s=%q: %w", name, v, err)
	}
	*target = n
	return nil
}

// setDurationFromEnv 当环境变量存在且非空时，按 "5s"、"10m" 格式解析为 time.Duration
func setDurationFromEnv(name string, target *time.Duration) error {
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return fmt.Errorf("invalid env %s=%q: %w", name, v, err)
	}
	*target = d
	return nil
}

func (c *Config) Validate() error {
	var errs []error

	// 1. Server 校验
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port))
	}
	if c.Server.ReadTimeout <= 0 {
		errs = append(errs, errors.New("server.read_timeout must be > 0"))
	}
	if c.Server.WriteTimeout <= 0 {
		errs = append(errs, errors.New("server.write_timeout must be > 0"))
	}
	if c.Server.IdleTimeout <= 0 {
		errs = append(errs, errors.New("server.idle_timeout must be > 0"))
	}
	if c.Server.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("server.shutdown_timeout must be > 0"))
	}

	// 2. MySQL 校验
	if strings.TrimSpace(c.MySQL.Host) == "" {
		errs = append(errs, errors.New("mysql.host is required"))
	}
	if c.MySQL.Port < 1 || c.MySQL.Port > 65535 {
		errs = append(errs, fmt.Errorf("mysql.port must be between 1 and 65535, got %d", c.MySQL.Port))
	}
	if strings.TrimSpace(c.MySQL.Username) == "" {
		errs = append(errs, errors.New("mysql.username is required"))
	}
	if strings.TrimSpace(c.MySQL.Password) == "" {
		errs = append(errs, errors.New("mysql.password (env MYSQL_PASSWORD) is required"))
	}
	if strings.TrimSpace(c.MySQL.Database) == "" {
		errs = append(errs, errors.New("mysql.database is required"))
	}
	if c.MySQL.MaxOpenConns <= 0 {
		errs = append(errs, errors.New("mysql.max_open_conns must be > 0"))
	}
	if c.MySQL.MaxIdleConns < 0 || c.MySQL.MaxIdleConns > c.MySQL.MaxOpenConns {
		errs = append(errs, errors.New("mysql.max_idle_conns must be between 0 and max_open_conns"))
	}

	// 3. Redis 校验
	if strings.TrimSpace(c.Redis.Host) == "" {
		errs = append(errs, errors.New("redis.host is required"))
	}
	if c.Redis.Port < 1 || c.Redis.Port > 65535 {
		errs = append(errs, fmt.Errorf("redis.port must be between 1 and 65535, got %d", c.Redis.Port))
	}
	if strings.TrimSpace(c.Redis.Password) == "" {
		errs = append(errs, errors.New("redis.password (env REDIS_PASSWORD) is required"))
	}
	if c.Redis.DB < 0 {
		errs = append(errs, errors.New("redis.db must be >= 0"))
	}
	if c.Redis.PoolSize <= 0 {
		errs = append(errs, errors.New("redis.pool_size must be > 0"))
	}

	// 4. Log 校验
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("log.level must be one of [debug, info, warn, error], got %q", c.Log.Level))
	}
	switch strings.ToLower(c.Log.Format) {
	case "text", "json":
	default:
		errs = append(errs, fmt.Errorf("log.format must be one of [text, json], got %q", c.Log.Format))
	}

	return errors.Join(errs...)
}

// LogValue 实现 slog.LogValuer 接口，确保日志打印 MySQLConfig 时自动脱敏 Password
func (m MySQLConfig) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("host", m.Host),
		slog.Int("port", m.Port),
		slog.String("username", m.Username),
		slog.String("password", "[REDACTED]"),
		slog.String("database", m.Database),
		slog.Int("max_open_conns", m.MaxOpenConns),
		slog.Int("max_idle_conns", m.MaxIdleConns),
		slog.Duration("conn_max_lifetime", m.ConnMaxLifetime),
		slog.Duration("conn_max_idle_time", m.ConnMaxIdleTime),
	)
}

// LogValue 实现 slog.LogValuer 接口，确保日志打印 RedisConfig 时自动脱敏 Password
func (r RedisConfig) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("host", r.Host),
		slog.Int("port", r.Port),
		slog.String("password", "[REDACTED]"),
		slog.Int("db", r.DB),
		slog.Int("pool_size", r.PoolSize),
		slog.Int("min_idle_conns", r.MinIdleConns),
		slog.Duration("dial_timeout", r.DialTimeout),
		slog.Duration("read_timeout", r.ReadTimeout),
		slog.Duration("write_timeout", r.WriteTimeout),
	)
}

// LogValue 实现 slog.LogValuer 接口，展开子配置项以触发子结构体的 LogValue 脱敏
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("server", c.Server),
		slog.Any("mysql", c.MySQL),
		slog.Any("redis", c.Redis),
		slog.Any("log", c.Log),
	)
}
