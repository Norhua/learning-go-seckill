package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一 HTTP JSON 响应体结构
type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

// Success 返回成功响应（HTTP 200，code=0）
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code: OK.Code,
		Msg:  OK.Msg,
		Data: data,
	})
}

// Fail 根据 error 类型自动映射 HTTP 状态码与业务错误码。
// 若传入的不是 *ErrorCode（如底层数据库或网络未知错误），统一兜底为 ErrInternal (500)，防止内部敏感信息泄露。
func Fail(c *gin.Context, err error) {
	if err == nil {
		Success(c, nil)
		return
	}

	var errCode *ErrorCode
	if errors.As(err, &errCode) && errCode != nil {
		c.JSON(errCode.HTTPStatus, Response{
			Code: errCode.Code,
			Msg:  errCode.Msg,
		})
		return
	}

	c.JSON(ErrInternal.HTTPStatus, Response{
		Code: ErrInternal.Code,
		Msg:  ErrInternal.Msg,
	})
}
