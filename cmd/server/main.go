package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	delivery "github.com/northfieldzz/kura/internal/delivery/http"
	"github.com/northfieldzz/kura/internal/infrastructure/adapter"
	"github.com/northfieldzz/kura/internal/infrastructure/config"
	"github.com/northfieldzz/kura/internal/infrastructure/dynamodb"
	"github.com/northfieldzz/kura/internal/infrastructure/logger"
	"github.com/northfieldzz/kura/internal/infrastructure/metrics"
	"github.com/northfieldzz/kura/internal/infrastructure/notifier"
	"github.com/northfieldzz/kura/internal/infrastructure/proxy"
	"github.com/northfieldzz/kura/internal/infrastructure/ratelimit"
	"github.com/northfieldzz/kura/internal/infrastructure/scheduler"
	"github.com/northfieldzz/kura/internal/infrastructure/websocket"
	"github.com/northfieldzz/kura/internal/usecase"
)

func main() {
	// 1. 設定のロード
	cfg := config.Load()
	log.Printf("[INFO] Starting Kura on port %s (Region: %s)", cfg.Port, cfg.AWSRegion)

	// 2. インフラ層の初期化
	// 非同期構造化ロガー (CloudWatch Logs 向け)
	usageLogger := logger.NewConsoleLogger(cfg.LogChannelBufferSize)

	// DynamoDB / インメモリ クォータリポジトリ
	quotaRepo := dynamodb.NewQuotaRepository(cfg.DynamoDBEndpoint, cfg.AWSRegion, cfg.DynamoDBTableName, cfg.DefaultTokenQuota)

	// ベンダーアダプター (Azure AI Foundry: GPT/Claude)
	openAIAdapter := adapter.NewOpenAIAdapter(cfg)

	// プロバイダ有効化状況のロギング
	log.Printf("[INFO] Provider Status -> Azure (GPT/Claude): %t", openAIAdapter.IsEnabled())

	// Prometheus メトリクス
	promMetrics := metrics.NewMetrics()

	// プロキシ & WebSocket
	llmProxy := proxy.NewLLMProxy(usageLogger, quotaRepo, promMetrics)
	realtimeProxy := websocket.NewRealtimeProxy(cfg)

	// 3. ユースケース層の初期化
	authUseCase := usecase.NewAuthUseCaseWithConfig(quotaRepo, usecase.AuthUseCaseConfig{
		EnforceTollgateAuth: cfg.EnforceTollgateAuth,
		DefaultTenantID:     cfg.DefaultTenantID,
	})
	adminUseCase := usecase.NewAdminUseCase(quotaRepo)
	chatUseCase := usecase.NewChatUseCase(openAIAdapter, llmProxy)

	internalNotifier := notifier.NewInternalNotifier(quotaRepo)
	batchUseCase := usecase.NewBatchUseCase(quotaRepo, internalNotifier)

	// 4. 定期バッチスケジューラーの初期化 (JST タイムゾーン)
	var cronScheduler *scheduler.CronScheduler
	if cfg.EnableInternalCron {
		cronScheduler = scheduler.NewCronScheduler(batchUseCase)
		cronScheduler.Start()
	}

	// 5. プレゼンテーション層（HTTP ハンドラ・ミドルウェア・レートリミッター）の初期化
	rateLimiter := ratelimit.NewMemoryRateLimiter(cfg.RateLimitRPM)
	rateLimitMiddleware := delivery.NewRateLimitMiddleware(rateLimiter, promMetrics)
	authMiddleware := delivery.NewAuthMiddleware(authUseCase)
	handler := delivery.NewHandler(chatUseCase, realtimeProxy, quotaRepo, authUseCase)
	adminHandler := delivery.NewAdminHandler(adminUseCase, batchUseCase, cfg.AdminAPIKey)

	// 6. ルーティング & Huma v2 (OpenAPI 3.1 & Scalar 自動生成) 設定
	mux := http.NewServeMux()
	delivery.SetupHumaAPI(mux, handler, authMiddleware, adminHandler, cfg.DocsPath, cfg.OpenAPIPath, cfg.InternalSecret, rateLimitMiddleware)

	// OpenAI 互換標準パスの直接ルーティング
	standardChatHandler := authMiddleware.Wrap(rateLimitMiddleware.Wrap(handler.ChatCompletions))
	mux.HandleFunc("/v1/chat/completions", standardChatHandler)
	mux.HandleFunc("/v1/realtime", authMiddleware.Wrap(handler.Realtime))
	mux.HandleFunc("/v1/usage", authMiddleware.Wrap(handler.GetKeyUsage))
	mux.HandleFunc("/health", handler.HealthCheck)
	mux.HandleFunc("/health/live", handler.Liveness)
	mux.HandleFunc("/health/ready", handler.Readiness)
	mux.HandleFunc("/livez", handler.Liveness)
	mux.HandleFunc("/readyz", handler.Readiness)
	mux.Handle("/metrics", promMetrics.Handler())

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           delivery.CORSMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 7. サーバー起動
	go func() {
		log.Printf("[INFO] Server listening on http://0.0.0.0:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server listen failed: %v", err)
		}
	}()

	// 8. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[INFO] Received shutdown signal: %v. Initiating graceful shutdown...", sig)

	// ALB / クラスターからの新規トラフィックを遮断するため、即座に Readiness を落とす
	handler.SetShuttingDown(true)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Server forced shutdown: %v", err)
	}

	// スケジューラーの安全停止
	if cronScheduler != nil {
		cronScheduler.Stop()
	}

	// ロガーのフラッシュ
	if err := usageLogger.Close(); err != nil {
		log.Printf("[ERROR] Error closing usage logger: %v", err)
	}

	log.Println("[INFO] Kura exited successfully.")
}
