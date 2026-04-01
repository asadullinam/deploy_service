package app

import (
	"context"
	"deploy-service/internal/crypto"
	"deploy-service/internal/domain"
	apihttp "deploy-service/internal/http"
	"deploy-service/internal/integration/github"
	"deploy-service/internal/integration/kubernetes"
	integrationlogs "deploy-service/internal/integration/logs"
	integrationtelegram "deploy-service/internal/integration/telegram"
	"deploy-service/internal/integration/yookassa"
	"deploy-service/internal/monetization"
	"deploy-service/internal/service"
	"deploy-service/internal/store/memory"
	"deploy-service/internal/store/postgres"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func (a *Application) Close() {
	if a.pool != nil {
		a.pool.Close()
	}
}

func New(ctx context.Context) (*Application, error) {
	address := strings.TrimSpace(os.Getenv("HTTP_ADDRESS"))
	if address == "" {
		address = ":8080"
	}

	config := Config{Address: address}

	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if jwtSecret == "" {
		jwtSecret = "dev-secret-change-in-production"
		log.Println("WARNING: JWT_SECRET not set, using insecure default")
	}
	jwtTTL := 24 * time.Hour
	if ttlStr := strings.TrimSpace(os.Getenv("JWT_TTL")); ttlStr != "" {
		if d, err := time.ParseDuration(ttlStr); err == nil {
			jwtTTL = d
		}
	}

	webhookSecret := strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	telegramWebhookSecret := strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET"))

	store, releaseStore, pool, err := buildStore(ctx)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	var userStore service.UserStore
	var usageStore service.UsageStore
	var txStore service.BillingTransactionStore
	var stageStore service.StageStore
	if pool != nil {
		userStore = postgres.NewUserStore(pool)
		usageStore = postgres.NewUsageStore(pool)
		txStore = postgres.NewBillingTransactionStore(pool)
		stageStore = postgres.NewStageStore(pool)
	} else {
		userStore = memory.NewUserStore()
		usageStore = memory.NewUsageStore()
		txStore = memory.NewBillingTransactionStore()
		stageStore = memory.NewStageStore()
	}

	defaultBalanceRUB := 0.0
	if balanceStr := strings.TrimSpace(os.Getenv("DEFAULT_USER_BALANCE_RUB")); balanceStr != "" {
		if value, err := strconv.ParseFloat(balanceStr, 64); err != nil {
			log.Printf("WARNING: DEFAULT_USER_BALANCE_RUB=%q is invalid, using 0", balanceStr)
			defaultBalanceRUB = 0
		} else {
			defaultBalanceRUB = value
		}
	}
	authSvc := service.NewAuthService(userStore, txStore, jwtSecret, jwtTTL, defaultBalanceRUB)
	if ykClient := yookassa.NewClientFromEnvironment(); ykClient != nil {
		var ykStore service.YooKassaPaymentStore
		if pool != nil {
			ykStore = postgres.NewYooKassaPaymentStore(pool)
		} else {
			ykStore = memory.NewYooKassaPaymentStore()
		}
		returnURL := strings.TrimSpace(os.Getenv("YOOKASSA_RETURN_URL"))
		authSvc.SetYooKassa(ykClient, ykStore, returnURL)
		log.Println("YooKassa payment gateway configured")
	}

	provisioner := kubernetes.NewProvisionerFromEnvironment()
	githubAutomation := github.NewAutomationFromEnvironment()
	appsBaseDomain := strings.TrimSpace(os.Getenv("APPS_BASE_DOMAIN"))
	appsPublicScheme := strings.TrimSpace(os.Getenv("APPS_PUBLIC_SCHEME"))
	appsPublicPort := strings.TrimSpace(os.Getenv("APPS_PUBLIC_PORT"))
	if appsBaseDomain == "" {
		log.Println("WARNING: APPS_BASE_DOMAIN is not set; public URL for LoadBalancer deployments will not be shown in project UI")
	}

	var monetizationEngine service.MonetizationEngine
	if pool != nil {
		monetizationEngine = monetization.NewPostgresEngine(usageStore, monetization.TariffFromEnvironment())
	} else {
		monetizationEngine = monetization.NewEngineMock()
	}

	cryptoSvc := buildCryptoService()

	projectService := service.NewProjectService(store, releaseStore, stageStore, provisioner, githubAutomation, monetizationEngine, userStore, txStore, cryptoSvc, appsBaseDomain, appsPublicScheme, appsPublicPort)
	projectService.SetBillingGuardGracePeriod(billingGuardGracePeriod())
	projectService.SetBillingGuardRetentionPeriod(billingGuardRetentionPeriod())
	if logReader := integrationlogs.NewLokiReaderFromEnvironment(); logReader != nil {
		projectService.SetLogReader(logReader)
	}
	telegramClient := integrationtelegram.NewClientFromEnvironment()
	var telegramSender service.TelegramSender
	if telegramClient != nil {
		telegramSender = telegramClient
	}
	notificationService := service.NewNotificationService(userStore, telegramSender, telegramBotUsername(telegramClient))
	notificationService.SetAdminTelegramUsernames(systemTelegramAdminUsernames())
	projectService.SetNotificationService(notificationService)
	handler := apihttp.NewHandlerWithNotifications(projectService, authSvc, notificationService, webhookSecret)
	handler.SetTelegramWebhookSecret(telegramWebhookSecret)
	router := apihttp.NewRouter(handler, jwtSecret)

	app := &Application{
		Config: config,
		Router: router,
		pool:   pool,
	}

	if pool != nil {
		go runMetricsAggregator(ctx, store, userStore, usageStore, txStore, monetizationEngine, kubernetes.NewMetricsCollectorFromEnvironment(metricsCollectorInterval()), notificationService)
	}
	go runBillingGuard(ctx, projectService, billingGuardInterval(), notificationService)

	return app, nil
}

// buildCryptoService читает ENCRYPTION_KEY (64 hex-символа = 32 байта) из переменных окружения.
// В режиме разработки при ошибке переключается на нулевой ключ и пишет предупреждение.
func buildCryptoService() *crypto.Service {
	keyHex := strings.TrimSpace(os.Getenv("ENCRYPTION_KEY"))
	if keyHex == "" {
		log.Println("WARNING: ENCRYPTION_KEY not set, using insecure zero key")
		svc, _ := crypto.NewService(make([]byte, 32))
		return svc
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		log.Println("WARNING: ENCRYPTION_KEY is invalid (must be 64 hex chars), using insecure zero key")
		svc, _ := crypto.NewService(make([]byte, 32))
		return svc
	}
	svc, err := crypto.NewService(key)
	if err != nil {
		log.Printf("WARNING: failed to create crypto service: %v, using insecure zero key", err)
		svc, _ = crypto.NewService(make([]byte, 32))
	}
	return svc
}

// buildStore возвращает ProjectStore и ReleaseStore на базе PostgreSQL, если задан DATABASE_URL,
// иначе используется in-memory-хранилище.
func buildStore(ctx context.Context) (service.ProjectStore, service.ReleaseStore, *pgxpool.Pool, error) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		log.Println("DATABASE_URL not set, using in-memory store")
		return memory.NewProjectStore(), memory.NewReleaseStore(), nil, nil
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, nil, fmt.Errorf("ping database: %w", err)
	}

	if err := postgres.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, nil, fmt.Errorf("run migrations: %w", err)
	}

	log.Println("connected to PostgreSQL, migrations applied")
	return postgres.NewProjectStore(pool), postgres.NewReleaseStore(pool), pool, nil
}

