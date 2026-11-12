package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"go-seckill/internal/config"
	"go-seckill/internal/logger"
)

func newTestServer(buf *bytes.Buffer) *Server {
	log := logger.NewWithOptions(logger.Options{
		Level:   "info",
		Format:  "json",
		Service: "api-test",
		Version: "v0.1.0",
		Output:  buf,
	})
	cfg := config.ServerConfig{
		Host:            "127.0.0.1",
		Port:            8080,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		IdleTimeout:     10 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	}
	return New(cfg, log)
}

// 1. 验证统一响应 Success 与 Fail 的错误码映射（含 errors.As 包装识别与未知错误脱敏兜底）
func TestResponseAndErrorCodeMapping(t *testing.T) {
	var logBuf bytes.Buffer
	srv := newTestServer(&logBuf)

	srv.Engine().GET("/test/ok", func(c *gin.Context) {
		Success(c, gin.H{"item_id": 42})
	})
	srv.Engine().GET("/test/custom-err", func(c *gin.Context) {
		// 测试 WithMsg 派生 + fmt.Errorf("%w") 包装后 errors.As 仍能识别
		wrappedErr := fmt.Errorf("service layer failed: %w", ErrInvalidParam.WithMsg("activity_id must be > 0"))
		Fail(c, wrappedErr)
	})
	srv.Engine().GET("/test/unknown-err", func(c *gin.Context) {
		// 模拟底层数据库报错，验证不会把敏感内部错误暴露给客户端
		Fail(c, errors.New("Error 1062: Duplicate entry 'xxx' for key 'uk_request_id'"))
	})

	t.Run("success response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test/ok", nil)
		rec := httptest.NewRecorder()
		srv.Engine().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.Code != 0 || resp.Msg != "ok" {
			t.Errorf("unexpected response: %+v", resp)
		}
	})

	t.Run("wrapped ErrorCode mapping", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test/custom-err", nil)
		rec := httptest.NewRecorder()
		srv.Engine().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rec.Code)
		}
		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.Code != ErrInvalidParam.Code || resp.Msg != "activity_id must be > 0" {
			t.Errorf("expected code=%d msg='activity_id must be > 0', got %+v", ErrInvalidParam.Code, resp)
		}
	})

	t.Run("unknown error masked as 500", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test/unknown-err", nil)
		rec := httptest.NewRecorder()
		srv.Engine().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "Duplicate entry") {
			t.Fatalf("SECURITY LEAK: internal sql error leaked in HTTP response: %s", rec.Body.String())
		}
		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.Code != ErrInternal.Code || resp.Msg != ErrInternal.Msg {
			t.Errorf("expected internal error fallback, got %+v", resp)
		}
	})
}

// 2. 验证 404 NoRoute 与 405 NoMethod 统一响应
func TestNoRouteAndNoMethod(t *testing.T) {
	var logBuf bytes.Buffer
	srv := newTestServer(&logBuf)

	// 请求不存在的路由 -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/non-existent-path", nil)
	rec404 := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec404, req404)

	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec404.Code)
	}

	// 用 POST 请求只注册了 GET 的 /healthz -> 405
	req405 := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec405 := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec405, req405)

	if rec405.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec405.Code)
	}
}

// 3. 验证中间件：RequestID 生成/透传、AccessLog 记录、Recovery 捕获 panic
func TestMiddlewares(t *testing.T) {
	t.Run("RequestID generated when missing and propagated to log", func(t *testing.T) {
		var logBuf bytes.Buffer
		srv := newTestServer(&logBuf)

		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		srv.Engine().ServeHTTP(rec, req)

		reqID := rec.Header().Get(HeaderXRequestID)
		if len(reqID) != 32 {
			t.Fatalf("expected 32-char hex X-Request-ID, got %q", reqID)
		}
		if !strings.Contains(logBuf.String(), reqID) {
			t.Errorf("expected access log to contain request_id %q, got log: %s", reqID, logBuf.String())
		}
	})

	t.Run("RequestID preserved when provided by client", func(t *testing.T) {
		var logBuf bytes.Buffer
		srv := newTestServer(&logBuf)

		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set(HeaderXRequestID, "client-req-id-9527")
		rec := httptest.NewRecorder()
		srv.Engine().ServeHTTP(rec, req)

		if got := rec.Header().Get(HeaderXRequestID); got != "client-req-id-9527" {
			t.Errorf("expected X-Request-ID='client-req-id-9527', got %q", got)
		}
		if !strings.Contains(logBuf.String(), "client-req-id-9527") {
			t.Errorf("expected log to contain custom request_id, got: %s", logBuf.String())
		}
	})

	t.Run("Recovery catches panic and returns unified 500", func(t *testing.T) {
		var logBuf bytes.Buffer
		srv := newTestServer(&logBuf)

		srv.Engine().GET("/test/panic", func(c *gin.Context) {
			panic("unexpected nil pointer in handler")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/panic", nil)
		rec := httptest.NewRecorder()
		srv.Engine().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500 on panic, got %d", rec.Code)
		}
		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal panic response: %v", err)
		}
		if resp.Code != ErrInternal.Code {
			t.Errorf("expected code=%d, got %d", ErrInternal.Code, resp.Code)
		}
		if !strings.Contains(logBuf.String(), "http panic recovered") {
			t.Errorf("expected panic log, got: %s", logBuf.String())
		}
	})
}

// 4. 验证 /healthz 与 /readyz 探针状态切换（无 Docker 纯内存测试）
func TestHealthAndReadyz(t *testing.T) {
	var logBuf bytes.Buffer
	srv := newTestServer(&logBuf)

	redisErr := errors.New("dial tcp: connection refused")
	redisHealthy := true

	srv.RegisterChecker("mysql", func(ctx context.Context) error {
		return nil
	})
	srv.RegisterChecker("redis", func(ctx context.Context) error {
		if !redisHealthy {
			return redisErr
		}
		return nil
	})

	// (a) 所有依赖正常 -> /readyz 返回 200
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /readyz 200 when all healthy, got %d: %s", rec.Code, rec.Body.String())
	}

	// (b) 模拟 Redis 故障 -> /readyz 返回 503，但 /healthz 依然返回 200
	redisHealthy = false
	recReady := httptest.NewRecorder()
	srv.Engine().ServeHTTP(recReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /readyz 503 when redis down, got %d", recReady.Code)
	}
	if !strings.Contains(recReady.Body.String(), "connection refused") {
		t.Errorf("expected /readyz body to mention redis error, got: %s", recReady.Body.String())
	}

	recLive := httptest.NewRecorder()
	srv.Engine().ServeHTTP(recLive, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recLive.Code != http.StatusOK {
		t.Fatalf("expected /healthz 200 even when redis down, got %d", recLive.Code)
	}

	// (c) 恢复 Redis，但标记服务进入停机状态 -> /readyz 立即返回 503 shutting_down
	redisHealthy = true
	srv.MarkShuttingDown()
	recShut := httptest.NewRecorder()
	srv.Engine().ServeHTTP(recShut, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recShut.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /readyz 503 when shutting down, got %d", recShut.Code)
	}
	if !strings.Contains(recShut.Body.String(), "shutting_down") {
		t.Errorf("expected shutting_down status, got: %s", recShut.Body.String())
	}
}
