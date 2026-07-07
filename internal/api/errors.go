package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorCode 是结构化错误码，方便 Agent 自动重试逻辑识别。
type ErrorCode string

const (
	CodeInvalidInput       ErrorCode = "INVALID_INPUT"
	CodeInvalidDescription ErrorCode = "INVALID_DESCRIPTION"
	CodeInvalidCustomCode  ErrorCode = "INVALID_CUSTOM_CODE"
	CodeInvalidFilePath    ErrorCode = "INVALID_FILE_PATH"
	CodeContentTooLarge    ErrorCode = "CONTENT_TOO_LARGE"
	CodeRateLimited        ErrorCode = "RATE_LIMITED"
	CodeNotFound           ErrorCode = "NOT_FOUND"
	CodeVersionLocked      ErrorCode = "VERSION_LOCKED"
	CodeForbidden          ErrorCode = "FORBIDDEN"
	CodeConflict           ErrorCode = "CONFLICT"
	CodeUnauthorized       ErrorCode = "UNAUTHORIZED"
	CodeMethodNotAllowed   ErrorCode = "METHOD_NOT_ALLOWED"
	CodeInternal           ErrorCode = "INTERNAL"

	CodeZipOpenFailed      ErrorCode = "ZIP_OPEN_FAILED"
	CodeZipUnsafePath      ErrorCode = "ZIP_UNSAFE_PATH"
	CodeZipEntryReadFailed ErrorCode = "ZIP_ENTRY_READ_FAILED"
	CodeZipFileTooLarge    ErrorCode = "ZIP_FILE_TOO_LARGE"
	CodeZipTotalTooLarge   ErrorCode = "ZIP_TOTAL_TOO_LARGE"
	CodeZipTooManyFiles    ErrorCode = "ZIP_TOO_MANY_FILES"
	CodeZipEmpty           ErrorCode = "ZIP_EMPTY"
	CodeZipDuplicatePath   ErrorCode = "ZIP_DUPLICATE_PATH"
	CodeZipEntryMissing    ErrorCode = "ZIP_ENTRY_MISSING"
	CodeZipAmbiguousEntry  ErrorCode = "ZIP_AMBIGUOUS_ENTRY"
)

// httpStatus 把 ErrorCode 映射到 HTTP 状态码。
// 对齐项目 OpenAPI 3.1：VERSION_LOCKED 走 423 Locked。
func (c ErrorCode) httpStatus() int {
	switch c {
	case CodeInvalidInput, CodeInvalidDescription, CodeInvalidCustomCode, CodeInvalidFilePath,
		CodeZipOpenFailed, CodeZipUnsafePath, CodeZipEntryReadFailed, CodeZipEmpty,
		CodeZipDuplicatePath, CodeZipEntryMissing, CodeZipAmbiguousEntry:
		return http.StatusBadRequest
	case CodeContentTooLarge, CodeZipFileTooLarge, CodeZipTotalTooLarge, CodeZipTooManyFiles:
		return http.StatusRequestEntityTooLarge
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeNotFound:
		return http.StatusNotFound
	case CodeVersionLocked:
		return http.StatusLocked // 423
	case CodeConflict:
		return http.StatusConflict
	case CodeForbidden:
		return http.StatusForbidden
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	default:
		return http.StatusInternalServerError
	}
}

// APIError 是统一的错误响应结构。
type APIError struct {
	Success           bool      `json:"success"`
	ErrorCode         ErrorCode `json:"errorCode"`
	Stage             string    `json:"stage,omitempty"`
	Detail            string    `json:"detail"`
	Hint              string    `json:"hint,omitempty"`
	RetryAfterSeconds *int      `json:"retryAfterSeconds,omitempty"`
	RequestID         string    `json:"requestId,omitempty"`
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.ErrorCode, e.Detail)
}

// NewError 构造一个 APIError。
func NewError(code ErrorCode, stage, detail string) *APIError {
	return &APIError{
		Success:   false,
		ErrorCode: code,
		Stage:     stage,
		Detail:    detail,
	}
}

// WithHint 加提示。
func (e *APIError) WithHint(hint string) *APIError {
	e.Hint = hint
	return e
}

// WithRetryAfter 加重试等待（仅用于 429）。
func (e *APIError) WithRetryAfter(sec int) *APIError {
	e.RetryAfterSeconds = &sec
	return e
}

// WithRequestID 加请求 ID。
func (e *APIError) WithRequestID(id string) *APIError {
	e.RequestID = id
	return e
}

// writeError 把 APIError 序列化写入 HTTP 响应。
// 对 RATE_LIMITED 错误，额外加 Retry-After HTTP 头（标准头，比 JSON 字段更通用）。
func writeError(w http.ResponseWriter, e *APIError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if e.ErrorCode == CodeRateLimited && e.RetryAfterSeconds != nil {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", *e.RetryAfterSeconds))
	}
	w.WriteHeader(e.ErrorCode.httpStatus())
	_ = json.NewEncoder(w).Encode(e)
}
