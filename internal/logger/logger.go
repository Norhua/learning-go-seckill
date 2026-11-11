package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"go-seckill/internal/config"
)

type ctxKey struct{}
type reqIDKey struct{}

// Options 定义初始化日志所需的参数
type Options struct {
	Level   string
	Format  string
	Service string
	Version string
	Output  io.Writer // 默认 os.Stdout；留出字段方便单测注入 bytes.Buffer
}

// New 根据配置创建 *slog.Logger，并附带固定字段 service 与 version
func New(cfg config.LogConfig, service, version string) *slog.Logger {
	return NewWithOptions(Options{
		Level:   cfg.Level,
		Format:  cfg.Format,
		Service: service,
		Version: version,
		Output:  os.Stdout,
	})
}

func NewWithOptions(opts Options) *slog.Logger {
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

	handlerOpts := &slog.HandlerOptions{
		Level: parseLevel(opts.Level),
	}

	var baseHandler slog.Handler
	if strings.EqualFold(opts.Format, "json") {
		baseHandler = slog.NewJSONHandler(opts.Output, handlerOpts)
	} else {
		baseHandler = slog.NewTextHandler(opts.Output, handlerOpts)
	}

	// 包装 contextHandler，使其支持从 ctx 中自动提取 request_id
	h := &contextHandler{Handler: baseHandler}

	l := slog.New(h)
	if opts.Service != "" {
		l = l.With(slog.String("service", opts.Service))
	}
	if opts.Version != "" {
		l = l.With(slog.String("version", opts.Version))
	}
	return l
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// contextHandler 自动将 ctx 中的 request_id 注入到每条日志记录中
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID, ok := RequestIDFromContext(ctx); ok && reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

// WithRequestID 将 request_id 存入 context（供 Step 8 中间件使用）
func WithRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, reqIDKey{}, reqID)
}

// RequestIDFromContext 从 context 中读取 request_id
func RequestIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	reqID, ok := ctx.Value(reqIDKey{}).(string)
	return reqID, ok
}

// WithContext 将派生的 *slog.Logger 存入 context
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext 从 context 中取出 *slog.Logger；若不存在则返回 slog.Default()
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
