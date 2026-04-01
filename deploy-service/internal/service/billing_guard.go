package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"deploy-service/internal/domain"
)

func (s *ProjectService) reserveDeployBudget(ctx context.Context, project domain.Project, requestedServiceType string, requestedDedicatedLoadBalancer bool) (func(), error) {
	projects := s.listBillingProjects(ctx, project.OwnerID)
	snapshot, err := s.loadBillingSnapshot(ctx, project.OwnerID, projects)
	if err != nil {
		return nil, err
	}
	if snapshot.ExemptFromGuard {
		return func() {}, nil
	}

	serviceType := normalizedServiceType(requestedServiceType)
	if serviceType == "" {
		serviceType = normalizedServiceType(project.ServiceType)
	}
	requiredReserve := estimateRequiredDeployReserve(
		serviceType,
		project.DedicatedLoadBalancer || requestedDedicatedLoadBalancer,
		snapshot.SpentThisMonth,
		project.ReplicaCount,
		project.ResourceProfile,
	)
	now := time.Now().UTC()

	s.billingGuardMu.Lock()
	pending := s.pendingCharges[project.OwnerID]
	available := snapshot.BalanceRUB - snapshot.SpentThisMonth + snapshot.RefundRUB - pending
	if available < requiredReserve {
		var graceDeadline time.Time
		var hasGrace bool
		if available <= 0 {
			graceDeadline, hasGrace = s.ensureGraceDeadlineLocked(project.OwnerID, now)
		} else {
			graceDeadline, hasGrace = s.peekGraceDeadlineLocked(project.OwnerID)
		}
		s.billingGuardMu.Unlock()
		return nil, buildInsufficientBalanceError(snapshot, pending, available, requiredReserve, graceDeadline, hasGrace, now)
	}

	s.pendingCharges[project.OwnerID] = pending + requiredReserve
	s.billingGuardMu.Unlock()

	released := false
	return func() {
		s.billingGuardMu.Lock()
		defer s.billingGuardMu.Unlock()
		if released {
			return
		}
		current := s.pendingCharges[project.OwnerID] - requiredReserve
		if current <= 1e-9 {
			delete(s.pendingCharges, project.OwnerID)
		} else {
			s.pendingCharges[project.OwnerID] = current
		}
		released = true
	}, nil
}

func buildInsufficientBalanceError(snapshot billingSnapshot, pending, available, requiredReserve float64, graceDeadline time.Time, hasGrace bool, now time.Time) error {
	suffix := ""
	if hasGrace {
		remaining := graceDeadline.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		suffix = fmt.Sprintf(
			", auto-suspend in %s (until %s)",
			remaining.Round(time.Second),
			graceDeadline.Format(time.RFC3339),
		)
	}

	return fmt.Errorf(
		"%w: balance %.2f RUB, spent this month %.2f RUB, refund %.2f RUB, pending %.2f RUB, available %.2f RUB, required reserve %.2f RUB%s",
		domain.ErrInsufficientBalance,
		snapshot.BalanceRUB,
		snapshot.SpentThisMonth,
		snapshot.RefundRUB,
		pending,
		available,
		requiredReserve,
		suffix,
	)
}

func (s *ProjectService) loadBillingState(ctx context.Context, userID string, now time.Time) (billingState, error) {
	projects := s.listBillingProjects(ctx, userID)
	snapshot, err := s.loadBillingSnapshot(ctx, userID, projects)
	if err != nil {
		return billingState{}, err
	}

	var graceEndsAt *time.Time
	var graceRemainingSeconds int64

	s.billingGuardMu.Lock()
	pending := s.pendingCharges[userID]
	available := snapshot.BalanceRUB - snapshot.SpentThisMonth + snapshot.RefundRUB - pending
	if deadline, ok := s.peekGraceDeadlineLocked(userID); ok {
		deadlineCopy := deadline
		graceEndsAt = &deadlineCopy
		if remaining := deadline.Sub(now); remaining > 0 {
			graceRemainingSeconds = int64(remaining.Seconds())
		}
	}
	s.billingGuardMu.Unlock()

	return billingState{
		Snapshot:                    snapshot,
		Projects:                    projects,
		PendingRUB:                  pending,
		AvailableRUB:                available,
		GracePeriodEndsAt:           graceEndsAt,
		GracePeriodRemainingSeconds: graceRemainingSeconds,
	}, nil
}

func (s *ProjectService) listBillingProjects(ctx context.Context, userID string) []domain.Project {
	projects := make([]domain.Project, 0)
	for _, project := range s.store.List(ctx) {
		if project.OwnerID != userID || project.Status == domain.ProjectStatusDeleted {
			continue
		}
		projects = append(projects, project)
	}
	return projects
}