func runMetricsAggregator(ctx context.Context, projectStore service.ProjectStore, userStore service.UserStore, usageStore service.UsageStore, txStore service.BillingTransactionStore, monetizationEngine service.MonetizationEngine, collector service.MetricsCollector, notifications *service.NotificationService) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("metrics aggregator panicked: %v", recovered)
			if notifications != nil {
				_ = notifications.SendSystemAlert(ctx, "system:metrics-aggregator-panic", fmt.Sprintf("[critical] Сбой системного воркера metrics aggregator\npanic: %v", recovered), 15*time.Minute)
			}
		}
	}()
	interval := metricsCollectorInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			collectAndStoreUsage(ctx, t.Add(-interval), t, projectStore, userStore, usageStore, txStore, monetizationEngine, collector, notifications)
		}
	}
}

func metricsCollectorInterval() time.Duration {
	interval := 5 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("METRICS_COLLECTOR_INTERVAL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			return parsed
		}
		log.Printf("WARNING: METRICS_COLLECTOR_INTERVAL=%q is invalid, using %s", raw, interval)
	}
	return interval
}

func billingGuardInterval() time.Duration {
	interval := 1 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("BILLING_GUARD_INTERVAL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			return parsed
		}
		log.Printf("WARNING: BILLING_GUARD_INTERVAL=%q is invalid, using %s", raw, interval)
	}
	return interval
}

