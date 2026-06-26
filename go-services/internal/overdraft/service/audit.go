package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/overdraft/model"
	"github.com/athena-lms/go-services/internal/overdraft/repository"
)

// AuditService writes and reads audit log entries.
type AuditService struct {
	repo   *repository.Repository
	logger *zap.Logger
}

// NewAuditService creates a new AuditService.
func NewAuditService(repo *repository.Repository, logger *zap.Logger) *AuditService {
	return &AuditService{repo: repo, logger: logger}
}

// Audit writes an audit log entry.
func (s *AuditService) Audit(ctx context.Context, tenantID, entityType string, entityID uuid.UUID, action string,
	before, after, metadata map[string]interface{}) {

	actor := auth.UserIDFromContext(ctx)
	if actor == "" {
		actor = "SYSTEM"
	}

	entry := &model.OverdraftAuditLog{
		TenantID:       tenantID,
		EntityType:     entityType,
		EntityID:       entityID,
		Action:         action,
		Actor:          actor,
		BeforeSnapshot: before,
		AfterSnapshot:  after,
		Metadata:       metadata,
	}

	if err := s.repo.CreateAuditLog(ctx, entry); err != nil {
		s.logger.Error("Failed to write audit log",
			zap.String("action", action),
			zap.String("entityType", entityType),
			zap.Error(err))
	}
}

// GetAuditLog returns paginated audit log entries.
func (s *AuditService) GetAuditLog(ctx context.Context, tenantID string, entityType *string, entityID *uuid.UUID,
	page, size int) (dto.PageResponse, error) {

	logs, total, err := s.repo.ListAuditLogs(ctx, tenantID, entityType, entityID, size, page*size)
	if err != nil {
		return dto.PageResponse{}, err
	}

	responses := make([]model.AuditLogResponse, 0, len(logs))
	for _, l := range logs {
		responses = append(responses, model.AuditLogResponse{
			ID:             l.ID,
			TenantID:       l.TenantID,
			EntityType:     l.EntityType,
			EntityID:       l.EntityID,
			Action:         l.Action,
			Actor:          l.Actor,
			BeforeSnapshot: l.BeforeSnapshot,
			AfterSnapshot:  l.AfterSnapshot,
			Metadata:       l.Metadata,
			CreatedAt:      l.CreatedAt,
		})
	}

	return dto.NewPageResponse(responses, page, size, total), nil
}

// VerifyAuditChain reports whether the tamper-evident audit chain is intact.
func (s *AuditService) VerifyAuditChain(ctx context.Context, tenantID string) (*repository.ChainVerification, error) {
	return s.repo.VerifyAuditChain(ctx, tenantID)
}
