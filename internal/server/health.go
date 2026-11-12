package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const readinessCheckTimeout = 2 * time.Second

// HealthChecker 定义依赖组件的探活函数
type HealthChecker func(ctx context.Context) error

// RegisterChecker 注册一个需要在 /readyz 中检查的下游依赖（如 mysql、redis）
func (s *Server) RegisterChecker(name string, checker HealthChecker) {
	if checker == nil {
		return
	}

	if s.checkers == nil {
		s.checkers = make(map[string]HealthChecker)
	}
	s.checkers[name] = checker
}

// MarkShuttingDown 标记服务进入停机阶段，使 /readyz 立即返回 503（配合 Step 10 优雅停机）
func (s *Server) MarkShuttingDown() {
	s.shuttingDown.Store(true)
}

// handleHealthz 存活检查（Liveness）：只要进程活着就返回 200，绝不检查外部依赖
func (s *Server) handleHealthz(c *gin.Context) {
	Success(c, gin.H{
		"status": "ok",
	})
}

// handleReadyz 就绪检查（Readiness）：检查是否正在停机，并带超时检查所有注册的依赖
func (s *Server) handleReadyz(c *gin.Context) {
	// 1. 若正在停机，直接拒绝新流量
	if s.shuttingDown.Load() {
		c.JSON(ErrServiceUnavailable.HTTPStatus, Response{
			Code: ErrServiceUnavailable.Code,
			Msg:  "server is shutting down",
			Data: gin.H{"status": "shutting_down"},
		})
		return
	}

	// 2. 带超时检查各个下游依赖
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessCheckTimeout)
	defer cancel()

	checks := make(map[string]string, len(s.checkers))
	allReady := true

	for name, checker := range s.checkers {
		if err := checker(ctx); err != nil {
			allReady = false
			checks[name] = "unavailable: " + err.Error()
			s.log.WarnContext(ctx, "readiness check failed",
				slog.String("component", name),
				slog.Any("error", err))
		} else {
			checks[name] = "ok"
		}
	}

	if !allReady {
		c.JSON(http.StatusServiceUnavailable, Response{
			Code: ErrServiceUnavailable.Code,
			Msg:  ErrServiceUnavailable.Msg,
			Data: gin.H{
				"status": "not_ready",
				"checks": checks,
			},
		})
		return
	}

	Success(c, gin.H{
		"status": "ready",
		"checks": checks,
	})
}
