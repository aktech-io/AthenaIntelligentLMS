package errors

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func do(t *testing.T, err error) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/thing", nil)
	rec := httptest.NewRecorder()
	HandleError(rec, req, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec, body
}

func TestHandleErrorNotFound(t *testing.T) {
	rec, body := do(t, NotFoundResource("Biller", "abc"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "Biller not found with id: abc", body["message"])
	assert.Equal(t, "Not Found", body["error"])
	assert.Equal(t, float64(404), body["status"])
}

func TestHandleErrorBusiness(t *testing.T) {
	rec, body := do(t, BadRequest("phoneNumber is required"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "phoneNumber is required", body["message"])
}

func TestHandleErrorUnprocessable(t *testing.T) {
	rec, _ := do(t, NewBusinessError("insufficient balance"))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandleErrorUnknownIs500(t *testing.T) {
	rec, body := do(t, errors.New("boom"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	// Internal detail must not leak to the client.
	assert.Equal(t, "internal server error", body["message"])
}

func TestWriteErrorShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/auth/otp/send", nil)
	rec := httptest.NewRecorder()
	WriteError(rec, req, http.StatusUnauthorized, "invalid user")
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "Unauthorized", body["error"])
	assert.Equal(t, "invalid user", body["message"])
	assert.NotEmpty(t, body["timestamp"])
}
