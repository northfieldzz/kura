package config

import (
	"os"
	"strconv"
)

// Config は Kura の全体設定構造体
type Config struct {
	Port                     string
	AWSRegion                string
	AzureOpenAIEndpoint      string
	AzureOpenAIAPIKey        string
	AzureAPIVersion          string
	AzureDefaultDeployment   string
	GeminiAPIKey             string
	GeminiBaseURL            string
	DefaultTokenQuota        int64
	LogChannelBufferSize     int
	DynamoDBEndpoint         string
	DynamoDBTableName        string
	AdminAPIKey              string
	AzureOpenAIEndpointJapan string
	DefaultBillingType       string
	EnableInternalCron       bool
	RateLimitRPM             int
}

// Load は環境変数から設定を安全に読み込み、デフォルト値を適用する
func Load() *Config {
	return &Config{
		Port:                     getEnv("PORT", "8080"),
		AWSRegion:                getEnv("AWS_REGION", "ap-northeast-1"),
		AzureOpenAIEndpoint:      getEnv("MICROSOFT_FOUNDRY_ENDPOINT", getEnv("FOUNDRY_ENDPOINT", getEnv("AZURE_OPENAI_ENDPOINT", ""))),
		AzureOpenAIAPIKey:        getEnv("MICROSOFT_FOUNDRY_API_KEY", getEnv("FOUNDRY_API_KEY", getEnv("AZURE_OPENAI_API_KEY", ""))),
		AzureAPIVersion:          getEnv("MICROSOFT_FOUNDRY_API_VERSION", getEnv("FOUNDRY_API_VERSION", getEnv("AZURE_OPENAI_API_VERSION", "2024-02-15-preview"))),
		AzureDefaultDeployment:   getEnv("MICROSOFT_FOUNDRY_DEFAULT_DEPLOYMENT", getEnv("FOUNDRY_DEFAULT_DEPLOYMENT", getEnv("AZURE_OPENAI_DEFAULT_DEPLOYMENT", ""))),
		GeminiAPIKey:             getEnv("GEMINI_API_KEY", ""),
		GeminiBaseURL:            getEnv("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		DefaultTokenQuota:        getEnvAsInt64("DEFAULT_TOKEN_QUOTA", 1000000), // デフォルト100万トークン
		LogChannelBufferSize:     getEnvAsInt("LOG_CHANNEL_BUFFER_SIZE", 10000),
		DynamoDBEndpoint:         getEnv("DYNAMODB_ENDPOINT", ""),
		DynamoDBTableName:        getEnv("DYNAMODB_TABLE_NAME", "KuraUsage"),
		AdminAPIKey:              getEnv("ADMIN_API_KEY", ""),
		AzureOpenAIEndpointJapan: getEnv("MICROSOFT_FOUNDRY_ENDPOINT_JAPAN", getEnv("FOUNDRY_ENDPOINT_JAPAN", getEnv("AZURE_OPENAI_ENDPOINT_JAPAN", ""))),
		DefaultBillingType:       getEnv("DEFAULT_BILLING_TYPE", "payg"),
		EnableInternalCron:       getEnvAsBool("ENABLE_INTERNAL_CRON", true),
		RateLimitRPM:             getEnvAsInt("RATE_LIMIT_RPM", 600), // デフォルト 600 req/min (0なら無制限)
	}
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
