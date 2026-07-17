// Proxy helpers with honest error mapping, used by clients whose responses the
// BFF passes through to the app (onboarding, media). The older doJSON* helpers
// in helpers.go collapse every downstream failure into an opaque error that
// handlers surface as 500; these instead classify failures so the app can tell
// "you sent something invalid" (4xx passthrough) from "the platform is
// degraded" (502/503) — and a dead downstream is NEVER a silent success.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

// mapDownstreamResponse converts a downstream non-2xx response into a typed
// error for apperrors.HandleError:
//
//	5xx        -> 502 Bad Gateway ("<service> service error")
//	404        -> NotFoundError with the downstream message
//	other 4xx  -> BusinessError with the downstream status + message
func mapDownstreamResponse(service string, statusCode int, body []byte) error {
	msg := downstreamMessage(body)
	switch {
	case statusCode >= 500:
		return &apperrors.BusinessError{
			StatusCode: http.StatusBadGateway,
			Message:    fmt.Sprintf("%s service error", service),
		}
	case statusCode == http.StatusNotFound:
		if msg == "" {
			msg = fmt.Sprintf("%s resource not found", service)
		}
		return apperrors.NotFound(msg)
	default:
		if msg == "" {
			msg = fmt.Sprintf("%s service rejected the request", service)
		}
		return &apperrors.BusinessError{StatusCode: statusCode, Message: msg}
	}
}

// unavailableError is the mapping for "no HTTP response at all" (connection
// refused, DNS, timeout): 503 Service Unavailable.
func unavailableError(service string) error {
	return &apperrors.BusinessError{
		StatusCode: http.StatusServiceUnavailable,
		Message:    fmt.Sprintf("%s service is unavailable, please retry", service),
	}
}

// downstreamMessage extracts the "message" field from the platform's standard
// error JSON ({status, error, message, path}); empty when the body is not in
// that shape.
func downstreamMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return ""
	}
	return e.Message
}

// doProxyPost sends a JSON POST and decodes the JSON object response, mapping
// failures per mapDownstreamResponse/unavailableError.
func doProxyPost(ctx context.Context, client *http.Client, service, url string, body any) (map[string]any, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return doProxy(client, service, req)
}

// doProxyGet sends a GET and decodes the JSON object response, mapping
// failures per mapDownstreamResponse/unavailableError.
func doProxyGet(ctx context.Context, client *http.Client, service, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return doProxy(client, service, req)
}

// doProxy executes the request and applies the shared response/error mapping.
func doProxy(client *http.Client, service string, req *http.Request) (map[string]any, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, unavailableError(service)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, mapDownstreamResponse(service, resp.StatusCode, respBody)
	}
	var result map[string]any
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, &apperrors.BusinessError{
				StatusCode: http.StatusBadGateway,
				Message:    fmt.Sprintf("%s service returned an unreadable response", service),
			}
		}
	}
	return result, nil
}
