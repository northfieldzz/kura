package config

import (
	"strings"

	"os"
	"testing"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear all env vars that might affect the test
	clearEnv(t)

	// Since we are clearing the environment, we must unset ADMIN_API_KEY explicitly
	// in case t.Setenv overrides the effect of os.Clearenv in some Go versions/environments.
	os.Unsetenv("ADMIN_API_KEY")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Expected default Port to be 8080, got %s", cfg.Port)
	}
	if cfg.AWSRegion != "ap-northeast-1" {
		t.Errorf("Expected default AWSRegion to be ap-northeast-1, got %s", cfg.AWSRegion)
	}
	if cfg.AzureAPIVersion != "2024-02-15-preview" {
		t.Errorf("Expected default AzureAPIVersion to be 2024-02-15-preview, got %s", cfg.AzureAPIVersion)
	}
	if cfg.GeminiBaseURL != "https://generativelanguage.googleapis.com" {
		t.Errorf("Expected default GeminiBaseURL to be https://generativelanguage.googleapis.com, got %s", cfg.GeminiBaseURL)
	}
	if cfg.DefaultTokenQuota != 1000000 {
		t.Errorf("Expected default DefaultTokenQuota to be 1000000, got %d", cfg.DefaultTokenQuota)
	}
	if cfg.LogChannelBufferSize != 10000 {
		t.Errorf("Expected default LogChannelBufferSize to be 10000, got %d", cfg.LogChannelBufferSize)
	}
	if cfg.DynamoDBTableName != "KuraUsage" {
		t.Errorf("Expected default DynamoDBTableName to be KuraUsage, got %s", cfg.DynamoDBTableName)
	}
	if cfg.AdminAPIKey != "sk-admin-master-key" {
		t.Errorf("Expected default AdminAPIKey to be sk-admin-master-key, got %s", cfg.AdminAPIKey)
	}
	if cfg.DefaultBillingType != "payg" {
		t.Errorf("Expected default DefaultBillingType to be payg, got %s", cfg.DefaultBillingType)
	}
	if cfg.EnableInternalCron != true {
		t.Errorf("Expected default EnableInternalCron to be true, got %v", cfg.EnableInternalCron)
	}
	if cfg.RateLimitRPM != 600 {
		t.Errorf("Expected default RateLimitRPM to be 600, got %d", cfg.RateLimitRPM)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("DEFAULT_TOKEN_QUOTA", "500")
	t.Setenv("ENABLE_INTERNAL_CRON", "false")
	t.Setenv("RATE_LIMIT_RPM", "100")
	t.Setenv("AZURE_OPENAI_ENDPOINT", "custom-endpoint")
	t.Setenv("ADMIN_API_KEY", "custom-admin-key")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("Expected Port to be 9090, got %s", cfg.Port)
	}
	if cfg.AWSRegion != "us-east-1" {
		t.Errorf("Expected AWSRegion to be us-east-1, got %s", cfg.AWSRegion)
	}
	if cfg.DefaultTokenQuota != 500 {
		t.Errorf("Expected DefaultTokenQuota to be 500, got %d", cfg.DefaultTokenQuota)
	}
	if cfg.EnableInternalCron != false {
		t.Errorf("Expected EnableInternalCron to be false, got %v", cfg.EnableInternalCron)
	}
	if cfg.RateLimitRPM != 100 {
		t.Errorf("Expected RateLimitRPM to be 100, got %d", cfg.RateLimitRPM)
	}
	if cfg.AzureOpenAIEndpoint != "custom-endpoint" {
		t.Errorf("Expected AzureOpenAIEndpoint to be custom-endpoint, got %s", cfg.AzureOpenAIEndpoint)
	}
	if cfg.AdminAPIKey != "custom-admin-key" {
		t.Errorf("Expected AdminAPIKey to be custom-admin-key, got %s", cfg.AdminAPIKey)
	}
}

