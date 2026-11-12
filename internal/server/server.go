package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"go-seckill/internal/config"
)

// Server 封装标准库 http.Server 与 Gin Engine
type Server struct {
	httpServer *http.Server
	engine     *gin.Engine
	log        *slog.Logger
}

// New 创建并配置 HTTP Server
func New(cfg config.ServerConfig, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}

	// 禁用 Gin 默认的控制台调试噪音，统一由我们的 slog 输出
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	// 允许 gin.Context.Value(key) 回退查找 c.Request.Context().Value(key)
	engine.ContextWithFallback = true
	// 当请求方法不匹配但路径存在时，交由 NoMethod 返回 405
	engine.HandleMethodNotAllowed = true

	// 注册全局中间件（注意顺序：RequestID -> AccessLog -> Recovery）
	engine.Use(
		RequestID(),
		AccessLog(log),
		Recovery(log),
	)

	// 统一 404 响应
	engine.NoRoute(func(c *gin.Context) {
		Fail(c, ErrNotFound)
	})

	// 统一 405 响应
	engine.NoMethod(func(c *gin.Context) {
		Fail(c, ErrMethodNotAllowed)
	})

	httpSrv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      engine,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &Server{
		httpServer: httpSrv,
		engine:     engine,
		log:        log,
	}
}

// Engine 暴露底层 *gin.Engine（方便后续注册路由或使用 httptest 单测）
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// Start 启动 HTTP 监听（阻塞调用，忽略正常停机产生的 http.ErrServerClosed）
func (s *Server) Start() error {
	s.log.Info("http server listening", slog.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown 优雅停机（供 Step 10 使用）
func (s *Server) Shutdown(ctx context.Context) error {
	s.log.InfoContext(ctx, "http server shutting down")
	return s.httpServer.Shutdown(ctx)
}
