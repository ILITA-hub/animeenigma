package httputil

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/go-chi/render"
)

// Response is a generic API response wrapper
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

// ErrorBody represents an error in the response
type ErrorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// Meta contains optional metadata
type Meta struct {
	Page       int   `json:"page,omitempty"`
	PageSize   int   `json:"page_size,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
	TotalCount int64 `json:"total_count,omitempty"`
}

// JSON writes a JSON response
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: status >= 200 && status < 300,
		Data:    data,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Default().Errorw("failed to encode response", "error", err)
	}
}

// JSONWithMeta writes a JSON response with metadata
func JSONWithMeta(w http.ResponseWriter, status int, data interface{}, meta Meta) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: true,
		Data:    data,
		Meta:    &meta,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Default().Errorw("failed to encode response", "error", err)
	}
}

// Error writes an error response
func Error(w http.ResponseWriter, err error) {
	var appErr *errors.AppError
	if e, ok := errors.IsAppError(err); ok {
		appErr = e
	} else {
		appErr = errors.Internal(err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.StatusCode)

	resp := Response{
		Success: false,
		Error: &ErrorBody{
			Code:    string(appErr.Code),
			Message: appErr.Message,
			Details: appErr.Details,
		},
	}

	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		logger.Default().Errorw("failed to encode error response", "error", encErr)
	}
}

// NotFound writes a 404 response
func NotFound(w http.ResponseWriter, resource string) {
	Error(w, errors.NotFound(resource))
}

// BadRequest writes a 400 response
func BadRequest(w http.ResponseWriter, message string) {
	Error(w, errors.InvalidInput(message))
}

// Unauthorized writes a 401 response
func Unauthorized(w http.ResponseWriter) {
	Error(w, errors.Unauthorized(""))
}

// Forbidden writes a 403 response
func Forbidden(w http.ResponseWriter) {
	Error(w, errors.Forbidden(""))
}

// NoContent writes a 204 response
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Created writes a 201 response
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, data)
}

// OK writes a 200 response
func OK(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, data)
}

// Bind decodes JSON request body
func Bind(r *http.Request, v interface{}) error {
	if err := render.DecodeJSON(r.Body, v); err != nil {
		return errors.InvalidInput("invalid request body")
	}
	return nil
}

// BindAndValidate decodes and validates JSON request body
func BindAndValidate(r *http.Request, v Validator) error {
	if err := Bind(r, v); err != nil {
		return err
	}
	if err := v.Validate(); err != nil {
		return errors.InvalidInput(err.Error())
	}
	return nil
}

// Validator interface for request validation
type Validator interface {
	Validate() error
}
