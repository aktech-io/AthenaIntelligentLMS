// ComplianceClient fronts the compliance-service A2 self-service onboarding
// API (POST /api/v1/onboarding, GET /api/v1/onboarding/{id}) for the mobile
// BFF. Calls go DIRECTLY to compliance-service with X-Service-Key auth
// (ServiceKeyTransport stamps key + tenant-from-context) — not through the
// public lms-api-gateway, which deliberately strips service-auth headers
// (CRIT-1: service-to-service calls never transit the public ingress).
//
// Unlike the older map-returning clients, downstream failures are mapped to
// typed errors so the handler surfaces them honestly instead of a blanket 500:
//   - network failure / no response  -> 503 Service Unavailable
//   - downstream 5xx                 -> 502 Bad Gateway
//   - downstream 404                 -> NotFoundError (404)
//   - other downstream 4xx           -> BusinessError with the same status
package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type ComplianceClient struct {
	baseURL string
	client  *http.Client
}

func NewComplianceClient(baseURL, serviceKey string) *ComplianceClient {
	return &ComplianceClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

// SubmitOnboarding posts a self-service onboarding application. The tenant is
// taken from ctx by ServiceKeyTransport (X-Service-Tenant); compliance-service
// resolves it server-side, so the body carries no tenant field.
func (c *ComplianceClient) SubmitOnboarding(ctx context.Context, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/onboarding", c.baseURL)
	return doProxyPost(ctx, c.client, "onboarding", url, body)
}

// GetOnboarding fetches an application by id for status polling.
func (c *ComplianceClient) GetOnboarding(ctx context.Context, id string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/onboarding/%s", c.baseURL, id)
	return doProxyGet(ctx, c.client, "onboarding", url)
}
