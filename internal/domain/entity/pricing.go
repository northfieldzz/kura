package entity

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"sync"
)

// ModelPricingItem は JSON 料金表内の 1 モデルあたりの単価定義 (USD / 1M tokens)
type ModelPricingItem struct {
	InputCost       float64 `json:"input_cost"`
	OutputCost      float64 `json:"output_cost"`
	CachedInputCost float64 `json:"cached_input_cost,omitempty"`
	ReasoningCost   float64 `json:"reasoning_cost,omitempty"`
}

// PricingConfigFile は pricing.json のルート構造体
type PricingConfigFile struct {
	Version  string                      `json:"version"`
	Currency string                      `json:"currency"`
	Unit     string                      `json:"unit"`
	Models   map[string]ModelPricingItem `json:"models"`
	Aliases  map[string]string           `json:"aliases,omitempty"`
}

// PricingEngine は価格解決とコスト計算を司るスレッドセーフな構造体
type PricingEngine struct {
	mu                 sync.RWMutex
	version            string
	currency           string
	unit               string
	models             map[string]ModelPricingItem
	aliases            map[string]string
	unknownModelPolicy string // "warn" or "reject"
}

// GlobalPricingEngine はグローバルな価格解決エンジンインスタンス
var (
	GlobalPricingEngine *PricingEngine
	pricingInitOnce     sync.Once
)

// FallbackPricingItem は未登録モデル向けのデフォルト単価
var FallbackPricingItem = ModelPricingItem{
	InputCost:       1.00,
	OutputCost:      3.00,
	CachedInputCost: 1.00,
	ReasoningCost:   3.00,
}

// DefaultEngine は初期化済みのデフォルト PricingEngine を返す
func DefaultEngine() *PricingEngine {
	pricingInitOnce.Do(func() {
		GlobalPricingEngine = NewPricingEngine()
	})
	return GlobalPricingEngine
}

// NewPricingEngine はデフォルト値で初期化された PricingEngine を生成する
func NewPricingEngine() *PricingEngine {
	pe := &PricingEngine{
		version:            "2026-09-24",
		currency:           "USD",
		unit:               "per_1m_tokens",
		models:             make(map[string]ModelPricingItem),
		aliases:            make(map[string]string),
		unknownModelPolicy: "warn",
	}

	// 既定モデル単価の初期ロード
	pe.models["gpt-5.4-mini"] = ModelPricingItem{InputCost: 0.15, OutputCost: 0.60, CachedInputCost: 0.075, ReasoningCost: 0.60}
	pe.models["gpt-4o"] = ModelPricingItem{InputCost: 2.50, OutputCost: 10.00, CachedInputCost: 1.25, ReasoningCost: 10.00}
	pe.models["claude-3-5-sonnet"] = ModelPricingItem{InputCost: 3.00, OutputCost: 15.00, CachedInputCost: 0.30, ReasoningCost: 15.00}
	pe.models["claude-3-5-haiku"] = ModelPricingItem{InputCost: 0.80, OutputCost: 4.00, CachedInputCost: 0.08, ReasoningCost: 4.00}
	pe.models["gemini-1.5-flash"] = ModelPricingItem{InputCost: 0.075, OutputCost: 0.30, CachedInputCost: 0.01875, ReasoningCost: 0.30}
	pe.models["gemini-1.5-pro"] = ModelPricingItem{InputCost: 1.25, OutputCost: 5.00, CachedInputCost: 0.3125, ReasoningCost: 5.00}
	pe.models["amazon.titan-text-express-v1"] = ModelPricingItem{InputCost: 0.20, OutputCost: 0.60}
	pe.models["anthropic.claude-3-5-sonnet-20240620-v1:0"] = ModelPricingItem{InputCost: 3.00, OutputCost: 15.00, CachedInputCost: 0.30, ReasoningCost: 15.00}

	// 既定エイリアス
	pe.aliases["fast"] = "gpt-5.4-mini"
	pe.aliases["default"] = "gpt-5.4-mini"
	pe.aliases["smart"] = "claude-3-5-sonnet"
	pe.aliases["code"] = "claude-3-5-sonnet"
	pe.aliases["flash"] = "gemini-1.5-flash"

	return pe
}

