package service

import (
	"time"

	"deploy-service/internal/domain"
)

type billingSnapshot struct {
	UserID          string
	Email           string
	BalanceRUB      float64
	SpentThisMonth  float64
	RefundRUB       float64
	ExemptFromGuard bool
}

type billingState struct {
	Snapshot                    billingSnapshot
	Projects                    []domain.Project
	PendingRUB                  float64
	AvailableRUB                float64
	GracePeriodEndsAt           *time.Time
	GracePeriodRemainingSeconds int64
}

type billingDecision struct {
	clearGrace            bool
	clearDeletionSchedule bool
	startGrace            bool
	graceDeadline         *time.Time
	suspendProjects       []domain.Project
	deleteProjects        []domain.Project
}
