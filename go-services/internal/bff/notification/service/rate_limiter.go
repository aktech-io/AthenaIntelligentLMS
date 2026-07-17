package service

import (
	"context"
	"time"

	"github.com/athena-lms/go-services/internal/bff/notification/model"
	"github.com/athena-lms/go-services/internal/bff/notification/repository"
)

const (
	maxMessagesPerWindow = 10
	windowDuration       = 5 * time.Minute
)

type RateLimiter struct {
	repo *repository.RateLimitRepo
}

func NewRateLimiter(repo *repository.RateLimitRepo) *RateLimiter {
	return &RateLimiter{repo: repo}
}

// IsRateLimited checks if the phone number has exceeded the SMS rate limit.
// Returns true if rate limited. Also increments the counter.
func (s *RateLimiter) IsRateLimited(ctx context.Context, phone string) (bool, error) {
	rl, err := s.repo.FindByPhone(ctx, phone)
	if err != nil {
		return false, err
	}

	now := time.Now()

	if rl == nil {
		// First message for this phone number.
		rl = &model.SmsRateLimit{
			PhoneNumber:  phone,
			MessageCount: 1,
			WindowStart:  now,
		}
		return false, s.repo.Upsert(ctx, rl)
	}

	// Window expired — reset.
	if now.After(rl.WindowStart.Add(windowDuration)) {
		rl.MessageCount = 1
		rl.WindowStart = now
		return false, s.repo.Upsert(ctx, rl)
	}

	// Within window.
	if rl.MessageCount >= maxMessagesPerWindow {
		return true, nil
	}

	rl.MessageCount++
	return false, s.repo.Upsert(ctx, rl)
}
