package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"freelance/apps/api/internal/acquisition"
	"freelance/apps/api/internal/admin"
	"freelance/apps/api/internal/ai"
	"freelance/apps/api/internal/analytics"
	"freelance/apps/api/internal/auth"
	"freelance/apps/api/internal/blog"
	"freelance/apps/api/internal/catalog"
	"freelance/apps/api/internal/communication"
	"freelance/apps/api/internal/favorites"
	"freelance/apps/api/internal/finance"
	"freelance/apps/api/internal/growth"
	"freelance/apps/api/internal/jobs"
	"freelance/apps/api/internal/matching"
	"freelance/apps/api/internal/media"
	"freelance/apps/api/internal/monetization"
	"freelance/apps/api/internal/notifications"
	"freelance/apps/api/internal/payments"
	"freelance/apps/api/internal/platform/objectstorage"
	"freelance/apps/api/internal/platform/ratelimit"
	"freelance/apps/api/internal/platform/requestmeta"
	platformruntime "freelance/apps/api/internal/platform/runtime"
	"freelance/apps/api/internal/portfolio"
	"freelance/apps/api/internal/profiles"
	"freelance/apps/api/internal/projects"
	"freelance/apps/api/internal/proposals"
	"freelance/apps/api/internal/reputation"
	"freelance/apps/api/internal/reviews"
	"freelance/apps/api/internal/safedeal"
	"freelance/apps/api/internal/services"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

type debugSandboxStatusProvider struct{ DB *sql.DB }

func (p debugSandboxStatusProvider) GetStatus(ctx context.Context, providerOperationID string) (payments.Status, string, error) {
	if p.DB == nil || strings.TrimSpace(providerOperationID) == "" {
		return payments.StatusUnknownReconciliation, "", payments.ErrProviderUnavailable
	}
	var raw string
	if err := p.DB.QueryRowContext(ctx, `SELECT provider_status FROM payment_records WHERE provider='sandbox' AND provider_payment_id=$1 ORDER BY updated_at DESC LIMIT 1`, providerOperationID).Scan(&raw); err != nil {
		return payments.StatusUnknownReconciliation, "", err
	}
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "FUNDED", "RELEASED":
		return payments.StatusSucceeded, raw, nil
	case "CANCELED":
		return payments.StatusCanceled, raw, nil
	case "REFUNDED":
		return payments.StatusRefunded, raw, nil
	case "FAILED":
		return payments.StatusFailed, raw, nil
	case "PENDING", "RELEASE_PENDING", "REFUND_PENDING", "CANCEL_PENDING":
		return payments.StatusProcessing, raw, nil
	default:
		return payments.StatusUnknownReconciliation, raw, nil
	}
}

