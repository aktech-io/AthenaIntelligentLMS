package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/mobile/auth", func(r chi.Router) {
		// Public endpoints
		r.Post("/otp/send", h.SendOTP)
		r.Post("/otp/verify", h.VerifyOTP)
		r.Post("/token/refresh", h.RefreshToken)

		// Authenticated endpoints
		r.Group(func(r chi.Router) {
			r.Use(authMw)
			r.Post("/pin/setup", h.SetupPIN)
			r.Post("/pin/verify", h.VerifyPIN)
			r.Post("/device/register", h.RegisterDevice)
		})
	})
}

func (h *AuthHandler) SendOTP(w http.ResponseWriter, r *http.Request) {
	var req service.SendOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.SendOTP(r.Context(), req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req service.VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.VerifyOTP(r.Context(), req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) SetupPIN(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.PinSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.SetupPIN(r.Context(), userID, req); err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "PIN set up successfully"})
}

func (h *AuthHandler) VerifyPIN(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.PinVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.VerifyPIN(r.Context(), userID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req service.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.RefreshToken(r.Context(), req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.DeviceRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.RegisterDevice(r.Context(), userID, tenantID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func resolveUserID(r *http.Request) (uuid.UUID, error) {
	id := auth.MobileUserIDFromContext(r.Context())
	if id == "" {
		return uuid.Nil, apperrors.BadRequest("user ID not found in context")
	}
	return uuid.Parse(id)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