// LoadFromFile は指定されたパスの JSON 料金表ファイルを読み込み、スキーマを検証する
func (pe *PricingEngine) LoadFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read pricing file %s: %w", filePath, err)
	}

	var cfg PricingConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("invalid json format in pricing file %s: %w", filePath, err)
	}

	// スキーマ検証
	if cfg.Version == "" {
		return fmt.Errorf("pricing file schema error: 'version' field is required")
	}
	if len(cfg.Models) == 0 {
		return fmt.Errorf("pricing file schema error: 'models' mapping cannot be empty")
	}

	for modelName, item := range cfg.Models {
		if item.InputCost < 0 || item.OutputCost < 0 {
			return fmt.Errorf("pricing file schema error: model %s has negative price", modelName)
		}
	}

	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.version = cfg.Version
	if cfg.Currency != "" {
		pe.currency = cfg.Currency
	}
	if cfg.Unit != "" {
		pe.unit = cfg.Unit
	}
	pe.models = make(map[string]ModelPricingItem, len(cfg.Models))
	for k, v := range cfg.Models {
		pe.models[strings.ToLower(strings.TrimSpace(k))] = v
	}
	if len(cfg.Aliases) > 0 {
		pe.aliases = make(map[string]string, len(cfg.Aliases))
		for k, v := range cfg.Aliases {
			pe.aliases[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}

	log.Printf("[INFO] [PRICING] Successfully loaded pricing table version=%s (models=%d, currency=%s)",
		pe.version, len(pe.models), pe.currency)
	return nil
}

// SetUnknownModelPolicy は未知モデル要求時のポリシー ("warn" | "reject") を設定する
func (pe *PricingEngine) SetUnknownModelPolicy(policy string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if strings.ToLower(policy) == "reject" {
		pe.unknownModelPolicy = "reject"
	} else {
		pe.unknownModelPolicy = "warn"
	}
}

// Version は現在の料金表バージョン文字列を返す
func (pe *PricingEngine) Version() string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.version
}

// ResolveAlias は仮想モデルエイリアスを物理モデル名に解決する
func (pe *PricingEngine) ResolveAlias(aliasOrModel string) string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	lower := strings.ToLower(strings.TrimSpace(aliasOrModel))
	if target, ok := pe.aliases[lower]; ok {
		return target
	}
	return aliasOrModel
}

// ResolveModelAlias はグローバルエンジンを使用した互換関数
func ResolveModelAlias(aliasOrModel string) string {
	return DefaultEngine().ResolveAlias(aliasOrModel)
}

// GetPricing は指定モデルに対応する単価アイテムを取得する
func (pe *PricingEngine) GetPricing(model string) (ModelPricingItem, bool) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	clean := strings.ToLower(strings.TrimSpace(model))
	// 1. 完全一致
	if item, ok := pe.models[clean]; ok {
		return item, true
	}

	// 2. プロバイダープレフィックス除去
	stripped := clean
	stripped = strings.TrimPrefix(stripped, "azure/")
	stripped = strings.TrimPrefix(stripped, "anthropic/")
	stripped = strings.TrimPrefix(stripped, "google/")
	stripped = strings.TrimPrefix(stripped, "bedrock/")
	if item, ok := pe.models[stripped]; ok {
		return item, true
	}

	// 未定義モデル
	return FallbackPricingItem, false
}

// CalculateCost はプロンプトトークン数・完了トークン数・キャッシュトークン数・推論トークン数からコストを計算する
func (pe *PricingEngine) CalculateCost(model string, promptTokens, completionTokens, cachedTokens, reasoningTokens int64) (float64, error) {
	item, found := pe.GetPricing(model)
	if !found {
		if pe.unknownModelPolicy == "reject" {
			return 0, fmt.Errorf("model '%s' is not defined in pricing table and unknown model policy is set to reject", model)
		}
		log.Printf("[WARN] [PRICING] Model '%s' is not defined in pricing table. Using fallback pricing (Input: $%.2f/1M, Output: $%.2f/1M)",
			model, FallbackPricingItem.InputCost, FallbackPricingItem.OutputCost)
	}

	// フォールバック規則: cached / reasoning が 0 なら input / output 単価を採用
	inCost := item.InputCost
	outCost := item.OutputCost
	cachedCost := item.CachedInputCost
	if cachedCost == 0 {
		cachedCost = inCost
	}
	reasonCost := item.ReasoningCost
	if reasonCost == 0 {
		reasonCost = outCost
	}

	regularPrompt := promptTokens - cachedTokens
	if regularPrompt < 0 {
		regularPrompt = 0
	}
	regularCompletion := completionTokens - reasoningTokens
	if regularCompletion < 0 {
		regularCompletion = 0
	}

	totalUSD := (float64(regularPrompt)*inCost +
		float64(cachedTokens)*cachedCost +
		float64(regularCompletion)*outCost +
		float64(reasoningTokens)*reasonCost) / 1000000.0

	// 小数点第6位で四捨五入
	return math.Round(totalUSD*1e6) / 1e6, nil
}

// CalculateCost は旧コード互換用の関数
func CalculateCost(model string, promptTokens, completionTokens int64) float64 {
	cost, _ := DefaultEngine().CalculateCost(model, promptTokens, completionTokens, 0, 0)
	return cost
}
