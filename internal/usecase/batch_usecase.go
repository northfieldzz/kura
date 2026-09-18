package usecase

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/domain/repository"
	"github.com/northfieldzz/kura/internal/infrastructure/notifier"
)

// BatchUseCase は定期バッチ処理のビジネスロジックを担うインターフェース
type BatchUseCase interface {
	// RunMonthlyReport は前月の月次締めレポートを集計し、Slack/ログへ通知する (毎月1日実行)
	RunMonthlyReport(ctx context.Context) error

	// RunQuotaAlerts は当月のクォータ使用率が 80% / 90% を超えたテナントを検知して警告通知する (毎時実行)
	RunQuotaAlerts(ctx context.Context) error
}

type batchUseCase struct {
	repo     repository.QuotaRepository
	notifier notifier.Notifier
}

// NewBatchUseCase は BatchUseCase を生成する
func NewBatchUseCase(repo repository.QuotaRepository, n notifier.Notifier) BatchUseCase {
	return &batchUseCase{
		repo:     repo,
		notifier: n,
	}
}

// RunMonthlyReport は前月の月次締めレポートを集計・通知する
func (u *batchUseCase) RunMonthlyReport(ctx context.Context) error {
	now := time.Now().In(entity.JST)
	// 前月キー (例: 9月1日実行なら 2026-08)
	lastMonth := entity.FormatMonthJST(now.AddDate(0, -1, 0))
	lockKey := "monthly_report#" + lastMonth

	// 1. DynamoDB 条件付き書き込みによる分散ロック取得 (30日間有効)
	acquired, err := u.repo.AcquireLock(ctx, lockKey, 30*86400)
	if err != nil {
		return fmt.Errorf("failed to acquire lock for monthly report: %w", err)
	}
	if !acquired {
		log.Printf("[INFO] [CRON] Monthly report for %s was already processed by another instance. Skipping.", lastMonth)
		return nil
	}

	log.Printf("[INFO] [CRON] Acquired lock for monthly report (%s). Generating report...", lastMonth)

	// 2. 前月分の全テナント利用実績を取得
	tenants, err := u.repo.GetAllTenantsUsageByMonth(ctx, lastMonth)
	if err != nil {
		return fmt.Errorf("failed to fetch monthly usage for %s: %w", lastMonth, err)
	}

	if len(tenants) == 0 {
		msg := fmt.Sprintf("対象月: %s\n前月の利用実績レコードは存在しませんでした（利用量 0）。", lastMonth)
		_ = u.notifier.Send(ctx, fmt.Sprintf("📊 Kura 月次利用実績レポート (%s)", lastMonth), msg, false)
		return nil
	}

	// サービス別に合算集計
	type serviceAgg struct {
		serviceID   string
		totalTokens int64
		totalCost   float64
		tenantCount int
	}
	serviceMap := make(map[string]*serviceAgg)
	var grandTotalTokens int64
	var grandTotalCost float64

	for _, t := range tenants {
		sID := t.ServiceID
		if sID == "" {
			sID = "default"
		}
		agg, ok := serviceMap[sID]
		if !ok {
			agg = &serviceAgg{serviceID: sID}
			serviceMap[sID] = agg
		}
		agg.totalTokens += t.TotalTokens
		agg.totalCost += t.TotalCost
		agg.tenantCount++

		grandTotalTokens += t.TotalTokens
		grandTotalCost += t.TotalCost
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*【確定月次利用レポート: %s】*\n", lastMonth))
	sb.WriteString(fmt.Sprintf("• 全社累計消費トークン: `%d` tokens\n", grandTotalTokens))
	sb.WriteString(fmt.Sprintf("• 全社概算請求金額: `$%.4f USD`\n", grandTotalCost))
	sb.WriteString(fmt.Sprintf("• 稼働テナント総数: `%d`\n\n", len(tenants)))
	sb.WriteString("*─── サービス別内訳 ───*\n")

	// ソートして出力
	var services []*serviceAgg
	for _, agg := range serviceMap {
		services = append(services, agg)
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].totalCost > services[j].totalCost
	})

	for _, s := range services {
		sb.WriteString(fmt.Sprintf("• *%s* (テナント数: %d): `%d` tokens | `$%.4f USD`\n",
			s.serviceID, s.tenantCount, s.totalTokens, s.totalCost))
	}

	title := fmt.Sprintf("📊 Kura 月次利用実績レポート (%s)", lastMonth)
	return u.notifier.Send(ctx, title, sb.String(), false)
}

// RunQuotaAlerts はクォータ上限間近 (80%/90%) のテナントを検知・通知する
func (u *batchUseCase) RunQuotaAlerts(ctx context.Context) error {
	now := time.Now().In(entity.JST)
	currentMonth := entity.CurrentMonthJST()
	hourSlot := now.Format("2006-01-02-15")
	lockKey := "quota_alert#" + hourSlot

	// 1. DynamoDB 分散ロック取得 (1時間有効)
	acquired, err := u.repo.AcquireLock(ctx, lockKey, 3600)
	if err != nil {
		return fmt.Errorf("failed to acquire lock for quota alert: %w", err)
	}
	if !acquired {
		log.Printf("[INFO] [CRON] Quota alert for slot %s was already processed. Skipping.", hourSlot)
		return nil
	}

	// 2. 当月分の全テナント利用実績を取得し、サービス単位に合算
	tenants, err := u.repo.GetAllTenantsUsageByMonth(ctx, currentMonth)
	if err != nil {
		return fmt.Errorf("failed to fetch tenants for quota alerts: %w", err)
	}

	serviceCosts := make(map[string]float64)
	var serviceIDs []string
	for _, t := range tenants {
		if _, exists := serviceCosts[t.ServiceID]; !exists {
			serviceIDs = append(serviceIDs, t.ServiceID)
		}
		serviceCosts[t.ServiceID] += t.TotalCost
	}

	configs, err := u.repo.GetServiceConfigs(ctx, serviceIDs)
	if err != nil {
		log.Printf("[WARN] [CRON] Failed to get service configs for quota alerts: %v", err)
	}

	var alertLines []string
	for serviceID, totalCost := range serviceCosts {
		cfg, ok := configs[serviceID]
		if !ok || cfg == nil {
			continue
		}
		// capped プランでコスト上限値が設定されているもののみチェック
		if cfg.CostLimit <= 0 || cfg.EffectiveBillingType() != entity.BillingTypeCapped {
			continue
		}

		rate := (totalCost / cfg.CostLimit) * 100.0
		if rate >= 80.0 {
			urgency := "⚠️ [警戒: 80%超]"
			if rate >= 90.0 {
				urgency = "🚨 *[危険: 90%超]*"
			}
			alertLines = append(alertLines, fmt.Sprintf("%s サービス: *%s* -> コスト消費率: `%.1f%%` (当月合計消費: $%.4f / サービス上限: $%.2f)",
				urgency, serviceID, rate, totalCost, cfg.CostLimit))
		}
	}

	if len(alertLines) == 0 {
		return nil // 警告対象なし
	}

	title := fmt.Sprintf("🚨 月次コスト上限接近アラート (%s)", currentMonth)
	msg := fmt.Sprintf("当月の月次コスト上限に近づいているサービスが検出されました。\n上限到達（HTTP 429）によるサービス停止を防ぐため、必要に応じて上限引き上げを実施してください。\n\n%s",
		strings.Join(alertLines, "\n"))

	return u.notifier.Send(ctx, title, msg, true)
}
