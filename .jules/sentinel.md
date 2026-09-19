## 2024-05-19 - Hardcoded Internal Secret Key
**Vulnerability:** Hardcoded internal secret key (`internalSecretKey = "itcp_internal_service_secret_key_888"`) found in `internal/delivery/http/api.go` used to bypass admin authentication.
**Learning:** Hardcoding secrets like internal API keys in the source code provides a critical vulnerability where an attacker with source code access or binary extraction could easily bypass authentication controls, essentially getting full administrative access.
**Prevention:** Avoid hardcoding any secrets in the codebase. Always use environment variables (e.g. `os.Getenv`) and inject these secrets into the application via configuration structures.
