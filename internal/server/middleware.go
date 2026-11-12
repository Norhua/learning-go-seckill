package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go-seckill/internal/logger"
)

const HeaderXRequestID = "X-Request-ID"

// RequestID 读取请求头 X-Request-ID（若无则生成），写入响应头与 request context
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := strings.TrimSpace(c.GetHeader(HeaderXRequestID))
		if reqID == "" {
			reqID = generateRequestID()
		}

		// 1. 写入响应头，方便客户端/前端排查问题时拿着 request_id 报障
		c.Header(HeaderXRequestID, reqID)

		// 2. 注入标准库 request context，配合 logger.contextHandler 自动附加 request_id
		ctx := logger.WithRequestID(c.Request.Context(), reqID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// AccessLog 记录每次 HTTP 请求的方法、路径、状态码、耗时、客户端 IP（request_id 会由 logger 从 ctx 自动提取）
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.String("query", rawQuery),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("client_ip", c.ClientIP()),
		}

		ctx := c.Request.Context()
		switch {
		case status >= http.StatusInternalServerError:
			log.ErrorContext(ctx, "http access", attrs...)
		case status >= http.StatusBadRequest:
			log.WarnContext(ctx, "http access", attrs...)
		default:
			log.InfoContext(ctx, "http access", attrs...)
		}
	}
}

// Recovery 捕获下游 handler 的 panic，记录堆栈并返回统一格式的 500 响应
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.ErrorContext(c.Request.Context(), "http panic recovered",
					slog.Any("panic", rec),
					slog.String("path", c.Request.URL.Path),
					slog.String("stack", string(debug.Stack())),
				)
				c.Abort()
				Fail(c, ErrInternal)
			}
		}()
		c.Next()
	}
}

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极端情况下回退到纳秒时间戳
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}