func TestLoad_FallbackValues(t *testing.T) {
	clearEnv(t)

	// Level 3 fallback
	t.Setenv("AZURE_OPENAI_ENDPOINT", "azure-endpoint")
	cfg := Load()
	if cfg.AzureOpenAIEndpoint != "azure-endpoint" {
		t.Errorf("Expected AzureOpenAIEndpoint to be azure-endpoint, got %s", cfg.AzureOpenAIEndpoint)
	}

	// Level 2 fallback
	t.Setenv("FOUNDRY_ENDPOINT", "foundry-endpoint")
	cfg = Load()
	if cfg.AzureOpenAIEndpoint != "foundry-endpoint" {
		t.Errorf("Expected AzureOpenAIEndpoint to be foundry-endpoint, got %s", cfg.AzureOpenAIEndpoint)
	}

	// Level 1 fallback (highest priority)
	t.Setenv("MICROSOFT_FOUNDRY_ENDPOINT", "ms-foundry-endpoint")
	cfg = Load()
	if cfg.AzureOpenAIEndpoint != "ms-foundry-endpoint" {
		t.Errorf("Expected AzureOpenAIEndpoint to be ms-foundry-endpoint, got %s", cfg.AzureOpenAIEndpoint)
	}
}

func TestGetEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("TEST_GET_ENV", "value")

	if val := getEnv("TEST_GET_ENV", "default"); val != "value" {
		t.Errorf("Expected getEnv to return 'value', got %s", val)
	}

	if val := getEnv("TEST_GET_ENV_MISSING", "default"); val != "default" {
		t.Errorf("Expected getEnv to return 'default', got %s", val)
	}
}

func TestGetEnvAsBool(t *testing.T) {
	clearEnv(t)
	t.Setenv("TEST_BOOL_TRUE", "true")
	t.Setenv("TEST_BOOL_FALSE", "false")
	t.Setenv("TEST_BOOL_INVALID", "invalid")

	if val := getEnvAsBool("TEST_BOOL_TRUE", false); val != true {
		t.Errorf("Expected getEnvAsBool to return true, got %v", val)
	}

	if val := getEnvAsBool("TEST_BOOL_FALSE", true); val != false {
		t.Errorf("Expected getEnvAsBool to return false, got %v", val)
	}

	if val := getEnvAsBool("TEST_BOOL_INVALID", true); val != true {
		t.Errorf("Expected getEnvAsBool to return true for invalid input, got %v", val)
	}

	if val := getEnvAsBool("TEST_BOOL_MISSING", true); val != true {
		t.Errorf("Expected getEnvAsBool to return true for missing input, got %v", val)
	}
}

func TestGetEnvAsInt(t *testing.T) {
	clearEnv(t)
	t.Setenv("TEST_INT_VALID", "42")
	t.Setenv("TEST_INT_INVALID", "invalid")

	if val := getEnvAsInt("TEST_INT_VALID", 10); val != 42 {
		t.Errorf("Expected getEnvAsInt to return 42, got %v", val)
	}

	if val := getEnvAsInt("TEST_INT_INVALID", 10); val != 10 {
		t.Errorf("Expected getEnvAsInt to return default 10 for invalid input, got %v", val)
	}

	if val := getEnvAsInt("TEST_INT_MISSING", 10); val != 10 {
		t.Errorf("Expected getEnvAsInt to return default 10 for missing input, got %v", val)
	}
}

func TestGetEnvAsInt64(t *testing.T) {
	clearEnv(t)
	t.Setenv("TEST_INT64_VALID", "42")
	t.Setenv("TEST_INT64_INVALID", "invalid")

	if val := getEnvAsInt64("TEST_INT64_VALID", 10); val != 42 {
		t.Errorf("Expected getEnvAsInt64 to return 42, got %v", val)
	}

	if val := getEnvAsInt64("TEST_INT64_INVALID", 10); val != 10 {
		t.Errorf("Expected getEnvAsInt64 to return default 10 for invalid input, got %v", val)
	}

	if val := getEnvAsInt64("TEST_INT64_MISSING", 10); val != 10 {
		t.Errorf("Expected getEnvAsInt64 to return default 10 for missing input, got %v", val)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	env := os.Environ()
	t.Cleanup(func() {
		os.Clearenv()
		for _, e := range env {
			if i := strings.Index(e, "="); i != -1 {
				os.Setenv(e[:i], e[i+1:])
			}
		}
	})
	os.Clearenv()
}
