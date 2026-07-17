package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/athena-lms/go-services/internal/bff/billpay/model"
	"github.com/athena-lms/go-services/internal/bff/billpay/repository"
)

// AutoSaveScheduler runs daily at 6 AM to process auto-save goals.
type AutoSaveScheduler struct {
	repo       *repository.SavingsRepo
	savingsSvc *SavingsService
}

func NewAutoSaveScheduler(repo *repository.SavingsRepo, savingsSvc *SavingsService) *AutoSaveScheduler {
	return &AutoSaveScheduler{repo: repo, savingsSvc: savingsSvc}
}

// Start begins the scheduler loop. It blocks until ctx is cancelled.
func (s *AutoSaveScheduler) Start(ctx context.Context) {
	go func() {
		for {
			now := time.Now()
			// Calculate next 6 AM.
			next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			waitDuration := time.Until(next)

			slog.Info("auto-save scheduler: next run", "at", next, "wait", waitDuration)

			select {
			case <-ctx.Done():
				slog.Info("auto-save scheduler stopped")
				return
			case <-time.After(waitDuration):
				s.runAutoSave(ctx)
			}
		}
	}()
}

func (s *AutoSaveScheduler) runAutoSave(ctx context.Context) {
	slog.Info("auto-save scheduler: starting run")

	goals, err := s.repo.FindActiveAutoSaveGoals(ctx)
	if err != nil {
		slog.Error("auto-save: failed to fetch goals", "error", err)
		return
	}

	now := time.Now()
	weekday := now.Weekday()
	dayOfMonth := now.Day()

	var processed, failed int
	for i := range goals {
		goal := &goals[i]
		if !shouldAutoSave(goal, weekday, dayOfMonth) {
			continue
		}

		if err := s.savingsSvc.ProcessAutoSave(ctx, goal); err != nil {
			slog.Error("auto-save failed for goal", "goalId", goal.ID, "error", err)
			failed++
		} else {
			processed++
		}
	}

	slog.Info("auto-save scheduler: run complete", "processed", processed, "failed", failed, "total", len(goals))
}

func shouldAutoSave(goal *model.SavingsGoal, weekday time.Weekday, dayOfMonth int) bool {
	if goal.AutoSaveFrequency == nil {
		return false
	}
	switch *goal.AutoSaveFrequency {
	case model.FrequencyDaily:
		return true
	case model.FrequencyWeekly:
		return weekday == time.Monday
	case model.FrequencyMonthly:
		return dayOfMonth == 1
	default:
		return false
	}
}
