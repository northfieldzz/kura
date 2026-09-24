package config_test

import (
	"testing"

	"github.com/northfieldzz/kura/internal/infrastructure/config"
)

func TestValidateGatewayAuth_InsecureFlag(t *testing.T) {
	cfg := &config.Config{
		InsecureNoGatewayAuth: true,
		GatewaySharedSecret:   "",
	}
	if err := cfg.ValidateGatewayAuth(); err != nil {
		t.Fatalf("expected nil error when InsecureNoGatewayAuth is true, got %v", err)
	}
}

func TestValidateGatewayAuth_MissingSecret(t *testing.T) {
	cfg := &config.Config{
		InsecureNoGatewayAuth: false,
		GatewaySharedSecret:   "",
	}
	if err := cfg.ValidateGatewayAuth(); err == nil {
		t.Fatalf("expected error when GatewaySharedSecret is empty and InsecureNoGatewayAuth is false")
	}
}

func TestValidateGatewayAuth_TooShortSecret(t *testing.T) {
	cfg := &config.Config{
		InsecureNoGatewayAuth: false,
		GatewaySharedSecret:   "short-secret-less-than-32-chars",
	}
	if err := cfg.ValidateGatewayAuth(); err == nil {
		t.Fatalf("expected error for secret with length < 32")
	}
}

func TestValidateGatewayAuth_ValidSecret(t *testing.T) {
	cfg := &config.Config{
		InsecureNoGatewayAuth: false,
		GatewaySharedSecret:   "12345678901234567890123456789012", // 32 chars
	}
	if err := cfg.ValidateGatewayAuth(); err != nil {
		t.Fatalf("expected nil error for valid 32 chars secret, got %v", err)
	}
}

func TestValidateGatewayAuth_InvalidPreviousSecret(t *testing.T) {
	cfg := &config.Config{
		InsecureNoGatewayAuth:       false,
		GatewaySharedSecret:         "12345678901234567890123456789012",
		GatewaySharedSecretPrevious: "too-short",
	}
	if err := cfg.ValidateGatewayAuth(); err == nil {
		t.Fatalf("expected error when GatewaySharedSecretPrevious is too short")
	}
}

func TestValidateGatewayAuth_ValidPreviousSecret(t *testing.T) {
	cfg := &config.Config{
		InsecureNoGatewayAuth:       false,
		GatewaySharedSecret:         "12345678901234567890123456789012",
		GatewaySharedSecretPrevious: "abcdefabcdefabcdefabcdefabcdefab",
	}
	if err := cfg.ValidateGatewayAuth(); err != nil {
		t.Fatalf("expected nil error for valid previous secret, got %v", err)
	}
}
