package scheduler

import (
	"context"
	"log"

	"github.com/robfig/cron/v3"
	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/usecase"
)

// CronScheduler は Go サーバー内蔵の定期バッチスケジューラー
type CronScheduler struct {
	cron     *cron.Cron
	batchUC  usecase.BatchUseCase
	stopChan chan struct{}
}

// NewCronScheduler は JST ロケーションでスケジューラーを初期化する
func NewCronScheduler(batchUC usecase.BatchUseCase) *CronScheduler {
	// 日本標準時 (JST) で cron を設定
	c := cron.New(cron.WithLocation(entity.JST))

	s := &CronScheduler{
		cron:     c,
		batchUC:  batchUC,
		stopChan: make(chan struct{}),
	}

	// 1. 月次締めレポート通知: 毎月1日 00:05 JST
	_, err := c.AddFunc("5 0 1 * *", func() {
		log.Println("[CRON] Triggered monthly usage report batch...")
		ctx := context.Background()
		if err := batchUC.RunMonthlyReport(ctx); err != nil {
			log.Printf("[ERROR] [CRON] Monthly report failed: %v", err)
		}
	})
	if err != nil {
		log.Printf("[ERROR] Failed to register monthly report cron: %v", err)
	}

	// 2. クォータ警告検知: 毎時 00 分 JST
	_, err = c.AddFunc("0 * * * *", func() {
		log.Println("[CRON] Triggered quota threshold alert batch...")
		ctx := context.Background()
		if err := batchUC.RunQuotaAlerts(ctx); err != nil {
			log.Printf("[ERROR] [CRON] Quota alerts check failed: %v", err)
		}
	})
	if err != nil {
		log.Printf("[ERROR] Failed to register quota alert cron: %v", err)
	}

	return s
}

// Start はスケジューラーをバックグラウンドで開始する
func (s *CronScheduler) Start() {
	log.Println("[INFO] Starting internal cron scheduler (JST timezone)...")
	s.cron.Start()
}

// Stop はスケジューラーを安全に停止する
func (s *CronScheduler) Stop() {
	log.Println("[INFO] Stopping internal cron scheduler...")
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("[INFO] Internal cron scheduler stopped.")
}
