package config

import (
	"os"
	"strconv"
)

// Config は Kura の全体設定構造体
type Config struct {
	Port                           string
	AWSRegion                      string
	AzureOpenAIEndpoint            string
	AzureOpenAIAPIKey              string
	AzureAPIVersion                string
	AzureDefaultDeployment         string
	GeminiAPIKey                   string
	GeminiBaseURL                  string
	BedrockRegion                  string
	BedrockAPIKey                  string
	BedrockEndpoint                string
	DefaultTokenQuota              int64
	LogChannelBufferSize           int
	CostStoreType                  string // "sqlite" | "dynamodb" | "valkey" | "redis" | "postgres"
	UsageStoreType                 string // "sqlite" | "dynamodb" | "postgres"
	SQLitePath                     string
	DynamoDBEndpoint               string
	DynamoDBTableName              string
	ValkeyURL                      string
	RedisURL                       string
	PostgresDSN                    string
	ReconcileIntervalSeconds       int
	PricingFilePath                string
	UnknownModelPolicy             string // "warn" | "reject"
	AdminAPIKey                    string
	AzureOpenAIEndpointJapan       string
	DefaultBillingType             string
	EnableInternalCron             bool
	DocsPath                       string
	OpenAPIPath                    string
	EnforceTollgateAuth            bool

	// Cache Layer Settings
	CacheEnabled                   bool
	CacheNegativeTTLSeconds        int
	CacheConfigTTLSeconds          int
	CacheBalanceTTLSeconds         int
	CacheBatchFlushIntervalSeconds int
}

// Load は環境変数から設定を安全に読み込み、デフォルト値を適用する
func Load() *Config {
	return &Config{
		Port:                           getEnv("PORT", "8080"),
		AWSRegion:                      getEnv("AWS_REGION", "ap-northeast-1"),
		AzureOpenAIEndpoint:            getEnv("MICROSOFT_FOUNDRY_ENDPOINT", getEnv("FOUNDRY_ENDPOINT", getEnv("AZURE_OPENAI_ENDPOINT", ""))),
		AzureOpenAIAPIKey:              getEnv("MICROSOFT_FOUNDRY_API_KEY", getEnv("FOUNDRY_API_KEY", getEnv("AZURE_OPENAI_API_KEY", ""))),
		AzureAPIVersion:                getEnv("MICROSOFT_FOUNDRY_API_VERSION", getEnv("FOUNDRY_API_VERSION", getEnv("AZURE_OPENAI_API_VERSION", "2024-02-15-preview"))),
		AzureDefaultDeployment:         getEnv("MICROSOFT_FOUNDRY_DEFAULT_DEPLOYMENT", getEnv("FOUNDRY_DEFAULT_DEPLOYMENT", getEnv("AZURE_OPENAI_DEFAULT_DEPLOYMENT", ""))),
		GeminiAPIKey:                   getEnv("GEMINI_API_KEY", ""),
		GeminiBaseURL:                  getEnv("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		BedrockRegion:                  getEnv("BEDROCK_REGION", getEnv("AWS_REGION", "us-east-1")),
		BedrockAPIKey:                  getEnv("BEDROCK_API_KEY", ""),
		BedrockEndpoint:                getEnv("BEDROCK_ENDPOINT", ""),
		DefaultTokenQuota:              getEnvAsInt64("DEFAULT_TOKEN_QUOTA", 1000000), // デフォルト100万トークン
		LogChannelBufferSize:           getEnvAsInt("LOG_CHANNEL_BUFFER_SIZE", 10000),
		CostStoreType:                  getEnv("COST_STORE", "sqlite"),
		UsageStoreType:                 getEnv("USAGE_STORE", "sqlite"),
		SQLitePath:                     getEnv("SQLITE_PATH", "./data/kura.db"),
		DynamoDBEndpoint:               getEnv("DYNAMODB_ENDPOINT", ""),
		DynamoDBTableName:              getEnv("DYNAMODB_TABLE_NAME", "KuraUsage"),
		ValkeyURL:                      getEnv("VALKEY_URL", getEnv("REDIS_URL", "")),
		RedisURL:                       getEnv("REDIS_URL", ""),
		PostgresDSN:                    getEnv("POSTGRES_DSN", ""),
		ReconcileIntervalSeconds:       getEnvAsInt("RECONCILE_INTERVAL_SECONDS", 0),
		PricingFilePath:                getEnv("PRICING_FILE", "pricing.json"),
		UnknownModelPolicy:             getEnv("UNKNOWN_MODEL_POLICY", "warn"),
		AdminAPIKey:                    getEnv("ADMIN_API_KEY", ""),
		AzureOpenAIEndpointJapan:       getEnv("MICROSOFT_FOUNDRY_ENDPOINT_JAPAN", getEnv("FOUNDRY_ENDPOINT_JAPAN", getEnv("AZURE_OPENAI_ENDPOINT_JAPAN", ""))),
		DefaultBillingType:             getEnv("DEFAULT_BILLING_TYPE", "payg"),
		EnableInternalCron:             getEnvAsBool("ENABLE_INTERNAL_CRON", true),
		DocsPath:                       getEnvPath("DOCS_PATH", "/docs"),       // 空文字または "none"/"off" で無効化
		OpenAPIPath:                    getEnvPath("OPENAPI_PATH", "/openapi"), // 空文字または "none"/"off" で無効化
		EnforceTollgateAuth:            getEnvAsBool("ENFORCE_TOLLGATE_AUTH", false),

		// Cache Layer Settings
		CacheEnabled:                   getEnvAsBool("CACHE_ENABLED", true),
		CacheNegativeTTLSeconds:        getEnvAsInt("CACHE_NEGATIVE_TTL_SECONDS", 300),
		CacheConfigTTLSeconds:          getEnvAsInt("CACHE_CONFIG_TTL_SECONDS", 60),
		CacheBalanceTTLSeconds:         getEnvAsInt("CACHE_BALANCE_TTL_SECONDS", 0), // 既定オフ
		CacheBatchFlushIntervalSeconds: getEnvAsInt("CACHE_BATCH_FLUSH_INTERVAL_SECONDS", 0), // 既定オフ
	}
}

// getEnvPath は環境変数が定義されていれば空文字や指定値をそのまま採用し、未定義ならデフォルト値を返す
func getEnvPath(key, defaultVal string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	if val == "none" || val == "off" || val == "false" {
		return ""
	}
	return val
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt64(key string, defaultVal int64) int64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil {
		return defaultVal
	}
	return val
}

func getEnvAsInt(key string, defaultVal int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}