func systemTelegramAdminUsernames() []string {
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_ADMIN_USERNAMES"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func billingGuardGracePeriod() time.Duration {
	gracePeriod := 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv("BILLING_GUARD_GRACE_PERIOD")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err == nil && parsed >= 0 {
			return parsed
		}
		log.Printf("WARNING: BILLING_GUARD_GRACE_PERIOD=%q is invalid, using %s", raw, gracePeriod)
	}
	return gracePeriod
}

func billingGuardRetentionPeriod() time.Duration {
	retentionPeriod := 7 * 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv("BILLING_GUARD_RETENTION_PERIOD")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err == nil && parsed >= 0 {
			return parsed
		}
		log.Printf("WARNING: BILLING_GUARD_RETENTION_PERIOD=%q is invalid, using %s", raw, retentionPeriod)
	}
	return retentionPeriod
}

func runBillingGuard(ctx context.Context, enforcer billingGuardEnforcer, interval time.Duration, notifications *service.NotificationService) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("billing guard panicked: %v", recovered)
			if notifications != nil {
				_ = notifications.SendSystemAlert(ctx, "system:billing-guard-panic", fmt.Sprintf("[critical] Сбой системного воркера billing guard\npanic: %v", recovered), 15*time.Minute)
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := enforcer.EnforceAllBillingGuards(ctx); err != nil {
				log.Printf("billing guard enforcement failed: %v", err)
				if notifications != nil {
					_ = notifications.SendSystemAlert(ctx, "system:billing-guard-failed", fmt.Sprintf("[warning] Системный billing guard завершился с ошибкой\n%v", err), 10*time.Minute)
				}
			}
		}
	}
}

func collectAndStoreUsage(ctx context.Context, periodStart, periodEnd time.Time, projectStore service.ProjectStore, userStore service.UserStore, usageStore service.UsageStore, txStore service.BillingTransactionStore, monetizationEngine service.MonetizationEngine, collector service.MetricsCollector, notifications *service.NotificationService) {
	projects := projectStore.List(ctx)
	periodHours := periodEnd.Sub(periodStart).Hours()
	for _, p := range projects {
		if p.Status != domain.ProjectStatusActive {
			continue
		}
		snapshot, err := collector.CollectProjectUsage(ctx, p.ID)
		if err != nil {
			log.Printf("collect usage for project %s: %v", p.ID, err)
			continue
		}
		usage := domain.ResourceUsage{
			ID:                         domain.NewID(),
			ProjectID:                  p.ID,
			PeriodStart:                periodStart,
			PeriodEnd:                  periodEnd,
			CPUCores:                   snapshot.CPUCores,
			MemoryGB:                   snapshot.MemoryGB,
			StorageGB:                  snapshot.StorageGB,
			EgressGB:                   snapshot.EgressGBDelta,
			ReplicaCount:               snapshot.ReplicaCount,
			PodUptimeHours:             snapshot.PodUptimeHours,
			CPUCoreHours:               snapshot.CPUCores * periodHours,
			MemoryGBHours:              snapshot.MemoryGB * periodHours,
			DedicatedLoadBalancerHours: dedicatedLoadBalancerHoursForUsage(p, periodHours),
			RecordedAt:                 periodEnd,
		}
		if err := usageStore.Record(ctx, usage); err != nil {
			log.Printf("record usage for project %s: %v", p.ID, err)
			continue
		}
		cost := monetizationEngine.ComputeUsageCost(usage)
		if cost <= 0 {
			continue
		}
		user, exists := userStore.GetByID(ctx, p.OwnerID)
		if !exists {
			log.Printf("user %s not found for project %s charge", p.OwnerID, p.ID)
			continue
		}
		newBalance := user.BalanceRUB - cost
		if err := userStore.UpdateBalance(ctx, user.ID, newBalance); err != nil {
			log.Printf("update balance for user %s: %v", user.ID, err)
			continue
		}
		tx := domain.BillingTransaction{
			ID:          domain.NewID(),
			UserID:      user.ID,
			ProjectID:   p.ID,
			Type:        domain.TransactionTypeCharge,
			AmountRUB:   -cost,
			Description: fmt.Sprintf("resource usage charge for project %s", p.ID),
			CreatedAt:   periodEnd,
		}
		if err := txStore.Record(ctx, tx); err != nil {
			log.Printf("record charge transaction for project %s: %v", p.ID, err)
		}
		notifyResourcePressure(ctx, notifications, p, snapshot)
	}
}

