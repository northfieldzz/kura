package usecase

import (
	"context"
	"fmt"
	"testing"

	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/infrastructure/dynamodb"
)

func BenchmarkRunQuotaAlerts_InnerLoop(b *testing.B) {
	repo := dynamodb.NewMemoryQuotaRepository(1000000)
	ctx := context.Background()

	serviceCosts := make(map[string]float64)

	// Create 1000 services with 85% usage (triggering limits)
	for i := 0; i < 1000; i++ {
		svc := fmt.Sprintf("svc-%d", i)
		_ = repo.SetServiceLimit(ctx, svc, 100.0, string(entity.BillingTypeCapped))
		serviceCosts[svc] = 85.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var serviceIDs []string
		for serviceID := range serviceCosts {
			serviceIDs = append(serviceIDs, serviceID)
		}

		configs, _ := repo.GetServiceConfigs(ctx, serviceIDs)

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
				alertLines = append(alertLines, fmt.Sprintf("%s サービス: *%s* -> コスト消費率: `%.1f%%`",
					urgency, serviceID, rate))
			}
		}
		_ = alertLines
	}
}
