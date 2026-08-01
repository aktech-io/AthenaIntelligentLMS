package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	"github.com/athena-lms/go-services/internal/bff/gateway/config"
	"github.com/athena-lms/go-services/internal/bff/gateway/model"
	"github.com/athena-lms/go-services/internal/bff/gateway/publisher"
	"github.com/athena-lms/go-services/internal/bff/gateway/repository"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

// Audit actions recorded on the gateway's audit_log (F4 mobile hardening).
const (
	auditPinSetup         = "PIN_SETUP"
	auditPinSetupRejected = "PIN_SETUP_REJECTED"
	auditPinChanged       = "PIN_CHANGED"
	auditPinVerifyFailed  = "PIN_VERIFY_FAILED"
	auditPinLocked        = "PIN_LOCKED"
)

type AuthService struct {
	cfg            *config.Config
	userRepo       *repository.UserRepo
	otpRepo        *repository.OTPRepo
	tokenRepo      *repository.TokenRepo
	deviceRepo     *repository.DeviceRepo
	jwtUtil        *auth.JWTUtil
	notifClient    *client.NotificationClient
	eventPublisher *publisher.EventPublisher
	audit          *audit.Logger
}

func NewAuthService(
	cfg *config.Config,
	userRepo *repository.UserRepo,
	otpRepo *repository.OTPRepo,
	tokenRepo *repository.TokenRepo,
	deviceRepo *repository.DeviceRepo,
	jwtUtil *auth.JWTUtil,
	notifClient *client.NotificationClient,
	eventPublisher *publisher.EventPublisher,
	auditLogger *audit.Logger,
) *AuthService {
	return &AuthService{
		cfg:            cfg,
		userRepo:       userRepo,
		otpRepo:        otpRepo,
		tokenRepo:      tokenRepo,
		deviceRepo:     deviceRepo,
		jwtUtil:        jwtUtil,
		notifClient:    notifClient,
		eventPublisher: eventPublisher,
		audit:          auditLogger,
	}
}

type SendOTPRequest struct {
	PhoneNumber string `json:"phoneNumber"`
	Purpose     string `json:"purpose"`
	TenantID    string `json:"tenantId"`
}

