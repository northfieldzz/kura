## 2024-05-19 - Hardcoded Internal Secret Key
**Vulnerability:** Hardcoded internal secret key (`internalSecretKey = "itcp_internal_service_secret_key_888"`) found in `internal/delivery/http/api.go` used to bypass admin authentication.
**Learning:** Hardcoding secrets like internal API keys in the source code provides a critical vulnerability where an attacker with source code access or binary extraction could easily bypass authentication controls, essentially getting full administrative access.
**Prevention:** Avoid hardcoding any secrets in the codebase. Always use environment variables (e.g. `os.Getenv`) and inject these secrets into the application via configuration structures.
## 2025-02-28 - [Timing Attack Mitigation for Secrets]
**Vulnerability:** String comparison (`==`) was being used for validating `adminAPIKey` and `internalSecret`, which can expose the application to timing attacks where an attacker can guess the secret character by character based on response times.
**Learning:** In Go, string comparison terminates at the first mismatched character. This is a common pattern for standard strings but insecure for secrets.
**Prevention:** Always use `crypto/subtle.ConstantTimeCompare` when comparing tokens, API keys, or any secret data. Convert strings to `[]byte` before comparison.
