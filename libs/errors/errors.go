package errors

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ErrorCode string

const (
	CodeInternal       ErrorCode = "INTERNAL"
	CodeNotFound       ErrorCode = "NOT_FOUND"
	CodeAlreadyExists  ErrorCode = "ALREADY_EXISTS"
	CodeInvalidInput   ErrorCode = "INVALID_INPUT"
	CodeUnauthorized   ErrorCode = "UNAUTHORIZED"
	CodeForbidden      ErrorCode = "FORBIDDEN"
	CodeRateLimited    ErrorCode = "RATE_LIMITED"
	CodeUnavailable    ErrorCode = "UNAVAILABLE"
	CodeTimeout        ErrorCode = "TIMEOUT"
	CodeConflict       ErrorCode = "CONFLICT"
	CodePrecondition   ErrorCode = "PRECONDITION_FAILED"
	CodeUnprocessable  ErrorCode = "UNPROCESSABLE"
	CodeExternalAPI    ErrorCode = "EXTERNAL_API_ERROR"
	CodeVideoNotReady  ErrorCode = "VIDEO_NOT_READY"
	CodeQuotaExceeded  ErrorCode = "QUOTA_EXCEEDED"
)

type AppError struct {
	Code       ErrorCode         `json:"code"`
	Message    string            `json:"message"`
	Details    map[string]string `json:"details,omitempty"`
	Cause      error             `json:"-"`
	StatusCode int               `json:"-"`
	GRPCCode   codes.Code        `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func (e *AppError) WithDetail(key, value string) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}

func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

func (e *AppError) ToGRPCStatus() *status.Status {
	return status.New(e.GRPCCode, e.Message)
}

func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: codeToHTTPStatus(code),
		GRPCCode:   codeToGRPCCode(code),
	}
}

func Wrap(err error, code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		Cause:      err,
		StatusCode: codeToHTTPStatus(code),
		GRPCCode:   codeToGRPCCode(code),
	}
}

func Internal(message string) *AppError {
	return New(CodeInternal, message)
}

func NotFound(resource string) *AppError {
	return New(CodeNotFound, fmt.Sprintf("%s not found", resource))
}

func AlreadyExists(resource string) *AppError {
	return New(CodeAlreadyExists, fmt.Sprintf("%s already exists", resource))
}

func InvalidInput(message string) *AppError {
	return New(CodeInvalidInput, message)
}

func Unauthorized(message string) *AppError {
	if message == "" {
		message = "authentication required"
	}
	return New(CodeUnauthorized, message)
}

func Forbidden(message string) *AppError {
	if message == "" {
		message = "access denied"
	}
	return New(CodeForbidden, message)
}

func RateLimited() *AppError {
	return New(CodeRateLimited, "rate limit exceeded")
}

func ServiceUnavailable(message string) *AppError {
	if message == "" {
		message = "service temporarily unavailable"
	}
	return New(CodeUnavailable, message)
}

func ExternalAPI(service string, err error) *AppError {
	return Wrap(err, CodeExternalAPI, fmt.Sprintf("external API error: %s", service))
}

func VideoNotReady(videoID string) *AppError {
	return New(CodeVideoNotReady, "video is still processing").WithDetail("video_id", videoID)
}

func codeToHTTPStatus(code ErrorCode) int {
	switch code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists:
		return http.StatusConflict
	case CodeInvalidInput:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeTimeout:
		return http.StatusGatewayTimeout
	case CodeConflict:
		return http.StatusConflict
	case CodePrecondition:
		return http.StatusPreconditionFailed
	case CodeUnprocessable:
		return http.StatusUnprocessableEntity
	case CodeVideoNotReady:
		return http.StatusAccepted
	case CodeQuotaExceeded:
		return http.StatusPaymentRequired
	default:
		return http.StatusInternalServerError
	}
}

func codeToGRPCCode(code ErrorCode) codes.Code {
	switch code {
	case CodeNotFound:
		return codes.NotFound
	case CodeAlreadyExists:
		return codes.AlreadyExists
	case CodeInvalidInput:
		return codes.InvalidArgument
	case CodeUnauthorized:
		return codes.Unauthenticated
	case CodeForbidden:
		return codes.PermissionDenied
	case CodeRateLimited:
		return codes.ResourceExhausted
	case CodeUnavailable:
		return codes.Unavailable
	case CodeTimeout:
		return codes.DeadlineExceeded
	case CodeConflict:
		return codes.Aborted
	case CodePrecondition:
		return codes.FailedPrecondition
	default:
		return codes.Internal
	}
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
