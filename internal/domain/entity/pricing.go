package entity

import (
	"math"
	"strings"
)

// ModelPricing は100万トークンあたりの単価 (USD)
type ModelPricing struct {
	InputPricePer1M  float64
	OutputPricePer1M float64
}

// DefaultPricingTable は既知モデルの標準単価テーブル (USD / 1M tokens)
var DefaultPricingTable = map[string]ModelPricing{
	"gpt-5.4-mini":      {InputPricePer1M: 0.15, OutputPricePer1M: 0.60},
	"gpt-4o":            {InputPricePer1M: 2.50, OutputPricePer1M: 10.00},
	"claude-3-5-sonnet": {InputPricePer1M: 3.00, OutputPricePer1M: 15.00},
	"claude-3-5-haiku":  {InputPricePer1M: 0.80, OutputPricePer1M: 4.00},
	"gemini-1.5-flash":  {InputPricePer1M: 0.075, OutputPricePer1M: 0.30},
	"gemini-1.5-pro":    {InputPricePer1M: 1.25, OutputPricePer1M: 5.00},
}

var FallbackPricing = ModelPricing{InputPricePer1M: 1.00, OutputPricePer1M: 3.00}

// ResolveModelAlias は仮想モデル名（エイリアス）を物理モデル名に変換する
// 直接のモデル名指定時はそのまま透過する
func ResolveModelAlias(aliasOrModel string) string {
	lower := strings.ToLower(strings.TrimSpace(aliasOrModel))
	switch lower {
	case "fast", "default":
		return "gpt-5.4-mini"
	case "smart", "code":
		return "claude-3-5-sonnet"
	case "flash":
		return "gemini-1.5-flash"
	default:
		return aliasOrModel
	}
}

// GetPricingForModel はモデル名に対応する単価を取得する（未登録時は FallbackPricing を返す）
func GetPricingForModel(model string) ModelPricing {
	cleanModel := strings.ToLower(strings.TrimSpace(model))
	cleanModel = strings.TrimPrefix(cleanModel, "azure/")
	cleanModel = strings.TrimPrefix(cleanModel, "anthropic/")
	cleanModel = strings.TrimPrefix(cleanModel, "google/")

	// 完全一致判定
	if pricing, ok := DefaultPricingTable[cleanModel]; ok {
		return pricing
	}

	return FallbackPricing
}

// CalculateCost はプロンプトトークンと完了トークンから利用費用(USD)を計算する
// Cost = (PromptTokens * InPrice + CompletionTokens * OutPrice) / 1,000,000
func CalculateCost(model string, promptTokens, completionTokens int64) float64 {
	pricing := GetPricingForModel(model)
	cost := (float64(promptTokens)*pricing.InputPricePer1M + float64(completionTokens)*pricing.OutputPricePer1M) / 1000000.0
	// 小数点第6位で四捨五入
	return math.Round(cost*1e6) / 1e6
}
