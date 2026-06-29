package rbac

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/httputil"
)

// Handler serves the RBAC matrix management API. Reads are open to any
// authenticated caller; writes must be gated by the caller (RequirePermission
// "rbac.manage") at route registration, and are recorded in the tamper-evident
// audit trail.
type Handler struct {
	store  *Store
	audit  *audit.Logger
	logger *zap.Logger
}

// NewHandler creates an RBAC management handler.
func NewHandler(store *Store, auditor *audit.Logger, logger *zap.Logger) *Handler {
	return &Handler{store: store, audit: auditor, logger: logger}
}

// RegisterRoutes mounts the RBAC routes. Apply the rbac.manage gate to the
// write route via the gate middleware passed in (kept here so the caller owns
// the auth wiring).
func (h *Handler) RegisterRoutes(r chi.Router, writeGate func(http.Handler) http.Handler) {
	r.Route("/api/v1/rbac", func(r chi.Router) {
		r.Get("/matrix", h.getMatrix)
		r.Get("/permissions", h.listPermissions)
		r.With(writeGate).Put("/roles/{role}", h.setRolePermissions)
	})
}

func (h *Handler) getMatrix(w http.ResponseWriter, r *http.Request) {
	m, err := h.store.Matrix(r.Context())
	if err != nil {
		h.logger.Error("rbac: get matrix", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to load RBAC matrix", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, m)
}

func (h *Handler) listPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.store.ListPermissions(r.Context())
	if err != nil {
		h.logger.Error("rbac: list permissions", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to load permissions", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"permissions": perms})
}

type setRolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

func (h *Handler) setRolePermissions(w http.ResponseWriter, r *http.Request) {
	role := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "role")))
	if role == "" {
		httputil.WriteBadRequest(w, "role is required", r.URL.Path)
		return
	}
	// Guard against locking everyone out: ADMIN must keep rbac.manage.
	var req setRolePermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	if role == "ADMIN" && !contains(req.Permissions, "rbac.manage") {
		httputil.WriteBadRequest(w, "ADMIN must retain the rbac.manage permission (refusing to lock out RBAC administration)", r.URL.Path)
		return
	}

	// Capture before-state for the audit trail.
	before, err := h.store.Matrix(r.Context())
	if err != nil {
		h.logger.Error("rbac: load before-state", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update role permissions", r.URL.Path)
		return
	}
	beforePerms := append([]string(nil), before.Roles[role]...)
	sort.Strings(beforePerms)

	newPerms := dedupeSorted(req.Permissions)
	version, err := h.store.SetRolePermissions(r.Context(), role, newPerms)
	if err != nil {
		// Unknown permission key etc. is a client error.
		if strings.HasPrefix(err.Error(), "unknown permission key") {
			httputil.WriteBadRequest(w, err.Error(), r.URL.Path)
			return
		}
		h.logger.Error("rbac: set role permissions", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update role permissions", r.URL.Path)
		return
	}

	h.audit.Record(r.Context(), "RBAC_ROLE_PERMISSIONS_UPDATE", "RBAC_ROLE", role,
		map[string]any{"permissions": beforePerms},
		map[string]any{"permissions": newPerms},
		map[string]any{"version": version})

	m, err := h.store.Matrix(r.Context())
	if err != nil {
		// The change succeeded; just echo minimal info.
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"role": role, "permissions": newPerms, "version": version})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, m)
}

func contains(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}

func dedupeSorted(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