func main() {
	runtimeConfig, err := platformruntime.Load(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	appEnv := runtimeConfig.Environment
	rateConfig, err := ratelimit.LoadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	aiConfig, err := ai.LoadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	aiBaselines, err := ai.LoadBaselines(os.Getenv("AI_ESTIMATE_BASELINES_JSON"))
	if err != nil {
		log.Fatal(err)
	}
	matchingConfig, err := matching.LoadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	var database *sql.DB
	var sessionRepository auth.SessionRepository
	var readinessChecks []func(context.Context) error
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		database, err = sql.Open("pgx", databaseURL)
		if err != nil {
			log.Fatal(err)
		}
		configurePool(database, os.Getenv)
		defer database.Close()
		sessionRepository = auth.PostgresSessionRepository{DB: database}
		readinessChecks = append(readinessChecks, database.PingContext)
	}

	var limiter ratelimit.Limiter
	var redisClient *redis.Client
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		redisOptions, parseErr := redis.ParseURL(redisURL)
		if parseErr != nil {
			log.Fatal("invalid REDIS_URL")
		}
		redisClient = redis.NewClient(redisOptions)
		defer redisClient.Close()
		limiter = ratelimit.NewRedis(redisClient, rateConfig, "freelance:rate")
		readinessChecks = append(readinessChecks, func(ctx context.Context) error { return redisClient.Ping(ctx).Err() })
	} else {
		limiter = ratelimit.NewMemory(rateConfig, nil)
	}
	rateMiddleware := ratelimit.Middleware{Limiter: limiter, FailOpen: false}
	sessionMiddleware := auth.SessionMiddleware{Repository: sessionRepository, CookieName: os.Getenv("SESSION_COOKIE_NAME")}
	adminCookieName := os.Getenv("ADMIN_SESSION_COOKIE_NAME")
	if adminCookieName == "" {
		base := os.Getenv("SESSION_COOKIE_NAME")
		if base == "" {
			base = "session"
		}
		adminCookieName = base + "_admin"
	}
	adminSessionMiddleware := auth.SessionMiddleware{Repository: sessionRepository, CookieName: adminCookieName}
	uploadSessionMiddleware := auth.UploadSessionMiddleware{Repository: sessionRepository, UserCookie: os.Getenv("SESSION_COOKIE_NAME"), AdminCookie: adminCookieName}
	authHandler := auth.Handler{DB: database, CookieName: os.Getenv("SESSION_COOKIE_NAME"), AdminCookieName: adminCookieName, Secure: appEnv == "production"}
	presenceHandler := auth.PresenceHandler{DB: database}
	protectWrite := func(handler http.Handler) http.Handler {
		limited := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			class := ratelimit.WriteStandard
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				class = ratelimit.PrivateRead
			}
			rateMiddleware.Limit(class, privateNoStore(handler)).ServeHTTP(w, r)
		})
		return sessionMiddleware.RequireSession(limited)
	}
	protectUpload := func(handler http.Handler) http.Handler {
		return uploadSessionMiddleware.RequireSession(rateMiddleware.Limit(ratelimit.Upload, privateNoStore(handler)))
	}
	protectProposal := func(handler http.Handler) http.Handler {
		return sessionMiddleware.RequireSession(rateMiddleware.Limit(ratelimit.ProposalSend, privateNoStore(handler)))
	}
	staffOnly := func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := auth.ActorID(r.Context())
			if !ok || actor == "" {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if database == nil {
				http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
				return
			}
			var privileged bool
			if err := database.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_roles WHERE user_id=$1 AND role IN ('MODERATOR','ADMIN','SUPER_ADMIN'))`, actor).Scan(&privileged); err != nil {
				http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
				return
			}
			if !privileged {
				http.NotFound(w, r)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
	protectAdmin := func(handler http.Handler) http.Handler {
		return adminSessionMiddleware.RequireSession(rateMiddleware.Limit(ratelimit.Admin, staffOnly(privateNoStore(handler))))
	}

	mux := http.NewServeMux()
	publicBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
	localIP := strings.TrimSpace(os.Getenv("LOCAL_IP_ADDRESS"))
	if localIP == "" {
		localIP = strings.TrimSpace(os.Getenv("local_ip_address"))
	}
	if appEnv != "production" && localIP != "" && localIP != "0.0.0.0" {
		publicBaseURL = "http://" + localIP + ":8088"
	}
	if publicBaseURL == "" {
		publicBaseURL = "http://localhost:8088"
	}

	// Payment adapters are constructed only from deployment secrets. Durable
	// routing stores no credentials and can select only adapters present here.
	paymentStatuses := map[payments.ProviderName]payments.StatusProvider{}
	paymentPurchases := map[payments.ProviderName]payments.PurchaseProvider{}
	paymentRecurring := map[payments.ProviderName]payments.RecurringProvider{}
	paymentWebhooks := map[payments.ProviderName]payments.WebhookVerifier{}
	paymentConfigured := map[payments.ProviderName]bool{}
	var yooKassa *payments.YooKassa
	var tbankNominal *payments.TBankNominal
	if p, e := payments.NewYooKassa(payments.YooKassaConfig{ShopID: os.Getenv("YOOKASSA_SHOP_ID"), SecretKey: os.Getenv("YOOKASSA_SECRET_KEY"), BaseURL: os.Getenv("YOOKASSA_BASE_URL")}); e == nil {
		yooKassa = p
		paymentStatuses[payments.ProviderYooKassa] = p
		paymentPurchases[payments.ProviderYooKassa] = p
		paymentRecurring[payments.ProviderYooKassa] = p
		paymentWebhooks[payments.ProviderYooKassa] = p
		paymentConfigured[payments.ProviderYooKassa] = true
	}
	if p, e := payments.NewTBank(payments.TBankConfig{TerminalKey: os.Getenv("TBANK_TERMINAL_KEY"), Password: os.Getenv("TBANK_PASSWORD"), BaseURL: os.Getenv("TBANK_BASE_URL")}); e == nil {
		paymentStatuses[payments.ProviderTBank] = p
		paymentPurchases[payments.ProviderTBank] = p
		paymentRecurring[payments.ProviderTBank] = p
		paymentWebhooks[payments.ProviderTBank] = p
		paymentConfigured[payments.ProviderTBank] = true
	}
	if token, account := strings.TrimSpace(os.Getenv("TBANK_NOMINAL_BEARER_TOKEN")), strings.TrimSpace(os.Getenv("TBANK_NOMINAL_ACCOUNT_NUMBER")); token != "" || account != "" {
		p, e := payments.NewTBankNominal(payments.TBankNominalConfig{
			BearerToken: token, AccountNumber: account, BaseURL: os.Getenv("TBANK_NOMINAL_BASE_URL"),
			ClientCertFile: os.Getenv("TBANK_NOMINAL_CLIENT_CERT_FILE"), ClientKeyFile: os.Getenv("TBANK_NOMINAL_CLIENT_KEY_FILE"),
		})
		if e != nil {
			log.Printf("tbank nominal adapter disabled: %v", e)
		} else {
			tbankNominal = p
		}
	}
	if p, e := payments.NewCloudPayments(payments.CloudPaymentsConfig{PublicID: os.Getenv("CLOUDPAYMENTS_PUBLIC_ID"), APISecret: os.Getenv("CLOUDPAYMENTS_API_SECRET"), BaseURL: os.Getenv("CLOUDPAYMENTS_BASE_URL")}); e == nil {
		paymentStatuses[payments.ProviderCloudPayments] = p
		paymentPurchases[payments.ProviderCloudPayments] = p
		paymentRecurring[payments.ProviderCloudPayments] = p
		paymentWebhooks[payments.ProviderCloudPayments] = p
		paymentConfigured[payments.ProviderCloudPayments] = true
	}
	if p, e := payments.NewYandexPay(payments.YandexPayConfig{JWKSURL: os.Getenv("YANDEX_PAY_JWKS_URL"), APIKey: os.Getenv("YANDEX_PAY_API_KEY"), MerchantID: os.Getenv("YANDEX_PAY_MERCHANT_ID"), BaseURL: os.Getenv("YANDEX_PAY_BASE_URL")}); e == nil && os.Getenv("YANDEX_PAY_API_KEY") != "" && os.Getenv("YANDEX_PAY_MERCHANT_ID") != "" {
		paymentStatuses[payments.ProviderYandexPay] = p
		paymentPurchases[payments.ProviderYandexPay] = p
		paymentRecurring[payments.ProviderYandexPay] = p
		paymentWebhooks[payments.ProviderYandexPay] = p
		paymentConfigured[payments.ProviderYandexPay] = true
	}
	if login, pass1, pass2 := os.Getenv("ROBOKASSA_LOGIN"), os.Getenv("ROBOKASSA_PASSWORD1"), os.Getenv("ROBOKASSA_PASSWORD2"); login != "" && pass1 != "" && pass2 != "" {
		p := &payments.Robokassa{Login: login, Password1: pass1, Password2: pass2, Password3: os.Getenv("ROBOKASSA_PASSWORD3"), BaseURL: os.Getenv("ROBOKASSA_BASE_URL"), ServicesBaseURL: os.Getenv("ROBOKASSA_SERVICES_BASE_URL"), Test: appEnv != "production"}
		paymentStatuses[payments.ProviderRobokassa] = p
		paymentPurchases[payments.ProviderRobokassa] = p
		paymentRecurring[payments.ProviderRobokassa] = p
		paymentWebhooks[payments.ProviderRobokassa] = p
		paymentConfigured[payments.ProviderRobokassa] = true
	}
	adminHandler := admin.Handler{Service: admin.Service{Repository: admin.PostgresRepository{DB: database}}, ActorID: auth.ActorID}
	financeHandler := finance.Handler{Service: finance.Service{Repository: finance.PostgresRepository{DB: database}}, ActorID: auth.ActorID}
	var catalogRepository catalog.Repository = &catalog.Store{}
	if database != nil {
		catalogRepository = catalog.PostgresRepository{DB: database}
	}
	catalogHandler := catalog.Handler{Repository: catalogRepository}
	var profileRepository profiles.Repository = &profiles.Store{Items: map[string]profiles.Profile{}}
	if database != nil {
		profileRepository = profiles.PostgresRepository{DB: database}
	}
	profileHandler := profiles.Handler{Repository: profileRepository}
	var portfolioRepository portfolio.Repository = &portfolio.Store{Items: map[string]portfolio.Item{}, Media: map[string]portfolio.MediaObject{}}
	if database != nil {
		portfolioRepository = portfolio.PostgresRepository{DB: database}
	}
	if database != nil {
		portfolioRepository = portfolio.LimitedRepository{Repository: portfolioRepository, Policy: monetization.LimitPolicy{Service: monetization.Service{Repository: monetization.PostgresRepository{DB: database}}}}
	}
	portfolioHandler := portfolio.Handler{Repository: portfolioRepository}
	publicPortfolioHandler := rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(portfolioHandler.PublicList))
	var mediaRepository media.Repository = &media.Store{Objects: map[string]media.Object{}}
	if database != nil {
		mediaRepository = media.PostgresRepository{DB: database}
	}
	bucket := os.Getenv("OBJECT_STORAGE_BUCKET")
	if bucket == "" {
		bucket = "local-media"
	}
	diskStorage := &media.DiskStorage{RootDir: envOr("DEV_MEDIA_ROOT", "/var/lib/naimio-media"), BaseURL: "/api/v1/dev-storage"}
	developmentMediaStorage := diskStorage

	envS3 := objectstorage.S3Config{
		Endpoint:     os.Getenv("OBJECT_STORAGE_ENDPOINT"),
		Region:       os.Getenv("OBJECT_STORAGE_REGION"),
		Bucket:       os.Getenv("OBJECT_STORAGE_BUCKET"),
		AccessKey:    os.Getenv("OBJECT_STORAGE_ACCESS_KEY"),
		SecretKey:    os.Getenv("OBJECT_STORAGE_SECRET_KEY"),
		SessionToken: os.Getenv("OBJECT_STORAGE_SESSION_TOKEN"),
	}

	storageMasterKey := strings.TrimSpace(os.Getenv("STORAGE_MASTER_KEY"))
	if storageMasterKey == "" {
		storageMasterKey = strings.TrimSpace(os.Getenv("PAYMENT_CONFIG_MASTER_KEY"))
	}
	if storageMasterKey == "" {
		storageMasterKey = strings.TrimSpace(os.Getenv("SAFE_DEAL_WEBHOOK_SECRET"))
	}

	storageManager, err := media.NewStorageManager(database, storageMasterKey, diskStorage, envS3)
	if err != nil {
		log.Fatal(err)
	}
	if database != nil {
		if err := storageManager.LoadFromDB(context.Background()); err != nil {
			log.Printf("failed to load storage settings from DB: %v", err)
		}
	}

	mediaStorage := storageManager
	blogRepository := blog.PostgresRepository{DB: database}
	mediaService := media.Service{Repository: mediaRepository, Storage: mediaStorage, Resolver: storageManager, Bucket: bucket, AutoClean: appEnv != "production"}
	if database != nil {
		mediaService.PurposeAuthorizer = blogRepository
	}
	authHandler.AvatarDeleter = mediaService
	mediaHandler := media.Handler{Service: mediaService, Database: database}
	blogHandler := blog.Handler{Service: blog.Service{Repository: blogRepository}, ActorID: auth.ActorID, Storage: mediaStorage}
	monetizationRepository := monetization.PostgresRepository{DB: database}
	monetizationService := monetization.Service{Repository: monetizationRepository}
	paymentEnvironment := payments.EnvironmentSandbox
	if appEnv == "production" {
		paymentEnvironment = payments.EnvironmentProduction
	}
	paymentRegistry := payments.DefaultRegistry()
	paymentRepository := payments.PostgresRepository{DB: database}
	paymentService := payments.Service{Repository: paymentRepository}
	legacyProviderSet := payments.ProviderSet{Purchases: paymentPurchases, Recurring: paymentRecurring, Webhooks: paymentWebhooks, Statuses: paymentStatuses}
	providerRuntime := payments.NewProviderRuntimeFromSet(legacyProviderSet, yooKassa, tbankNominal, paymentConfigured)
	var providerConfigStore *payments.ProviderConfigStore
	if database != nil {
		masterKey := strings.TrimSpace(os.Getenv("PAYMENT_CONFIG_MASTER_KEY"))
		if masterKey == "" {
			masterKey = strings.TrimSpace(os.Getenv("SAFE_DEAL_WEBHOOK_SECRET"))
		}
		if masterKey == "" && appEnv != "production" {
			masterKey = "development-payment-config-key-change-me"
		}
		var configErr error
		providerConfigStore, configErr = payments.NewProviderConfigStore(database, masterKey)
		if configErr != nil {
			if appEnv == "production" {
				log.Fatalf("payment provider admin configuration: %v", configErr)
			}
			log.Printf("payment provider admin configuration disabled: %v", configErr)
		} else {
			providerRuntime.AttachStore(providerConfigStore)
			if err := providerRuntime.LoadAll(context.Background(), providerConfigStore); err != nil {
				log.Fatalf("payment provider stored configuration: %v", err)
			}
		}
	}
	paymentRoutingRepository := payments.PostgresRoutingRepository{DB: database, ConfiguredProviders: paymentConfigured, IsProviderConfigured: providerRuntime.IsConfigured, ApplicationEnvironment: paymentEnvironment}
	paymentRoutingService := payments.RoutingService{Repository: paymentRoutingRepository, Registry: paymentRegistry, ApplicationEnvironment: paymentEnvironment}
	providerSet := payments.ProviderSet{Runtime: providerRuntime}
	monetizationHandler := monetization.Handler{Service: monetizationService, Provider: monetization.DisabledProvider{}, ActorID: auth.ActorID}
	var billingService *monetization.BillingService
	if database != nil {
		billingService = &monetization.BillingService{Repository: monetizationRepository, Payments: paymentService, Routing: paymentRoutingService, Providers: providerSet, PublicBaseURL: publicBaseURL}
		monetizationHandler.Billing = billingService
		monetizationHandler.ProviderConnected = anyProviderConfigured(providerRuntime)
	}
	paymentRoutingHandler := payments.RoutingHandler{Repository: paymentRoutingRepository, Registry: paymentRegistry, ActorID: auth.ActorID, ConfigStore: providerConfigStore, Runtime: providerRuntime}
	var payoutRecipientHandler *payments.PayoutRecipientHandler
	if database != nil {
		payoutRecipientHandler = &payments.PayoutRecipientHandler{DB: database, Routing: paymentRoutingService, YooKassaShopID: os.Getenv("YOOKASSA_SHOP_ID"), YooKassaShopIDResolver: func(ctx context.Context) string {
			if providerConfigStore != nil {
				if cfg, ok, _ := providerConfigStore.Get(ctx, payments.ProviderYooKassa); ok {
					return cfg.Values["shop_id"]
				}
			}
			return os.Getenv("YOOKASSA_SHOP_ID")
		}, ActorID: auth.ActorID}
	}
	analyticsHandler := analytics.Handler{Service: analytics.Service{
		Repository:   analytics.PostgresRepository{DB: database},
		Entitlements: analytics.EntitlementBridge{Service: monetizationService},
	}}
	serviceStore := &services.Store{Items: map[string]services.Item{}, Categories: map[string]services.Reference{}, Skills: map[string]services.Reference{}, Media: map[string]services.MediaObject{}, Admins: map[string]bool{}}
	var serviceRepository services.Repository = serviceStore
	var serviceSearch services.SearchEngine = serviceStore
	if database != nil {
		postgresServices := services.PostgresRepository{DB: database}
		serviceRepository, serviceSearch = postgresServices, postgresServices
	}
	serviceHandler := services.Handler{Repository: serviceRepository, Search: serviceSearch}
	publicServices := rateMiddleware.Limit(ratelimit.PublicRead, publicCache(300, http.HandlerFunc(serviceHandler.PublicCollection)))
	publicService := rateMiddleware.Limit(ratelimit.PublicRead, publicCache(300, http.HandlerFunc(serviceHandler.PublicItem)))
	projectStore := &projects.Store{Items: map[string]projects.Item{}, Categories: map[string]projects.Reference{}, Skills: map[string]projects.Reference{}, Media: map[string]projects.MediaObject{}}
	var projectRepository projects.Repository = projectStore
	var projectSearch projects.SearchEngine = projectStore
	if database != nil {
		postgresProjects := projects.PostgresRepository{DB: database}
		projectRepository, projectSearch = postgresProjects, postgresProjects
	}
	projectHandler := projects.Handler{Repository: projectRepository, Search: projectSearch}
	publicProjects := rateMiddleware.Limit(ratelimit.PublicRead, publicCache(120, http.HandlerFunc(projectHandler.PublicCollection)))
	publicProject := rateMiddleware.Limit(ratelimit.PublicRead, publicCache(120, http.HandlerFunc(projectHandler.PublicItem)))
	safeDealStore := &safedeal.Store{Deals: map[string]safedeal.Deal{}, Payments: map[string]safedeal.Payment{}, ProviderPayments: map[string]string{}, Disputes: map[string]safedeal.Dispute{}, Operations: map[string][]byte{}, Admins: map[string]bool{}, ProjectStatus: map[string]string{}}
	var safeDealRepository safedeal.Repository = safeDealStore
	if database != nil {
		safeDealRepository = safedeal.PostgresRepository{DB: database}
	}
	var paymentProvider safedeal.PaymentProvider
	var sandboxPaymentProvider *safedeal.SandboxProvider
	newSandboxProvider := func() *safedeal.SandboxProvider {
		webhookSecret := os.Getenv("SAFE_DEAL_WEBHOOK_SECRET")
		if webhookSecret == "" {
			webhookSecret = "development-sandbox-secret-only"
		}
		return safedeal.NewSandboxProvider(webhookSecret)
	}
	if appEnv != "production" {
		// Debug/local development must remain fully testable without merchant
		// credentials. Real provider routing is exercised in production/sandbox
		// smoke tooling; the browser flow uses the local checkout simulator here.
		sandboxPaymentProvider = newSandboxProvider()
		paymentProvider = sandboxPaymentProvider
		if database != nil {
			sandboxPaymentProvider.DB = database
			sandboxPaymentProvider.PaymentAttempts = &paymentService
			paymentStatuses[payments.ProviderName("sandbox")] = sandboxPaymentProvider
		}
	} else if database != nil {
		paymentProvider = safedeal.RoutedProvider{DB: database, Routing: paymentRoutingService, Payments: paymentService, Providers: providerSet, YooKassa: yooKassa, TBankNominal: tbankNominal, Runtime: providerRuntime, PublicBaseURL: publicBaseURL}
	} else {
		paymentProvider = safedeal.DisabledProvider{}
	}
	safeDealService := safedeal.Service{Repository: safeDealRepository, Provider: paymentProvider}
	safeDealHandler := safedeal.Handler{Service: safeDealService}

	var paymentAdminHandler *payments.AdminOperationsHandler
	if database != nil {
		safeTransitioner := safedeal.AttemptTransitioner{DB: database, Repository: safeDealRepository}
		afterPaymentTransition := func(ctx context.Context, attempt payments.Attempt) error {
			switch attempt.Domain {
			case payments.DomainProSubscription:
				if billingService != nil {
					_, err := billingService.ApplyAttempt(ctx, attempt, attempt.ProviderPaymentMethodRef)
					return err
				}
			case payments.DomainSafeDeal:
				return safeTransitioner.Apply(ctx, attempt)
			}
			return nil
		}
		webhookService := payments.WebhookService{Repository: paymentRepository, Verifiers: paymentWebhooks, Providers: providerSet, AfterVerifiedTransition: func(ctx context.Context, attempt payments.Attempt, event payments.VerifiedEvent) error {
			if attempt.Domain == payments.DomainProSubscription && billingService != nil {
				_, err := billingService.ApplyAttempt(ctx, attempt, event.SavedMethodRef)
				return err
			}
			return afterPaymentTransition(ctx, attempt)
		}}
		mux.Handle("/api/v1/payments/providers/", rateMiddleware.Limit(ratelimit.WriteStandard, privateNoStore(payments.ProviderWebhookHandler{Service: webhookService})))
		reconciler := payments.Reconciler{Repository: paymentRepository, Providers: paymentStatuses, ProviderSet: providerSet, Service: paymentService, AfterTransition: afterPaymentTransition}
		paymentAdminHandler = &payments.AdminOperationsHandler{Repository: paymentRepository, Reconciler: reconciler, Providers: providerSet, ActorID: auth.ActorID}
		if token := os.Getenv("PAYMENT_RECONCILIATION_TOKEN"); token != "" {
			mux.Handle("/api/v1/internal/payments/reconcile", payments.ReconciliationHandler{Reconciler: reconciler, Token: token})
		}
		if billingService != nil && os.Getenv("PRO_RENEWAL_TOKEN") != "" {
			mux.Handle("/api/v1/internal/payments/pro-renew", monetization.RenewalHandler{Billing: billingService, Token: os.Getenv("PRO_RENEWAL_TOKEN")})
		}
	}
	proposalStore := &proposals.Store{Items: map[string]proposals.Proposal{}, Projects: map[string]proposals.Project{}, Assignments: map[string]proposals.Assignment{}, DealCreator: safeDealStore}
	var proposalRepository proposals.Repository = proposalStore
	if database != nil {
		proposalRepository = proposals.PostgresRepository{DB: database}
	}
	proposalHandler := proposals.Handler{Repository: proposalRepository}
	favoriteStore := &favorites.Store{Items: map[string]favorites.Item{}}
	var favoriteRepository favorites.Repository = favoriteStore
	if database != nil {
		favoriteRepository = favorites.PostgresRepository{DB: database}
	}
	favoriteHandler := favorites.Handler{Repository: favoriteRepository}
	reputationStore := &reputation.Store{Items: map[string]reputation.Item{}, PublicUsers: map[string]string{}, Challenges: map[string]reputation.Challenge{}, Evidence: map[string]map[string]any{}, Admins: map[string]bool{}}
	var reputationRepository reputation.Repository = reputationStore
	if database != nil {
		reputationRepository = reputation.PostgresRepository{DB: database}
	}
	reputationHandler := reputation.Handler{Service: reputation.Service{Repository: reputationRepository}}
	publicReputation := rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(reputationHandler.Public))
	reviewStore := &reviews.Store{Items: map[string]reviews.Item{}, Projects: map[string]reviews.Relationship{}, Usernames: map[string]string{}, Stats: map[string]reviews.TrustStats{}, Admins: map[string]bool{}, Reports: map[string]bool{}}
	var reviewRepository reviews.Repository = reviewStore
	if database != nil {
		reviewRepository = reviews.PostgresRepository{DB: database}
	}
	reviewHandler := reviews.Handler{Service: reviews.Service{Repository: reviewRepository}}
	publicReviews := rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(reviewHandler.Public))
	communicationStore := &communication.Store{Conversations: map[string]communication.Conversation{}, Memberships: map[string]map[string]communication.Membership{}, Messages: map[string]communication.Message{}, Projects: map[string][]string{}, Users: map[string]bool{}, Media: map[string]communication.Media{}}
	var communicationRepository communication.Repository = communicationStore
	if database != nil {
		communicationRepository = communication.PostgresRepository{DB: database}
	}
	hub := communication.NewHub()
	communicationService := communication.Service{Repository: communicationRepository, Publisher: hub}
	communicationHandler := communication.Handler{Service: communicationService}
	adminSupportHandler := communication.AdminSupportHandler{Service: communicationService}
	if database != nil {
		communicationHandler.Attachments = communication.PostgresAttachmentViewer{DB: database, Storage: mediaStorage}
	}
	realtimeHandler := communication.RealtimeHandler{Service: communicationService, Hub: hub}
	notificationStore := &notifications.Store{Items: map[string]map[string]notifications.Notification{}, Prefs: map[string][]notifications.Preference{}}
	var notificationRepository notifications.Repository = notificationStore
	if database != nil {
		notificationRepository = notifications.PostgresRepository{DB: database}
	}
	adminHandler.Notify = func(ctx context.Context, userID string) {
		if userID == "" {
			return
		}
		page, err := notificationRepository.List(ctx, userID, nil, 1)
		if err != nil || len(page.Items) == 0 {
			if err != nil {
				log.Printf("realtime notification lookup failed user=%s error=%v", userID, err)
			}
			return
		}
		n := page.Items[0]
		_ = hub.Publish(ctx, []string{userID}, communication.Event{Event: "notification.created", Version: 1, ID: n.ID, OccurredAt: n.CreatedAt, Data: n})
	}
	reviewHandler.Notify = adminHandler.Notify
	notificationHandler := notifications.Handler{Service: notifications.Service{Repository: notificationRepository}}
	growthStore := &growth.Store{Invites: map[string]growth.StoredInvite{}, Users: map[string]bool{}, Emails: map[string]string{}, Capabilities: map[string]map[string]bool{}, Projects: map[string]growth.StoreProject{}, Attributions: map[string]growth.Attribution{}, Rewards: map[string]growth.Reward{}, RulesMap: map[string]growth.Rule{}, Admins: map[string]bool{}, TeamMap: map[string]growth.TeamMember{}, Invited: map[string]bool{}}
	var growthRepository growth.Repository = growthStore
	if database != nil {
		growthRepository = growth.PostgresRepository{DB: database}
	}
	growthHandler := growth.Handler{Service: growth.Service{Repository: growthRepository, PublicBaseURL: publicBaseURL}}
	aiStore := &ai.MemoryRepository{Drafts: map[string]ai.Draft{}, TokenHashes: map[[32]byte]string{}}
	var aiRepository interface {
		ai.DraftRepository
		ai.MetricRecorder
		ai.TaxonomyRepository
	} = aiStore
	if database != nil {
		aiRepository = ai.PostgresRepository{DB: database}
	}
	deterministicAI := ai.DeterministicProvider{Taxonomy: aiRepository, Baselines: aiBaselines}
	aiRunner := ai.Runner{Config: aiConfig, Providers: map[string]ai.AIProvider{"deterministic": deterministicAI}, Metrics: aiRepository}
	aiService := ai.Service{Runner: aiRunner, Fallback: deterministicAI, Drafts: aiRepository, Taxonomy: aiRepository}
	aiHandler := ai.Handler{Service: aiService}
	matchingStore := &matching.Store{Projects: map[string]matching.Project{}, Candidates: map[string][]matching.Candidate{}, Runs: map[string]matching.Run{}, LatestRun: map[string]string{}, Manual: map[string]map[string]bool{}, Admins: map[string]bool{}, Events: map[string]bool{}}
	var matchingRepository matching.Repository = matchingStore
	if database != nil {
		matchingRepository = matching.PostgresRepository{DB: database}
	}
	matchingHandler := matching.Handler{Service: matching.Service{Repository: matchingRepository, Reranker: matching.AIReranker{Service: aiService}, Weights: matchingConfig.Weights, RetrievalLimit: matchingConfig.RetrievalLimit, ShortlistLimit: matchingConfig.ShortlistLimit}}
	acquisitionStore := &acquisition.Store{Items: acquisition.DefaultDefinitions(), Taxonomy: map[string]acquisition.Taxonomy{}}
	var acquisitionRepository acquisition.Repository = acquisitionStore
	if database != nil {
		acquisitionRepository = acquisition.PostgresRepository{DB: database}
	}
	acquisitionHandler := acquisition.Handler{Service: acquisition.Service{Repository: acquisitionRepository, Drafts: aiService}}
	jobStore := &jobs.Store{Companies: map[string]jobs.Company{}, Items: map[string]jobs.Item{}, Applications: map[string]jobs.Application{}, Categories: map[string]jobs.Reference{}, Skills: map[string]jobs.Reference{}, Customers: map[string]bool{}, Applicants: map[string]bool{}, Admins: map[string]bool{}}
	var jobRepository jobs.Repository = jobStore
	if database != nil {
		jobRepository = jobs.PostgresRepository{DB: database}
	}
	jobHandler := jobs.Handler{Repository: jobRepository}
	publicVacancies := rateMiddleware.Limit(ratelimit.PublicRead, publicCache(120, http.HandlerFunc(jobHandler.PublicCollection)))
	publicVacancy := rateMiddleware.Limit(ratelimit.PublicRead, publicCache(120, http.HandlerFunc(jobHandler.PublicItem)))
	mux.HandleFunc("/health/live", health)
	mux.Handle("/health/ready", readiness(readinessChecks...))
	mux.HandleFunc("/api/v1/health", health)
	if appEnv != "production" && database != nil && sandboxPaymentProvider != nil {
		mux.HandleFunc("/api/v1/dev/sandbox/payments/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			paymentID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/dev/sandbox/payments/"), "/")
			if paymentID == "" {
				http.NotFound(w, r)
				return
			}
			var dealID, currency string
			var amount int64
			if err := database.QueryRowContext(r.Context(), `SELECT deal_id::text,currency,amount_kopecks FROM payment_records WHERE provider='sandbox' AND provider_payment_id=$1`, paymentID).Scan(&dealID, &currency, &amount); err != nil {
				http.NotFound(w, r)
				return
			}
			if err := sandboxPaymentProvider.ConfirmPayment(paymentID); err != nil {
				http.Error(w, "sandbox payment unavailable", http.StatusConflict)
				return
			}
			if attempt, ok, lookupErr := paymentRepository.FindByProviderExternalID(r.Context(), payments.ProviderName("sandbox"), paymentID); lookupErr == nil && ok {
				attempt.ProviderRawStatus = "FUNDED"
				if !attempt.Terminal() && attempt.Status != payments.StatusSucceeded {
					if transitionErr := attempt.Transition(payments.StatusSucceeded, time.Now().UTC()); transitionErr == nil {
						_ = paymentRepository.Update(r.Context(), attempt)
					}
				}
			}
			eventID := "sandbox-browser-funding-" + paymentID
			if _, _, err := safeDealRepository.ApplyProviderEvent(r.Context(), safedeal.VerifiedProviderEvent{Provider: "sandbox", ProviderEventID: eventID, ProviderPaymentID: paymentID, Type: "FUNDING_CONFIRMED", State: "FUNDED", Currency: currency, AmountKopecks: amount, Verified: true, OccurredAt: time.Now().UTC(), Payload: map[string]any{"source": "debug_checkout"}}); err != nil && !errors.Is(err, safedeal.ErrInvalidState) {
				http.Error(w, "sandbox payment confirmation failed", http.StatusConflict)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"deal_id": dealID, "status": "FUNDED"}})
		})
	}
	if developmentMediaStorage != nil {
		mux.Handle("/api/v1/dev-storage", developmentMediaStorage)
	}
	mux.Handle("/api/v1/categories", rateMiddleware.Limit(ratelimit.PublicRead, publicCache(300, http.HandlerFunc(catalogHandler.Categories))))
	mux.Handle("/api/v1/categories/", rateMiddleware.Limit(ratelimit.PublicRead, publicCache(300, http.HandlerFunc(catalogHandler.Category))))
	mux.Handle("/api/v1/skills", rateMiddleware.Limit(ratelimit.PublicRead, publicCache(60, http.HandlerFunc(catalogHandler.Skills))))
	mux.Handle("/api/v1/admin/categories", protectAdmin(http.HandlerFunc(catalogHandler.AdminCategories)))
	mux.Handle("/api/v1/admin/categories/", protectAdmin(http.HandlerFunc(catalogHandler.AdminCategory)))
	mux.Handle("/api/v1/admin/skills", protectAdmin(http.HandlerFunc(catalogHandler.AdminSkills)))
	mux.Handle("/api/v1/admin/skills/", protectAdmin(http.HandlerFunc(catalogHandler.AdminSkill)))
	mux.Handle("/api/v1/admin/dashboard", protectAdmin(http.HandlerFunc(adminHandler.Dashboard)))
	mux.Handle("/api/v1/admin/users", protectAdmin(http.HandlerFunc(adminHandler.Users)))
	mux.Handle("/api/v1/admin/users/", protectAdmin(http.HandlerFunc(adminHandler.Users)))
	mux.Handle("/api/v1/admin/feature-flags", protectAdmin(http.HandlerFunc(adminHandler.FeatureFlags)))
	mux.Handle("/api/v1/admin/feature-flags/", protectAdmin(http.HandlerFunc(adminHandler.FeatureFlags)))
	mux.Handle("/api/v1/admin/storage-settings", protectAdmin(adminHandler.StorageSettingsHandler(storageManager)))
	mux.Handle("/api/v1/admin/storage-settings/", protectAdmin(adminHandler.StorageSettingsHandler(storageManager)))
	mux.Handle("/api/v1/site-settings", rateMiddleware.Limit(ratelimit.PublicRead, privateNoStore(http.HandlerFunc(adminHandler.SiteSettings))))
	if database != nil {
		mux.Handle("/api/v1/monetization", rateMiddleware.Limit(ratelimit.PublicRead, privateNoStore(http.HandlerFunc(monetizationHandler.Public))))
		mux.Handle("/api/v1/me/subscription", protectWrite(http.HandlerFunc(monetizationHandler.Mine)))
		mux.Handle("/api/v1/me/pro-billing", protectWrite(http.HandlerFunc(monetizationHandler.BillingRoutes)))
		mux.Handle("/api/v1/me/pro-billing/", protectWrite(http.HandlerFunc(monetizationHandler.BillingRoutes)))
		if payoutRecipientHandler != nil {
			mux.Handle("/api/v1/me/payout-recipient", protectWrite(http.HandlerFunc(payoutRecipientHandler.ServeHTTP)))
		}
		mux.Handle("/api/v1/me/analytics", protectWrite(http.HandlerFunc(analyticsHandler.Mine)))
		mux.Handle("/api/v1/engagement/events", sessionMiddleware.OptionalSession(rateMiddleware.Limit(ratelimit.PublicAI, privateNoStore(http.HandlerFunc(analyticsHandler.Track)))))
		mux.Handle("/api/v1/admin/monetization", protectAdmin(http.HandlerFunc(monetizationHandler.Admin)))
		mux.Handle("/api/v1/admin/monetization/", protectAdmin(http.HandlerFunc(monetizationHandler.Admin)))
		mux.Handle("/api/v1/admin/payment-routing", protectAdmin(http.HandlerFunc(paymentRoutingHandler.ServeHTTP)))
		mux.Handle("/api/v1/admin/payment-routing/", protectAdmin(http.HandlerFunc(paymentRoutingHandler.ServeHTTP)))
		if paymentAdminHandler != nil {
			mux.Handle("/api/v1/admin/payments", protectAdmin(http.HandlerFunc(paymentAdminHandler.ServeHTTP)))
			mux.Handle("/api/v1/admin/payments/", protectAdmin(http.HandlerFunc(paymentAdminHandler.ServeHTTP)))
		}
		mux.Handle("/api/v1/blog", rateMiddleware.Limit(ratelimit.PublicRead, publicCache(120, http.HandlerFunc(blogHandler.Public))))
		mux.Handle("/api/v1/blog/", rateMiddleware.Limit(ratelimit.PublicRead, publicCache(120, http.HandlerFunc(blogHandler.Public))))
		mux.Handle("/api/v1/admin/blog", protectAdmin(http.HandlerFunc(blogHandler.Admin)))
		mux.Handle("/api/v1/admin/blog/", protectAdmin(http.HandlerFunc(blogHandler.Admin)))
	}
	mux.Handle("/api/v1/presence/heartbeat", sessionMiddleware.RequireSession(http.HandlerFunc(presenceHandler.Heartbeat)))
	mux.Handle("/api/v1/presence/batch", rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(presenceHandler.Batch)))
	mux.Handle("/api/v1/presence/", rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(presenceHandler.Public)))
	mux.Handle("/api/v1/admin/reports", protectAdmin(http.HandlerFunc(adminHandler.Reports)))
	mux.Handle("/api/v1/admin/reports/", protectAdmin(http.HandlerFunc(adminHandler.Reports)))
	mux.Handle("/api/v1/admin/fraud-signals", protectAdmin(http.HandlerFunc(adminHandler.Fraud)))
	mux.Handle("/api/v1/admin/fraud-signals/", protectAdmin(http.HandlerFunc(adminHandler.Fraud)))
	mux.Handle("/api/v1/admin/audit", protectAdmin(http.HandlerFunc(adminHandler.Audit)))
	mux.Handle("/api/v1/admin/calculators", protectAdmin(http.HandlerFunc(acquisitionHandler.AdminCalculators)))
	mux.Handle("/api/v1/admin/calculators/", protectAdmin(http.HandlerFunc(acquisitionHandler.AdminCalculators)))
	mux.Handle("/api/v1/admin/projects", protectAdmin(adminHandler.Content("PROJECT")))
	mux.Handle("/api/v1/admin/services", protectAdmin(adminHandler.Content("SERVICE")))
	mux.Handle("/api/v1/admin/vacancies", protectAdmin(adminHandler.Content("VACANCY")))
	mux.Handle("/api/v1/admin/reviews", protectAdmin(http.HandlerFunc(adminHandler.Reviews)))
	mux.Handle("/api/v1/admin/disputes", protectAdmin(http.HandlerFunc(adminHandler.Disputes)))
	mux.Handle("/api/v1/profiles/", publicCache(120, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profilePath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/"), "/")
		profileParts := strings.Split(profilePath, "/")
		if len(profileParts) == 5 && profileParts[1] == "portfolio" && profileParts[3] == "media" {
			if r.Method != http.MethodGet || database == nil {
				http.NotFound(w, r)
				return
			}
			var objectKey string
			err := database.QueryRowContext(r.Context(), `SELECT mo.object_key