func telegramBotUsername(client *integrationtelegram.Client) string {
	if client == nil {
		return ""
	}
	return client.BotUsername()
}

func notifyResourcePressure(ctx context.Context, notifications *service.NotificationService, project domain.Project, snapshot domain.ResourceSnapshot) {
	if notifications == nil {
		return
	}
	cpuLimit, memoryLimit := projectResourceLimits(project.ResourceProfile, project.ReplicaCount)
	if cpuLimit > 0 {
		ratio := snapshot.CPUCores / cpuLimit
		switch {
		case ratio >= 0.95:
			_ = notifications.SendUserAlert(ctx, project.OwnerID, "cpu-critical:"+project.ID, fmt.Sprintf("[critical] Проект %s почти уперся в CPU\nТекущее использование: %.2f cores из %.2f cores.", project.Name, snapshot.CPUCores, cpuLimit), 2*time.Hour)
		case ratio >= 0.80:
			_ = notifications.SendUserAlert(ctx, project.OwnerID, "cpu-warning:"+project.ID, fmt.Sprintf("[warning] Проект %s близок к лимиту CPU\nТекущее использование: %.2f cores из %.2f cores.", project.Name, snapshot.CPUCores, cpuLimit), 6*time.Hour)
		}
	}
	if memoryLimit > 0 {
		ratio := snapshot.MemoryGB / memoryLimit
		switch {
		case ratio >= 0.95:
			_ = notifications.SendUserAlert(ctx, project.OwnerID, "memory-critical:"+project.ID, fmt.Sprintf("[critical] Проект %s почти уперся в память\nТекущее использование: %.2f GB из %.2f GB.", project.Name, snapshot.MemoryGB, memoryLimit), 2*time.Hour)
		case ratio >= 0.80:
			_ = notifications.SendUserAlert(ctx, project.OwnerID, "memory-warning:"+project.ID, fmt.Sprintf("[warning] Проект %s близок к лимиту памяти\nТекущее использование: %.2f GB из %.2f GB.", project.Name, snapshot.MemoryGB, memoryLimit), 6*time.Hour)
		}
	}
}

func projectResourceLimits(profile string, replicas int) (float64, float64) {
	count := replicas
	if count <= 0 {
		count = 1
	}
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "starter":
		return 0.3 * float64(count), 0.25 * float64(count)
	case "performance":
		return 1.0 * float64(count), 1.0 * float64(count)
	default:
		return 0.5 * float64(count), 0.5 * float64(count)
	}
}

func dedicatedLoadBalancerHoursForUsage(project domain.Project, periodHours float64) float64 {
	if !project.DedicatedLoadBalancer || !strings.EqualFold(strings.TrimSpace(project.ServiceType), "LoadBalancer") {
		return 0
	}
	return periodHours
}
