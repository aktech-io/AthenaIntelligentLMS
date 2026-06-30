package httputil

import (
	"encoding/json"
	"net/http"
	"time"
)

// ErrorResponse matches the Spring Boot error JSON shape.
type ErrorResponse struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Path      string `json:"path,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteErrorJSON writes a Spring Boot-compatible error JSON response.
func WriteErrorJSON(w http.ResponseWriter, status int, errorType, message, path string) {
	resp := ErrorResponse{
		Timestamp: time.Now().Format(time.RFC3339),
		Status:    status,
		Error:     errorType,
		Message:   message,
		Path:      path,
	}
	// Try to get requestId from response header (set by logging middleware)
	if reqID := w.Header().Get("X-Request-ID"); reqID != "" {
		resp.RequestID = reqID
	}
	WriteJSON(w, status, resp)
}

// WriteNotFound writes a 404 error response.
func WriteNotFound(w http.ResponseWriter, message, path string) {
	WriteErrorJSON(w, http.StatusNotFound, "Not Found", message, path)
}

// WriteBadRequest writes a 400 error response.
func WriteBadRequest(w http.ResponseWriter, message, path string) {
	WriteErrorJSON(w, http.StatusBadRequest, "Bad Request", message, path)
}

// WriteInternalError writes a 500 error response.
func WriteInternalError(w http.ResponseWriter, message, path string) {
	WriteErrorJSON(w, http.StatusInternalServerError, "Internal Server Error", message, path)
}

// WriteConflict writes a 409 error response.
func WriteConflict(w http.ResponseWriter, message, path string) {
	WriteErrorJSON(w, http.StatusConflict, "Conflict", message, path)
}

// WriteForbidden writes a 403 error response.
func WriteForbidden(w http.ResponseWriter, message, path string) {
	WriteErrorJSON(w, http.StatusForbidden, "Forbidden", message, path)
}

// WriteUnprocessable writes a 422 error response.
func WriteUnprocessable(w http.ResponseWriter, message, path string) {
	WriteErrorJSON(w, http.StatusUnprocessableEntity, "Unprocessable Entity", message, path)
}

// WriteTooManyRequests writes a 429 error response. Callers should set a
// Retry-After header before calling this when a retry window is known.
func WriteTooManyRequests(w http.ResponseWriter, message, path string) {
	WriteErrorJSON(w, http.StatusTooManyRequests, "Too Many Requests", message, path)
}
