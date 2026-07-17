// OnboardingHandler exposes the self-service eKYC onboarding surface to the
// Flutter app (Nemo A2). All three routes are PUBLIC (pre-auth): the applicant
// has no account yet, so there is no JWT to require — the BFF authenticates
// itself to compliance-service/media-service with the internal service key.
//
// TODO(rate-limit): bff-gateway does not yet wire the common in-memory rate
// limiter (internal/common/middleware/ratelimit.go — currently only used by
// lms-api-gateway). These pre-auth routes are the BFF's most abusable surface;
// when the limiter is wired into bff-gateway main, apply a strict per-IP
// bucket to this route group (like the gateway's loginLimiter).
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

// maxOnboardingUploadBytes caps a single document/selfie upload (media-service
// itself parses at most 10 MiB of multipart data).
const maxOnboardingUploadBytes = 12 << 20

type OnboardingHandler struct {
	svc *service.OnboardingService
}

func NewOnboardingHandler(svc *service.OnboardingService) *OnboardingHandler {
	return &OnboardingHandler{svc: svc}
}

func (h *OnboardingHandler) Routes(r chi.Router) {
	r.Route("/api/v1/mobile/onboarding", func(r chi.Router) {
		r.Post("/", h.Submit)
		r.Get("/{id}", h.Get)
		r.Post("/media", h.UploadMedia)
	})
}

// Submit handles POST /api/v1/mobile/onboarding — proxies to the compliance
// A2 API and returns the application plus a nextStep hint
// (PROCEED_TO_REGISTRATION | AWAIT_REVIEW | CONTACT_SUPPORT).
func (h *OnboardingHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var req service.SubmitOnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.svc.Submit(r.Context(), req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// Get handles GET /api/v1/mobile/onboarding/{id}?tenantId=... — status
// polling while an application sits in the officer referral queue.
func (h *OnboardingHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid application id")
		return
	}

	resp, err := h.svc.Get(r.Context(), id, r.URL.Query().Get("tenantId"))
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// UploadMedia handles POST /api/v1/mobile/onboarding/media — multipart form
// with fields: file (required), mediaType (ID_FRONT|ID_BACK|PASSPORT|SELFIE|
// PROOF_OF_ADDRESS, required), tenantId (optional). The file is streamed on to
// media-service; the returned mediaRef is used as documentRef/selfieRef on the
// subsequent submission.
func (h *OnboardingHandler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxOnboardingUploadBytes)
	// 1 MiB memory threshold — larger parts spill to temp files, keeping the
	// BFF from buffering photos in RAM.
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid multipart form (max upload 12MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "file part is required")
		return
	}
	defer file.Close()

	resp, err := h.svc.UploadMedia(
		r.Context(),
		r.FormValue("tenantId"),
		r.FormValue("mediaType"),
		header.Filename,
		header.Header.Get("Content-Type"),
		file,
	)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}