type SendOTPResponse struct {
	Message          string `json:"message"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
	// OTP is echoed ONLY when OTP_ECHO_ENABLED=true (dev/sandbox). In
	// production the flag defaults off and this field is omitted (F4).
	OTP string `json:"otp,omitempty"`
}

func (s *AuthService) SendOTP(ctx context.Context, req SendOTPRequest) (*SendOTPResponse, error) {
	if req.PhoneNumber == "" {
		return nil, apperrors.BadRequest("phoneNumber is required")
	}
	if req.TenantID == "" {
		req.TenantID = "default"
	}

	purpose := model.OTPPurpose(req.Purpose)
	if purpose == "" {
		purpose = model.PurposeLogin
	}

	// Generate OTP
	otp, err := generateOTP(s.cfg.OTPLength)
	if err != nil {
		return nil, fmt.Errorf("generate OTP: %w", err)
	}

	// Hash OTP
	otpHash, err := bcrypt.GenerateFromPassword([]byte(otp), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash OTP: %w", err)
	}

	// Store OTP record
	record := &model.OTPRecord{
		PhoneNumber: req.PhoneNumber,
		OTPHash:     string(otpHash),
		Purpose:     purpose,
		ExpiresAt:   time.Now().Add(s.cfg.OTPExpiry),
		Attempts:    0,
		Verified:    false,
	}
	if err := s.otpRepo.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("store OTP: %w", err)
	}

	// Send OTP via notification service (best effort)
	go func() {
		bgCtx := context.Background()
		if err := s.notifClient.SendOtp(bgCtx, req.PhoneNumber, otp); err != nil {
			slog.Error("failed to send OTP via notification service", "phone", req.PhoneNumber, "error", err)
		}
	}()

	resp := &SendOTPResponse{
		Message:          "OTP sent successfully",
		ExpiresInSeconds: int(s.cfg.OTPExpiry.Seconds()),
	}
	if s.cfg.OTPEchoEnabled {
		resp.OTP = otp // dev/sandbox only — gated by OTP_ECHO_ENABLED
	}
	return resp, nil
}

type VerifyOTPRequest struct {
	PhoneNumber string `json:"phoneNumber"`
	OTP         string `json:"otp"`
	Purpose     string `json:"purpose"`
	TenantID    string `json:"tenantId"`
}

func (s *AuthService) VerifyOTP(ctx context.Context, req VerifyOTPRequest) (*model.AuthResponse, error) {
	if req.PhoneNumber == "" || req.OTP == "" {
		return nil, apperrors.BadRequest("phoneNumber and otp are required")
	}
	if req.TenantID == "" {
		req.TenantID = "default"
	}

	purpose := model.OTPPurpose(req.Purpose)
	if purpose == "" {
		purpose = model.PurposeLogin
	}

	// Find latest OTP record
	record, err := s.otpRepo.FindLatestByPhoneAndPurpose(ctx, req.PhoneNumber, purpose)
	if err != nil {
		return nil, fmt.Errorf("find OTP: %w", err)
	}
	if record == nil {
		return nil, apperrors.BadRequest("no OTP found for this phone number")
	}

	// Check expiry
	if time.Now().After(record.ExpiresAt) {
		return nil, apperrors.BadRequest("OTP has expired")
	}

	// Check attempts
	if record.Attempts >= s.cfg.OTPMaxAttempts {
		return nil, apperrors.BadRequest("maximum OTP attempts exceeded")
	}

	// Increment attempts
	if err := s.otpRepo.IncrementAttempts(ctx, record.ID); err != nil {
		return nil, fmt.Errorf("increment attempts: %w", err)
	}

	// Verify OTP
	if err := bcrypt.CompareHashAndPassword([]byte(record.OTPHash), []byte(req.OTP)); err != nil {
		return nil, apperrors.BadRequest("invalid OTP")
	}

	// Mark verified
	if err := s.otpRepo.MarkVerified(ctx, record.ID); err != nil {
		return nil, fmt.Errorf("mark verified: %w", err)
	}

	// Find or create user
	user, err := s.userRepo.FindByPhoneAndTenant(ctx, req.PhoneNumber, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	if user == nil {
		if purpose != model.PurposeRegistration && purpose != model.PurposeLogin {
			return nil, apperrors.BadRequest("user not found")
		}
		// Create new user
		customerID := generateCustomerID()
		user = &model.MobileUser{
			TenantID:    req.TenantID,
			PhoneNumber: req.PhoneNumber,
			CustomerID:  customerID,
			Status:      model.StatusPendingOTP,
			KYCTier:     0,
		}
		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, fmt.Errorf("create user: %w", err)
		}
		slog.Info("new user registered", "userId", user.ID, "phone", req.PhoneNumber)

		// Publish registration event
		s.eventPublisher.PublishUserRegistered(req.TenantID, map[string]any{
			"userId":      user.ID.String(),
			"phoneNumber": req.PhoneNumber,
			"customerId":  customerID,
		})
	}

	// Check user status
	if user.Status == model.StatusBlocked {
		return nil, apperrors.Forbidden("account is blocked")
	}
	if user.Status == model.StatusSuspended {
		return nil, apperrors.Forbidden("account is suspended")
	}

	// Generate tokens
	return s.generateAuthResponse(ctx, user)
}

type PinSetupRequest struct {
	Pin string `json:"pin"`
}

// SetupPIN sets the user's PIN for the FIRST time only. If a PIN already
// exists the request is rejected (409): re-login must never silently replace
// the PIN — that would let anyone with stolen login credentials bypass it
// (F4). Changing an existing PIN goes through ChangePIN with the current PIN.
func (s *AuthService) SetupPIN(ctx context.Context, userID uuid.UUID, req PinSetupRequest) error {
	if err := validatePinFormat(req.Pin); err != nil {
		return err
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return apperrors.NotFoundResource("User", userID.String())
	}
	if user.PinHash != nil {
		s.audit.Record(ctx, auditPinSetupRejected, "MOBILE_USER", userID.String(), nil, nil,
			map[string]any{"reason": "PIN already set"})
		return apperrors.Conflict("PIN is already set; use pin/change with your current PIN")
	}

	pinHash, err := bcrypt.GenerateFromPassword([]byte(req.Pin), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash PIN: %w", err)
	}

	if err := s.userRepo.UpdatePinHash(ctx, userID, string(pinHash)); err != nil {
		return err
	}
	s.audit.Record(ctx, auditPinSetup, "MOBILE_USER", userID.String(), nil, nil, nil)
	return nil
}

type PinChangeRequest struct {
	CurrentPin string `json:"currentPin"`
	NewPin     string `json:"newPin"`
}

// ChangePIN replaces an existing PIN after verifying the current one (subject
// to the same attempt throttling as every other PIN check).
func (s *AuthService) ChangePIN(ctx context.Context, userID uuid.UUID, req PinChangeRequest) error {
	if err := validatePinFormat(req.NewPin); err != nil {
		return err
	}
	if err := s.VerifyPINForUser(ctx, userID, req.CurrentPin); err != nil {
		return err
	}

	pinHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPin), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash PIN: %w", err)
	}
	if err := s.userRepo.UpdatePinHash(ctx, userID, string(pinHash)); err != nil {
		return err
	}
	s.audit.Record(ctx, auditPinChanged, "MOBILE_USER", userID.String(), nil, nil, nil)
	return nil
}

type PinVerifyRequest struct {
	Pin string `json:"pin"`
}

type PinVerifyResponse struct {
	Valid bool `json:"valid"`
}

func (s *AuthService) VerifyPIN(ctx context.Context, userID uuid.UUID, req PinVerifyRequest) (*PinVerifyResponse, error) {
	err := s.VerifyPINForUser(ctx, userID, req.Pin)
	if err == nil {
		return &PinVerifyResponse{Valid: true}, nil
	}
	// Preserve the pre-F4 response contract for a plain mismatch
	// ({"valid": false}, HTTP 200); throttling and other errors surface as-is.
	if errors.Is(err, errInvalidPin) {
		return &PinVerifyResponse{Valid: false}, nil
	}
	return nil, err
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (s *AuthService) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*model.AuthResponse, error) {
	if req.RefreshToken == "" {
		return nil, apperrors.BadRequest("refreshToken is required")
	}

	// Validate the JWT signature
	claims, err := s.jwtUtil.ValidateToken(req.RefreshToken)
	if err != nil {
		return nil, apperrors.BadRequest("invalid or expired refresh token")
	}

	// Look up user by phone number from token subject
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	user, err := s.userRepo.FindByPhoneAndTenant(ctx, claims.Subject, tenantID)
	if err != nil || user == nil {
		return nil, apperrors.BadRequest("invalid refresh token")
	}

	// Find matching token in DB by comparing bcrypt hashes
	activeTokens, err := s.tokenRepo.FindActiveByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("find tokens: %w", err)
	}

	var matchedToken *model.RefreshToken
	for i := range activeTokens {
		if bcrypt.CompareHashAndPassword([]byte(activeTokens[i].TokenHash), tokenFingerprint(req.RefreshToken)) == nil {
			matchedToken = &activeTokens[i]
			break
		}
	}
	if matchedToken == nil {
		return nil, apperrors.BadRequest("refresh token not found or revoked")
	}

	// Revoke old token
	if err := s.tokenRepo.Revoke(ctx, matchedToken.ID); err != nil {
		return nil, fmt.Errorf("revoke token: %w", err)
	}

	// Generate new tokens
	return s.generateAuthResponse(ctx, user)
}

type DeviceRegisterRequest struct {
	DeviceID           string `json:"deviceId"`
	FCMToken           string `json:"fcmToken"`
	DeviceName         string `json:"deviceName"`
	OSType             string `json:"osType"`
	OSVersion          string `json:"osVersion"`
	BiometricEnabled   bool   `json:"biometricEnabled"`
	BiometricPublicKey string `json:"biometricPublicKey"`
}

func (s *AuthService) RegisterDevice(ctx context.Context, userID uuid.UUID, tenantID string, req DeviceRegisterRequest) (*model.UserDevice, error) {
	if req.DeviceID == "" {
		return nil, apperrors.BadRequest("deviceId is required")
	}

	osType := model.OSType(req.OSType)
	device := &model.UserDevice{
		TenantID:         tenantID,
		UserID:           userID,
		DeviceID:         req.DeviceID,
		FCMToken:         strPtr(req.FCMToken),
		DeviceName:       strPtr(req.DeviceName),
		OSType:           &osType,
		OSVersion:        strPtr(req.OSVersion),
		BiometricEnabled: req.BiometricEnabled,
	}
	if req.BiometricPublicKey != "" {
		device.BiometricPublicKey = strPtr(req.BiometricPublicKey)
	}

	if err := s.deviceRepo.Upsert(ctx, device); err != nil {
		return nil, fmt.Errorf("register device: %w", err)
	}

	// Re-fetch to get full record with timestamps
	result, err := s.deviceRepo.FindByUserAndDevice(ctx, userID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("fetch device: %w", err)
	}
	return result, nil
}

// errInvalidPin is the sentinel for a plain PIN mismatch (distinguishes it
// from lockout/state errors so VerifyPIN can keep its {"valid":false} shape).
var errInvalidPin = apperrors.BadRequest("invalid PIN")

// VerifyPINForUser verifies a user's PIN with server-side attempt throttling
// (F4). It is the single PIN check used by the auth endpoints AND the money
// paths (transfers, loans, overdraft): after PinMaxAttempts consecutive
// failures the PIN locks with exponential backoff, and every failure/lockout
// is audited.
func (s *AuthService) VerifyPINForUser(ctx context.Context, userID uuid.UUID, pin string) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return apperrors.NotFoundResource("User", userID.String())
	}
	if user.PinHash == nil {
		return apperrors.BadRequest("PIN not set up")
	}

	if user.PinLockedUntil != nil && time.Now().Before(*user.PinLockedUntil) {
		retryIn := time.Until(*user.PinLockedUntil).Round(time.Second)
		return apperrors.TooManyRequests(fmt.Sprintf(
			"too many failed PIN attempts; try again in %s", retryIn))
	}

	if bcrypt.CompareHashAndPassword([]byte(*user.PinHash), []byte(pin)) != nil {
		attempts, recErr := s.userRepo.RecordPinFailure(ctx, userID)
		if recErr != nil {
			slog.Error("failed to record PIN failure", "userId", userID, "error", recErr)
			attempts = user.FailedPinAttempts + 1
		}
		if attempts >= s.cfg.PinMaxAttempts {
			lockFor := pinLockoutDuration(attempts, s.cfg.PinMaxAttempts, s.cfg.PinLockoutBase, s.cfg.PinLockoutMax)
			until := time.Now().Add(lockFor)
			if lockErr := s.userRepo.SetPinLock(ctx, userID, until); lockErr != nil {
				slog.Error("failed to set PIN lock", "userId", userID, "error", lockErr)
			}
			s.audit.Record(ctx, auditPinLocked, "MOBILE_USER", userID.String(), nil, nil,
				map[string]any{"failedAttempts": attempts, "lockedForSeconds": int(lockFor.Seconds()), "lockedUntil": until})
			slog.Warn("PIN locked after repeated failures", "userId", userID, "attempts", attempts, "lockedFor", lockFor)
			return apperrors.TooManyRequests(fmt.Sprintf(
				"too many failed PIN attempts; try again in %s", lockFor.Round(time.Second)))
		}
		s.audit.Record(ctx, auditPinVerifyFailed, "MOBILE_USER", userID.String(), nil, nil,
			map[string]any{"failedAttempts": attempts, "remaining": s.cfg.PinMaxAttempts - attempts})
		return errInvalidPin
	}

	// Success: clear any accumulated failures.
	if user.FailedPinAttempts > 0 || user.PinLockedUntil != nil {
		if resetErr := s.userRepo.ResetPinFailures(ctx, userID); resetErr != nil {
			slog.Error("failed to reset PIN failures", "userId", userID, "error", resetErr)
		}
	}
	return nil
}

// pinLockoutDuration computes the exponential-backoff lockout: base at the
// threshold, doubling for each further failure, capped at max.
func pinLockoutDuration(attempts, maxAttempts int, base, max time.Duration) time.Duration {
	over := attempts - maxAttempts
	if over < 0 {
		over = 0
	}
	d := base
	for i := 0; i < over; i++ {
		d *= 2
		if d >= max {
			return max
		}
	}
	if d > max {
		return max
	}
	return d
}

// validatePinFormat enforces the 4-6 digit PIN contract.
func validatePinFormat(pin string) error {
	if len(pin) < 4 || len(pin) > 6 {
		return apperrors.BadRequest("PIN must be 4-6 digits")
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return apperrors.BadRequest("PIN must contain only digits")
		}
	}
	return nil
}

func (s *AuthService) generateAuthResponse(ctx context.Context, user *model.MobileUser) (*model.AuthResponse, error) {
	// Access token
	accessToken, err := s.jwtUtil.GenerateToken(
		user.PhoneNumber,
		[]string{"MOBILE_USER"},
		user.CustomerID,
		user.TenantID,
		user.ID.String(),
		s.cfg.JWTAccessExpiry,
	)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	// Refresh token
	jti := uuid.New().String()
	refreshTokenStr, err := s.jwtUtil.GenerateRefreshToken(user.PhoneNumber, jti, user.TenantID, s.cfg.JWTRefreshExpiry)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Store refresh token hash in DB (SHA-256 first to fit bcrypt 72-byte limit)
	tokenHash, err := bcrypt.GenerateFromPassword(tokenFingerprint(refreshTokenStr), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash refresh token: %w", err)
	}

	refreshRecord := &model.RefreshToken{
		UserID:    user.ID,
		TokenHash: string(tokenHash),
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiry),
		Revoked:   false,
	}
	if err := s.tokenRepo.Create(ctx, refreshRecord); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &model.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		User:         user.ToSummary(),
	}, nil
}

func generateOTP(length int) (string, error) {
	digits := "0123456789"
	result := make([]byte, length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		result[i] = digits[idx.Int64()]
	}
	return string(result), nil
}

func generateCustomerID() string {
	id := uuid.New().String()
	return "MOB-" + strings.ToUpper(strings.ReplaceAll(id[:8], "-", ""))
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// tokenFingerprint produces a short SHA-256 hash of a JWT token string
// so it fits within bcrypt's 72-byte input limit.
func tokenFingerprint(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return []byte(hex.EncodeToString(h[:]))
}
