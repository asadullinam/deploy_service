package service

import (
	"sync"
	"time"

	"deploy-service/internal/domain"
)

type ProjectService struct {
	store          ProjectStore
	releaseStore   ReleaseStore
	stageStore     StageStore
	provisioner    Provisioner
	logReader      LogReader
	automation     GitHubAutomation
	monetization   MonetizationEngine
	users          UserStore
	txStore        BillingTransactionStore
	crypto         CryptoService
	notifications  *NotificationService
	appsBaseDomain string
	appsURLScheme  string
	appsURLPort    string

	billingGuardMu         sync.Mutex
	pendingCharges         map[string]float64
	graceStartedAt         map[string]time.Time
	billingGracePeriod     time.Duration
	billingRetentionPeriod time.Duration

	perfMu        sync.RWMutex
	costCache     map[string]cachedCostBreakdown
	releasesCache map[string]cachedReleases
	urlsCache     map[string]cachedProjectURLs
}

type cachedCostBreakdown struct {
	value     domain.CostBreakdown
	expiresAt time.Time
}

type cachedReleases struct {
	value     []domain.Release
	expiresAt time.Time
}

type cachedProjectURLs struct {
	value     domain.ProjectURLsResponse
	expiresAt time.Time
}
