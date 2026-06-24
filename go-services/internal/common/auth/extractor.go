package auth

import (
	"context"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// init registers the real context extractor used by httputil.ServiceClient to
// propagate the tenant and user on internal service-to-service calls. Without
// this, ServiceClient falls back to the no-op default and every internal call
// is scoped to the "default" tenant (causing cross-service 404s / skipped
// validation). Every service imports this package for its auth middleware, so
// the registration runs in all of them.
func init() {
	httputil.ContextExtractor = func(ctx context.Context) (string, string) {
		return TenantIDFromContext(ctx), UserIDFromContext(ctx)
	}
}