func (s *ProjectService) loadBillingSnapshot(ctx context.Context, userID string, projects []domain.Project) (billingSnapshot, error) {
	user, exists := s.users.GetByID(ctx, userID)
	if !exists {
		return billingSnapshot{}, domain.ErrUserNotFound
	}

	spentThisMonth, err := s.calculateUserSpentThisMonth(ctx, projects)
	if err != nil {
		return billingSnapshot{}, err
	}

	refundRUB, err := s.calculateUserRefunds(ctx, userID)
	if err != nil {
		return billingSnapshot{}, err
	}

	return billingSnapshot{
		UserID:          user.ID,
		Email:           user.Email,
		BalanceRUB:      user.BalanceRUB,
		SpentThisMonth:  spentThisMonth,
		RefundRUB:       refundRUB,
		ExemptFromGuard: strings.EqualFold(strings.TrimSpace(user.Email), "asadullinam@yandex.ru"),
	}, nil
}

func (s *ProjectService) GetBillingSummary(ctx context.Context, userID string) (domain.BillingSummary, error) {
	state, err := s.loadBillingState(ctx, userID, time.Now().UTC())
	if err != nil {
		return domain.BillingSummary{}, err
	}

	return domain.BillingSummary{
		UserID:                      state.Snapshot.UserID,
		Email:                       state.Snapshot.Email,
		BalanceRUB:                  state.Snapshot.BalanceRUB,
		SpentThisMonth:              state.Snapshot.SpentThisMonth,
		RefundRUB:                   state.Snapshot.RefundRUB,
		PendingChargesRUB:           state.PendingRUB,
		AvailableRUB:                state.AvailableRUB,
		ExemptFromGuard:             state.Snapshot.ExemptFromGuard,
		GracePeriodEndsAt:           state.GracePeriodEndsAt,
		GracePeriodRemainingSeconds: state.GracePeriodRemainingSeconds,
	}, nil
}

func (s *ProjectService) EnforceBillingGuard(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	state, err := s.loadBillingState(ctx, userID, now)
	if err != nil {
		return err
	}

	decision := s.evaluateBillingGuard(state, now)
	return s.applyBillingDecision(ctx, state, decision, now)
}

func (s *ProjectService) evaluateBillingGuard(state billingState, now time.Time) billingDecision {
	decision := billingDecision{}
	if state.Snapshot.ExemptFromGuard {
		decision.clearGrace = true
		decision.clearDeletionSchedule = true
		return decision
	}
	if state.AvailableRUB > 0 {
		decision.clearGrace = true
		decision.clearDeletionSchedule = true
		return decision
	}

	if state.GracePeriodEndsAt == nil {
		decision.startGrace = true
		graceDeadline := now.Add(s.billingGracePeriod)
		decision.graceDeadline = &graceDeadline
		return decision
	}
	decision.graceDeadline = state.GracePeriodEndsAt
	if now.Before(*state.GracePeriodEndsAt) {
		return decision
	}

	for _, project := range state.Projects {
		switch project.Status {
		case domain.ProjectStatusActive:
			decision.suspendProjects = append(decision.suspendProjects, project)
		case domain.ProjectStatusSuspended:
			if project.DeletionDueAt != nil && !now.Before(*project.DeletionDueAt) {
				decision.deleteProjects = append(decision.deleteProjects, project)
			}
		}
	}
	return decision
}

