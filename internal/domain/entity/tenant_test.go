package entity

import (
	"testing"
)

func TestServiceConfig_IsExceeded(t *testing.T) {
	tests := []struct {
		name       string
		cfg        ServiceConfig
		totalCost  float64
		wantExceed bool
	}{
		{
			name: "PAYG - No limits",
			cfg: ServiceConfig{
				CostLimit: 0,
			},
			totalCost:  120.50,
			wantExceed: false,
		},
		{
			name: "Capped - Within cost limit",
			cfg: ServiceConfig{
				CostLimit: 50.0,
			},
			totalCost:  49.99,
			wantExceed: false,
		},
		{
			name: "Capped - Cost limit reached exactly",
			cfg: ServiceConfig{
				CostLimit: 50.0,
			},
			totalCost:  50.0,
			wantExceed: true,
		},
		{
			name: "Capped - Cost limit exceeded",
			cfg: ServiceConfig{
				CostLimit: 50.0,
			},
			totalCost:  50.01,
			wantExceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsExceeded(tt.totalCost); got != tt.wantExceed {
				t.Errorf("IsExceeded() = %v, want %v", got, tt.wantExceed)
			}
		})
	}
}

func TestServiceConfig_EffectiveBillingType(t *testing.T) {
	cfgPayg := ServiceConfig{BillingType: "payg", CostLimit: 0}
	if got := cfgPayg.EffectiveBillingType(); got != BillingTypePAYG {
		t.Errorf("EffectiveBillingType() = %v, want %v", got, BillingTypePAYG)
	}

	cfgCapped := ServiceConfig{BillingType: "", CostLimit: 50.0}
	if got := cfgCapped.EffectiveBillingType(); got != BillingTypeCapped {
		t.Errorf("EffectiveBillingType() = %v, want %v", got, BillingTypeCapped)
	}
}
