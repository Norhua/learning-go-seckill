package server

import (
	"fmt"
	"net/http"
)

// ErrorCode 统一业务错误码定义，同时实现 Go 标准 error 接口
type ErrorCode struct {
	Code       int    // 业务错误码（0 表示成功，非 0 按模块划分号段）
	HTTPStatus int    // 映射的 HTTP 状态码
	Msg        string // 对外暴露的安全错误提示（绝不能包含 SQL 报错等内部细节）
}

func (e *ErrorCode) Error() string {
	return fmt.Sprintf("code=%d, msg=%s", e.Code, e.Msg)
}

// WithMsg 基于已有错误码派生自定义提示信息（例如参数校验时指明具体哪个字段错了）
func (e *ErrorCode) WithMsg(msg string) *ErrorCode {
	return &ErrorCode{
		Code:       e.Code,
		HTTPStatus: e.HTTPStatus,
		Msg:        msg,
	}
}

// 号段规范：
//
//	0     : 成功
//	1xxxx : 通用/系统级错误 (Common)
//	2xxxx : 用户与认证模块 (User / Auth)
//	3xxxx : 秒杀与活动模块 (Seckill / Activity)
//	4xxxx : 订单模块 (Order)
var (
	// 0: 成功
	OK = &ErrorCode{Code: 0, HTTPStatus: http.StatusOK, Msg: "ok"}

	// 1xxxx: 通用错误
	ErrInternal           = &ErrorCode{Code: 10000, HTTPStatus: http.StatusInternalServerError, Msg: "internal server error"}
	ErrInvalidParam       = &ErrorCode{Code: 10001, HTTPStatus: http.StatusBadRequest, Msg: "invalid parameters"}
	ErrNotFound           = &ErrorCode{Code: 10002, HTTPStatus: http.StatusNotFound, Msg: "resource not found"}
	ErrMethodNotAllowed   = &ErrorCode{Code: 10003, HTTPStatus: http.StatusMethodNotAllowed, Msg: "method not allowed"}
	ErrTooManyRequests    = &ErrorCode{Code: 10004, HTTPStatus: http.StatusTooManyRequests, Msg: "too many requests"}
	ErrServiceUnavailable = &ErrorCode{Code: 10005, HTTPStatus: http.StatusServiceUnavailable, Msg: "service unavailable"}

	// 2xxxx: 用户与认证模块（为阶段 1 预留）
	ErrUnauthorized = &ErrorCode{Code: 20001, HTTPStatus: http.StatusUnauthorized, Msg: "unauthorized"}
	ErrForbidden    = &ErrorCode{Code: 20002, HTTPStatus: http.StatusForbidden, Msg: "forbidden"}

	// 3xxxx: 秒杀模块（为阶段 2~3 预留）
	ErrActivityNotOngoing = &ErrorCode{Code: 30001, HTTPStatus: http.StatusBadRequest, Msg: "seckill activity is not ongoing"}
	ErrSoldOut            = &ErrorCode{Code: 30002, HTTPStatus: http.StatusConflict, Msg: "stock sold out"}
	ErrLimitExceeded      = &ErrorCode{Code: 30003, HTTPStatus: http.StatusConflict, Msg: "purchase limit exceeded"}

	// 4xxxx: 订单模块（为阶段 1~5 预留）
	ErrOrderNotFound     = &ErrorCode{Code: 40001, HTTPStatus: http.StatusNotFound, Msg: "order not found"}
	ErrOrderStateInvalid = &ErrorCode{Code: 40002, HTTPStatus: http.StatusConflict, Msg: "invalid order state transition"}
)