func (s *ProjectService) applyBillingDecision(ctx context.Context, state billingState, decision billingDecision, now time.Time) error {
	if decision.clearGrace {
		s.billingGuardMu.Lock()
		delete(s.graceStartedAt, state.Snapshot.UserID)
		s.billingGuardMu.Unlock()
	}
	if decision.clearDeletionSchedule {
		if err := s.clearDeletionSchedule(state.Projects); err != nil {
			return err
		}
	}
	if decision.startGrace {
		s.billingGuardMu.Lock()
		graceDeadline, startedNow := s.ensureGraceDeadlineLocked(state.Snapshot.UserID, now)
		s.billingGuardMu.Unlock()
		if startedNow {
			log.Printf("billing guard: user %s entered grace period until %s", state.Snapshot.UserID, graceDeadline.Format(time.RFC3339))
			s.notifyUser(ctx, state.Snapshot.UserID, "billing-grace:"+state.Snapshot.UserID, fmt.Sprintf("[warning] Баланс почти исчерпан\nДоступный остаток: %.2f ₽.\nЕсли не пополнить баланс до %s, активные проекты будут автоматически приостановлены.", state.AvailableRUB, graceDeadline.Format(time.RFC3339)), 6*time.Hour)
		}
		return nil
	}
	for _, project := range decision.suspendProjects {
		if err := s.suspendProjectForNonPayment(ctx, project, now, s.billingRetentionPeriod); err != nil {
			return fmt.Errorf("auto-suspend project %s for user %s: %w", project.ID, state.Snapshot.UserID, err)
		}
		log.Printf("billing guard: auto-suspended project %s for user %s after grace period", project.ID, state.Snapshot.UserID)
		s.notifyUser(ctx, state.Snapshot.UserID, "billing-suspended:"+project.ID, fmt.Sprintf("[critical] Проект %s приостановлен из-за нехватки средств\nПополнение баланса разблокирует безопасное возобновление.", project.Name), 2*time.Hour)
	}
	for _, project := range decision.deleteProjects {
		if err := s.DeleteProject(ctx, project.ID); err != nil {
			return fmt.Errorf("auto-delete project %s for user %s after retention period: %w", project.ID, state.Snapshot.UserID, err)
		}
		log.Printf("billing guard: auto-deleted project %s for user %s after retention period", project.ID, state.Snapshot.UserID)
		s.notifyUser(ctx, state.Snapshot.UserID, "billing-deleted:"+project.ID, fmt.Sprintf("[critical] Проект %s удален после retention-периода\nЧтобы избежать повторения, пополняй баланс до окончания grace period.", project.Name), 24*time.Hour)
	}
	return nil
}

func (s *ProjectService) EnforceAllBillingGuards(ctx context.Context) error {
	seenUsers := make(map[string]struct{})
	for _, project := range s.store.List(ctx) {
		if project.OwnerID == "" || project.Status == domain.ProjectStatusDeleted {
			continue
		}
		if _, exists := seenUsers[project.OwnerID]; exists {
			continue
		}
		seenUsers[project.OwnerID] = struct{}{}
		if err := s.EnforceBillingGuard(ctx, project.OwnerID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProjectService) calculateUserSpentThisMonth(ctx context.Context, projects []domain.Project) (float64, error) {
	spentThisMonth := 0.0
	for _, candidate := range projects {
		if candidate.Status == domain.ProjectStatusDeleted {
			continue
		}
		cost, err := s.monetization.GetProjectCost(ctx, candidate.ID)
		if err != nil {
			return 0, fmt.Errorf("calculate monthly spend for project %s: %w", candidate.ID, err)
		}
		spentThisMonth += cost.Total
	}
	return spentThisMonth, nil
}

func (s *ProjectService) calculateUserRefunds(ctx context.Context, userID string) (float64, error) {
	txs, err := s.txStore.ListByUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("list billing transactions for user %s: %w", userID, err)
	}
	refundRUB := 0.0
	for _, tx := range txs {
		if tx.Type == domain.TransactionTypeRefund && tx.AmountRUB > 0 {
			refundRUB += tx.AmountRUB
		}
	}
	return refundRUB, nil
}

func (s *ProjectService) peekGraceDeadlineLocked(userID string) (time.Time, bool) {
	startedAt, ok := s.graceStartedAt[userID]
	if !ok {
		return time.Time{}, false
	}
	return startedAt.Add(s.billingGracePeriod), true
}

func (s *ProjectService) ensureGraceDeadlineLocked(userID string, now time.Time) (time.Time, bool) {
	if deadline, ok := s.peekGraceDeadlineLocked(userID); ok {
		return deadline, false
	}
	s.graceStartedAt[userID] = now
	return now.Add(s.billingGracePeriod), true
}

func (s *ProjectService) clearDeletionSchedule(projects []domain.Project) error {
	for _, project := range projects {
		if project.Status != domain.ProjectStatusSuspended {
			continue
		}
		if project.SuspendedAt == nil && project.DeletionDueAt == nil {
			continue
		}
		project.SuspendedAt = nil
		project.DeletionDueAt = nil
		project.UpdatedAt = time.Now().UTC()
		if err := s.store.Update(context.Background(), project); err != nil {
			return fmt.Errorf("clear deletion schedule for project %s: %w", project.ID, err)
		}
	}
	return nil
}

func (s *ProjectService) suspendProjectForNonPayment(ctx context.Context, project domain.Project, now time.Time, retentionPeriod time.Duration) error {
	if project.Status != domain.ProjectStatusActive {
		return fmt.Errorf("can only auto-suspend an active project, current status: %s", project.Status)
	}
	project.Status = domain.ProjectStatusSuspended
	project.UpdatedAt = now
	project.SuspendedAt = &now
	deletionDueAt := now.Add(retentionPeriod)
	project.DeletionDueAt = &deletionDueAt
	if err := s.store.Update(ctx, project); err != nil {
		return err
	}
	return s.provisioner.SuspendProjectEnvironment(ctx, project.ID)
}