FROM users u
JOIN professional_profiles pp ON pp.user_id=u.id AND pp.profile_visibility='PUBLIC'
JOIN portfolio_items pi ON pi.user_id=u.id AND pi.id::text=$2 AND pi.visibility='PUBLIC' AND pi.deleted_at IS NULL
JOIN portfolio_media pm ON pm.portfolio_item_id=pi.id
JOIN media_objects mo ON mo.id=pm.media_object_id AND mo.id::text=$3
WHERE u.username=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL
AND mo.purpose='PORTFOLIO' AND mo.uploaded_at IS NOT NULL AND mo.scan_status='CLEAN' AND mo.deleted_at IS NULL`, profileParts[0], profileParts[2], profileParts[4]).Scan(&objectKey)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			imageURL, _, err := mediaStorage.PresignGet(r.Context(), objectKey, 5*time.Minute)
			if err != nil {
				http.Error(w, "portfolio image unavailable", http.StatusServiceUnavailable)
				return
			}
			http.Redirect(w, r, imageURL, http.StatusFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reviews") {
			publicReviews.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/external-reputations") {
			publicReputation.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/portfolio") {
			publicPortfolioHandler.ServeHTTP(w, r)
			return
		}
		profileHandler.Public(w, r)
	})))
	mux.Handle("/api/v1/freelancers", rateMiddleware.Limit(ratelimit.PublicRead, publicCache(120, http.HandlerFunc(profileHandler.List))))
	mux.Handle("/api/v1/me/professional-profile", protectWrite(http.HandlerFunc(profileHandler.Update)))
	mux.Handle("/api/v1/me/categories", protectWrite(http.HandlerFunc(profileHandler.ReplaceCategories)))
	mux.Handle("/api/v1/me/skills", protectWrite(http.HandlerFunc(profileHandler.ReplaceSkills)))
	mux.Handle("/api/v1/me/languages", protectWrite(http.HandlerFunc(profileHandler.ReplaceLanguages)))
	mux.Handle("/api/v1/me/portfolio", protectWrite(http.HandlerFunc(portfolioHandler.Collection)))
	mux.Handle("/api/v1/me/portfolio/", protectWrite(http.HandlerFunc(portfolioHandler.Item)))
	mux.Handle("/api/v1/uploads/presign", protectUpload(http.HandlerFunc(mediaHandler.Presign)))
	mux.Handle("/api/v1/uploads/", protectUpload(http.HandlerFunc(mediaHandler.Item)))
	mux.Handle("/api/v1/avatars/", rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(mediaHandler.Avatar)))
	mux.Handle("/api/v1/services", publicServices)
	mux.Handle("/api/v1/services/", publicService)
	mux.Handle("/api/v1/me/services", protectWrite(http.HandlerFunc(serviceHandler.OwnerCollection)))
	mux.Handle("/api/v1/me/services/", protectWrite(http.HandlerFunc(serviceHandler.OwnerItem)))
	mux.Handle("/api/v1/admin/services/", protectAdmin(adminHandler.Content("SERVICE")))
	mux.Handle("/api/v1/projects", publicProjects)
	mux.Handle("/api/v1/projects/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/")
		projectParts := strings.Split(projectPath, "/")
		if len(projectParts) == 3 && projectParts[1] == "attachments" {
			if r.Method != http.MethodGet || database == nil {
				http.NotFound(w, r)
				return
			}
			var objectKey string
			err := database.QueryRowContext(r.Context(), `SELECT mo.object_key FROM projects p JOIN project_media pm ON pm.project_id=p.id JOIN media_objects mo ON mo.id=pm.media_object_id WHERE (p.id::text=$1 OR p.slug=$1) AND mo.id::text=$2 AND p.visibility='PUBLIC' AND p.deleted_at IS NULL AND p.status IN('OPEN','MATCHING','AWAITING_FUNDING','IN_PROGRESS','COMPLETED') AND mo.purpose='PROJECT' AND mo.uploaded_at IS NOT NULL AND mo.scan_status='CLEAN' AND mo.deleted_at IS NULL`, projectParts[0], projectParts[2]).Scan(&objectKey)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			downloadURL, _, err := mediaStorage.PresignGet(r.Context(), objectKey, 5*time.Minute)
			if err != nil {
				http.Error(w, "attachment unavailable", http.StatusServiceUnavailable)
				return
			}
			http.Redirect(w, r, downloadURL, http.StatusFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reviews") {
			protectWrite(http.HandlerFunc(reviewHandler.Project)).ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/proposals") {
			protectProposal(http.HandlerFunc(proposalHandler.PublicProject)).ServeHTTP(w, r)
			return
		}
		publicProject.ServeHTTP(w, r)
	}))
	mux.Handle("/api/v1/me/projects", protectWrite(http.HandlerFunc(projectHandler.OwnerCollection)))
	mux.Handle("/api/v1/me/projects/", protectWrite(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/matching-runs") || strings.HasSuffix(r.URL.Path, "/recommendations") || strings.HasSuffix(r.URL.Path, "/matching-events") {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/matching-runs") {
				rateMiddleware.Limit(ratelimit.PublicAI, http.HandlerFunc(matchingHandler.Customer)).ServeHTTP(w, r)
			} else {
				matchingHandler.Customer(w, r)
			}
			return
		}
		if strings.HasSuffix(r.URL.Path, "/repeat") || strings.HasSuffix(r.URL.Path, "/share") {
			growthHandler.ProjectAction(w, r)
			return
		}
		if strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/v1/me/projects/"), "/proposals") {
			proposalHandler.CustomerProject(w, r)
			return
		}
		projectHandler.OwnerItem(w, r)
	})))
	mux.Handle("/api/v1/me/proposals", protectWrite(http.HandlerFunc(proposalHandler.Mine)))
	mux.Handle("/api/v1/me/proposals/", protectWrite(http.HandlerFunc(proposalHandler.MineItem)))
	mux.Handle("/api/v1/me/favorites", protectWrite(http.HandlerFunc(favoriteHandler.Collection)))
	mux.Handle("/api/v1/me/favorites/", protectWrite(http.HandlerFunc(favoriteHandler.Item)))
	mux.Handle("/api/v1/me/external-reputations", protectWrite(http.HandlerFunc(reputationHandler.OwnerCollection)))
	mux.Handle("/api/v1/me/external-reputations/", protectWrite(http.HandlerFunc(reputationHandler.OwnerItem)))
	mux.Handle("/api/v1/me/reviews/given", protectWrite(http.HandlerFunc(reviewHandler.Given)))
	mux.Handle("/api/v1/reviews/", protectWrite(http.HandlerFunc(reviewHandler.Report)))
	mux.Handle("/api/v1/admin/reviews/", protectAdmin(http.HandlerFunc(reviewHandler.Admin)))
	mux.Handle("/api/v1/admin/external-reputations", protectAdmin(http.HandlerFunc(reputationHandler.Admin)))
	mux.Handle("/api/v1/admin/external-reputations/", protectAdmin(http.HandlerFunc(reputationHandler.Admin)))
	mux.Handle("/api/v1/conversations", protectWrite(http.HandlerFunc(communicationHandler.Conversations)))
	mux.Handle("/api/v1/conversations/", sessionMiddleware.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/messages") && r.Method == http.MethodPost {
			rateMiddleware.Limit(ratelimit.ChatSend, http.HandlerFunc(communicationHandler.Conversation)).ServeHTTP(w, r)
			return
		}
		rateMiddleware.Limit(ratelimit.WriteStandard, http.HandlerFunc(communicationHandler.Conversation)).ServeHTTP(w, r)
	})))
	mux.Handle("/api/v1/messages/", protectWrite(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/attachments/") {
			communicationHandler.Attachment(w, r)
			return
		}
		communicationHandler.Message(w, r)
	})))
	mux.Handle("/api/v1/ws", sessionMiddleware.RequireSession(realtimeHandler))
	mux.Handle("/api/v1/notifications", protectWrite(http.HandlerFunc(notificationHandler.Collection)))
	mux.Handle("/api/v1/notifications/", protectWrite(http.HandlerFunc(notificationHandler.Item)))
	mux.Handle("/api/v1/notification-preferences", protectWrite(http.HandlerFunc(notificationHandler.Preferences)))
	mux.Handle("/api/v1/me/invites", protectWrite(http.HandlerFunc(growthHandler.Invites)))
	mux.Handle("/api/v1/invites/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/accept") {
			protectWrite(http.HandlerFunc(growthHandler.PublicInvite)).ServeHTTP(w, r)
			return
		}
		rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(growthHandler.PublicInvite)).ServeHTTP(w, r)
	}))
	mux.Handle("/api/v1/me/referrals", protectWrite(http.HandlerFunc(growthHandler.Referrals)))
	mux.Handle("/api/v1/me/customer-team", protectWrite(http.HandlerFunc(growthHandler.Team)))
	mux.Handle("/api/v1/me/customer-team/", protectWrite(http.HandlerFunc(growthHandler.Team)))
	mux.Handle("/api/v1/me/invited-projects/", protectWrite(http.HandlerFunc(growthHandler.InvitedProject)))
	mux.Handle("/api/v1/admin/referral-rules", protectAdmin(http.HandlerFunc(growthHandler.Rules)))
	mux.Handle("/api/v1/admin/referral-rules/", protectAdmin(http.HandlerFunc(growthHandler.Rules)))
	mux.Handle("/api/v1/admin/projects/", protectAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/recommendations") {
			matchingHandler.Admin(w, r)
			return
		}
		adminHandler.Content("PROJECT").ServeHTTP(w, r)
	})))
	mux.Handle("/api/v1/admin/metrics/ai", protectAdmin(http.HandlerFunc(matchingHandler.AdminMetrics)))
	mux.Handle("/api/v1/me/safe-deals", protectWrite(http.HandlerFunc(safeDealHandler.Mine)))
	mux.Handle("/api/v1/me/safe-deals/", protectWrite(http.HandlerFunc(safeDealHandler.Mine)))
	mux.Handle("/api/v1/admin/safe-deals", protectAdmin(http.HandlerFunc(safeDealHandler.Admin)))
	mux.Handle("/api/v1/admin/safe-deals/", protectAdmin(http.HandlerFunc(safeDealHandler.Admin)))
	mux.Handle("/api/v1/admin/finance/fees", protectAdmin(http.HandlerFunc(financeHandler.Fees)))
	mux.Handle("/api/v1/admin/finance/provider-pricing", protectAdmin(http.HandlerFunc(financeHandler.ProviderPricing)))
	mux.Handle("/api/v1/vacancies", publicVacancies)
	mux.Handle("/api/v1/vacancies/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/applications") {
			protectProposal(http.HandlerFunc(jobHandler.PublicItem)).ServeHTTP(w, r)
			return
		}
		publicVacancy.ServeHTTP(w, r)
	}))
	mux.Handle("/api/v1/me/companies", protectWrite(http.HandlerFunc(jobHandler.Companies)))
	mux.Handle("/api/v1/me/vacancies", protectWrite(http.HandlerFunc(jobHandler.OwnerCollection)))
	mux.Handle("/api/v1/me/vacancies/", protectWrite(http.HandlerFunc(jobHandler.OwnerItem)))
	mux.Handle("/api/v1/me/job-applications", protectWrite(http.HandlerFunc(jobHandler.Mine)))
	mux.Handle("/api/v1/admin/vacancies/", protectAdmin(adminHandler.Content("VACANCY")))
	mux.Handle("/api/v1/calculators/", sessionMiddleware.OptionalSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		class := ratelimit.PublicAI
		if r.Method == http.MethodGet {
			class = ratelimit.PublicRead
		}
		rateMiddleware.Limit(class, http.HandlerFunc(acquisitionHandler.Calculator)).ServeHTTP(w, r)
	})))
	mux.Handle("/api/v1/calculators", rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(acquisitionHandler.Calculator)))
	mux.Handle("/api/v1/acquisition/events", sessionMiddleware.OptionalSession(rateMiddleware.Limit(ratelimit.PublicAI, privateNoStore(http.HandlerFunc(acquisitionHandler.Event)))))
	mux.Handle("/api/v1/seo/sitemap", rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(acquisitionHandler.Sitemap)))
	mux.Handle("/api/v1/project-drafts", sessionMiddleware.OptionalSession(rateMiddleware.Limit(ratelimit.PublicAI, privateNoStore(http.HandlerFunc(aiHandler.DraftCollection)))))
	mux.Handle("/api/v1/project-drafts/", sessionMiddleware.OptionalSession(rateMiddleware.Limit(ratelimit.PublicAI, privateNoStore(http.HandlerFunc(aiHandler.DraftItem)))))
	mux.Handle("/api/v1/ai/", sessionMiddleware.OptionalSession(rateMiddleware.Limit(ratelimit.PublicAI, privateNoStore(http.HandlerFunc(aiHandler.Tool)))))
	mux.Handle("/api/v1/admin/support/ws", protectAdmin(http.HandlerFunc(realtimeHandler.ServeSupportHTTP)))
	mux.Handle("/api/v1/admin/support/messages/", protectAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.Replace(r.URL.Path, "/api/v1/admin/support/messages/", "/api/v1/messages/", 1)
		r = r.WithContext(auth.WithActorID(r.Context(), communication.SupportUserID))
		if strings.Contains(r.URL.Path, "/attachments/") {
			communicationHandler.Attachment(w, r)
			return
		}
		communicationHandler.Message(w, r)
	})))
	mux.Handle("/api/v1/admin/support/uploads/presign", protectAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/v1/uploads/presign"
		r = r.WithContext(auth.WithActorID(r.Context(), communication.SupportUserID))
		mediaHandler.Presign(w, r)
	})))
	mux.Handle("/api/v1/admin/support/uploads/", protectAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.Replace(r.URL.Path, "/api/v1/admin/support/uploads/", "/api/v1/uploads/", 1)
		r = r.WithContext(auth.WithActorID(r.Context(), communication.SupportUserID))
		mediaHandler.Item(w, r)
	})))
	mux.Handle("/api/v1/admin/support/", protectAdmin(adminSupportHandler))
	mux.Handle("/api/v1/auth/register", rateMiddleware.Limit(ratelimit.AuthStrict, http.HandlerFunc(authHandler.Register)))
	mux.Handle("/api/v1/auth/login", rateMiddleware.Limit(ratelimit.AuthStrict, http.HandlerFunc(authHandler.Login)))
	mux.Handle("/api/v1/auth/verify-email", rateMiddleware.Limit(ratelimit.AuthStrict, http.HandlerFunc(authHandler.VerifyEmail)))
	mux.Handle("/api/v1/auth/resend-verification", protectWrite(http.HandlerFunc(authHandler.ResendEmailVerification)))
	mux.Handle("/api/v1/auth/logout", rateMiddleware.Limit(ratelimit.AuthStrict, http.HandlerFunc(authHandler.Logout)))
	mux.Handle("/api/v1/auth/logout-all", sessionMiddleware.RequireSession(rateMiddleware.Limit(ratelimit.AuthStrict, http.HandlerFunc(authHandler.LogoutAll))))
	mux.Handle("/api/v1/auth/session", sessionMiddleware.OptionalSession(rateMiddleware.Limit(ratelimit.PublicRead, http.HandlerFunc(authHandler.Session))))
	mux.Handle("/api/v1/auth/admin-session", adminSessionMiddleware.RequireSession(rateMiddleware.Limit(ratelimit.Admin, http.HandlerFunc(authHandler.Session))))
	mux.Handle("/api/v1/auth/admin-logout", rateMiddleware.Limit(ratelimit.AuthStrict, http.HandlerFunc(authHandler.AdminLogout)))
	mux.Handle("/api/v1/auth/change-password", protectWrite(http.HandlerFunc(authHandler.ChangePassword)))
	mux.Handle("/api/v1/me", protectWrite(http.HandlerFunc(authHandler.Me)))
	mux.Handle("/api/v1/me/capabilities/", protectWrite(http.HandlerFunc(authHandler.Capability)))
	server := &http.Server{Addr: ":8080", Handler: accessLog(requestID(requestmeta.Middleware(mux))), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 75 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			log.Printf("api shutdown failed: %v", shutdownErr)
		}
	}()
	log.Printf("service=api environment=%s event=listening address=:8080", appEnv)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func anyProviderConfigured(runtime *payments.ProviderRuntime) bool {
	if runtime == nil {
		return false
	}
	for _, provider := range []payments.ProviderName{payments.ProviderYooKassa, payments.ProviderTBank, payments.ProviderCloudPayments, payments.ProviderYandexPay, payments.ProviderRobokassa} {
		if runtime.IsConfigured(provider) {
			return true
		}
	}
	return false
}

func configurePool(db *sql.DB, getenv func(string) string) {
	maxOpen := envPositiveInt(getenv("DB_MAX_OPEN_CONNS"), 20)
	maxIdle := envPositiveInt(getenv("DB_MAX_IDLE_CONNS"), 5)
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(envDuration(getenv("DB_CONN_MAX_LIFETIME"), 30*time.Minute))
	db.SetConnMaxIdleTime(envDuration(getenv("DB_CONN_MAX_IDLE_TIME"), 5*time.Minute))
}
func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envPositiveInt(raw string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
func envDuration(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}
func (w *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	w.status = http.StatusSwitchingProtocols
	return h.Hijack()
}
func (w *statusRecorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		entry, _ := json.Marshal(map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "level": "info", "service": "api", "request_id": rec.Header().Get("X-Request-ID"), "method": r.Method, "route": r.URL.Path, "status": rec.status, "duration_ms": time.Since(started).Milliseconds()})
		log.Print(string(entry))
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readiness(checks ...func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		for _, check := range checks {
			if err := check(ctx); err != nil {
				writeError(w, r, http.StatusServiceUnavailable, "NOT_READY", "service is not ready")
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			var b [8]byte
			if _, err := rand.Read(b[:]); err == nil {
				id = "req_" + hex.EncodeToString(b[:])
			} else {
				id = "req_unknown"
			}
		}
		w.Header().Set("X-Request-ID", id)
		r.Header.Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func publicCache(seconds int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, s-maxage=%d, stale-while-revalidate=86400", seconds, seconds))
		w.Header().Add("Vary", "Accept-Encoding")
		next.ServeHTTP(w, r)
	})
}

func privateNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = r.Header.Get("X-Request-ID")
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": id}})
}
