// HTTP error mapping, promoted from the wallet BFF's shared/errors library
// during the A1 Phase 0 fold-in so handlers can map domain errors to JSON
// responses with one call instead of a per-handler type switch.
package errors

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// WriteError writes a standard error JSON response for the given status.
// The "error" field is the canonical status text (e.g. "Bad Request").
func WriteError(w http.ResponseWriter, r *http.Request, status int, message string) {
	httputil.WriteErrorJSON(w, status, http.StatusText(status), message, r.URL.Path)
}

// HandleError maps known domain error types to HTTP responses:
// *NotFoundError -> 404, *BusinessError -> its StatusCode, anything else ->
// 500 with a generic message (and the real error logged).
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *NotFoundError:
		WriteError(w, r, http.StatusNotFound, e.Message)
	case *BusinessError:
		WriteError(w, r, e.StatusCode, e.Message)
	default:
		zap.L().Error("unhandled error", zap.Error(err), zap.String("path", r.URL.Path))
		WriteError(w, r, http.StatusInternalServerError, "internal server error")
	}
}
