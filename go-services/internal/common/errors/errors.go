package errors

import (
	"fmt"
	"net/http"
)

// BusinessError represents a domain business rule violation.
// Port of Java BusinessException.java.
type BusinessError struct {
	StatusCode int
	Message    string
}

func (e *BusinessError) Error() string {
	return e.Message
}

// NewBusinessError creates a BusinessError with 422 Unprocessable Entity.
func NewBusinessError(message string) *BusinessError {
	return &BusinessError{
		StatusCode: http.StatusUnprocessableEntity,
		Message:    message,
	}
}

// Conflict creates a 409 Conflict error.
func Conflict(message string) *BusinessError {
	return &BusinessError{StatusCode: http.StatusConflict, Message: message}
}

// BadRequest creates a 400 Bad Request error.
func BadRequest(message string) *BusinessError {
	return &BusinessError{StatusCode: http.StatusBadRequest, Message: message}
}

// Forbidden creates a 403 Forbidden error.
func Forbidden(message string) *BusinessError {
	return &BusinessError{StatusCode: http.StatusForbidden, Message: message}
}

// TooManyRequests creates a 429 Too Many Requests error (rate/attempt limiting).
func TooManyRequests(message string) *BusinessError {
	return &BusinessError{StatusCode: http.StatusTooManyRequests, Message: message}
}

// NotFoundError represents a resource not found.
// Port of Java ResourceNotFoundException.java.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// NotFound creates a NotFoundError with a message.
func NotFound(message string) *NotFoundError {
	return &NotFoundError{Message: message}
}

// NotFoundResource creates a NotFoundError for a resource with ID.
func NotFoundResource(resource string, id any) *NotFoundError {
	return &NotFoundError{Message: fmt.Sprintf("%s not found with id: %v", resource, id)}
}
