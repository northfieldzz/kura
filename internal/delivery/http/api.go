package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/usecase"
)

func init() {
	origNewError := huma.NewError
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		// 他サービス (FastAPI / MCP Gateway 等) に合わせて validation エラー時は 422 Unprocessable Entity を返す
		if status == http.StatusBadRequest && (msg == "validation failed" || len(errs) > 0) {
			status = http.StatusUnprocessableEntity
		}
		return origNewError(status, msg, errs...)
	}
}

// HealthOutput はヘルスチェックのレスポンス型
type HealthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"ゲートウェイの稼働ステータス"`
	}
}

// ChatCompletionInput はチャット補完リクエスト型
type ChatCompletionInput struct {
	Authorization string                       `header:"Authorization" doc:"Bearer トークン (service:tenant:user または APIキー)" required:"true" example:"Bearer my-app:team-alpha:user-01"`
	DataResidency string                       `header:"X-Data-Residency" doc:"日本国内リージョンルーティング ('japan' 指定時は東日本リージョン限定)" enum:"japan"`
	Environment   string                       `header:"X-Environment" doc:"実行環境識別子 (例: production, staging, dev)" example:"staging"`
	Feature       string                       `header:"X-Feature" doc:"機能・ユースケース識別子 (例: rag-search, summarize)" example:"rag-search"`
	Tags          string                       `header:"X-Tags" doc:"カンマ区切りタグ (例: 'env=stg,team=alpha')" example:"team=alpha,experiment=1"`
	RequestID     string                       `header:"X-Request-ID" doc:"分散トレース・リクエスト追跡用一意識別子 (UUID)"`
	Traceparent   string                       `header:"traceparent" doc:"W3C 分散トレースコンテキストヘッダー"`
	Body          entity.ChatCompletionRequest `doc:"OpenAI 互換チャット補完リクエストペイロード (仮想モデル fast, smart, flash 対応)"`
}

// ChatCompletionOutput はチャット補完レスポンス型
type ChatCompletionOutput struct {
	BillingType          string                        `header:"X-Billing-Type" doc:"テナントの課金プラン種別 (pay_as_you_go または capped)"`
	QuotaLimitTokens     string                        `header:"X-Quota-Limit-Tokens" doc:"当月のトークン上限設定値 (または 'unlimited')"`
	QuotaRemainingTokens string                        `header:"X-Quota-Remaining-Tokens" doc:"当月の残り利用可能トークン数 (または 'unlimited')"`
	MonthlyUsageTokens   int64                         `header:"X-Monthly-Usage-Tokens" doc:"当月の累計消費トークン数"`
	MonthlyUsageCost     string                        `header:"X-Monthly-Usage-Cost" doc:"当月の累計概算利用コスト (USD)"`
	RateLimitRPM         string                        `header:"X-RateLimit-Limit-RPM" doc:"分間リクエスト上限 (RPM)"`
	RateLimitRemaining   string                        `header:"X-RateLimit-Remaining-RPM" doc:"当分内の残りリクエスト可能数"`
	RequestID            string                        `header:"X-Request-ID" doc:"リクエスト追跡識別子 (UUID)"`
	Body                 entity.ChatCompletionResponse `doc:"チャット完了レスポンスまたは SSE ストリーム"`
}

// RealtimeInput は Realtime WebSocket 接続型
type RealtimeInput struct {
	Authorization string `header:"Authorization" doc:"WebSocket 接続用 Bearer トークン" required:"true" example:"Bearer my-app:team-alpha:user-01"`
}

// RealtimeOutput は Realtime WebSocket 接続レスポンス型
type RealtimeOutput struct {
	Body struct {
		Status  string `json:"status" example:"connected" doc:"WebSocket 接続ステータス"`
		Message string `json:"message" example:"WebSocket protocol upgraded for realtime session" doc:"接続完了メッセージ"`
	} `doc:"WebSocket 接続情報"`
}

// AdminUsageInput は管理者向け利用実績取得入力型
type AdminUsageInput struct {
	AdminKey       string `header:"X-Admin-API-Key" doc:"管理者用マスター API キー (または Authorization: Bearer)" example:"sk-admin-master-key"`
	Authorization  string `header:"Authorization" doc:"管理者用マスター API キー (Bearer 形式)" example:"Bearer sk-admin-master-key"`
	InternalSecret string `header:"X-Internal-Secret" doc:"内部サービス専用シークレット"`
	ServiceID      string `query:"service_id" doc:"集計対象のサービス識別子" required:"true" example:"demo-service"`
	Month          string `query:"month" doc:"対象月 (YYYY-MM形式、省略時は当月)" example:"2026-09"`
}

// AdminUsageOutput は管理者向け利用実績取得出力型
type AdminUsageOutput struct {
	Body entity.ServiceMonthlyReport `doc:"月次利用実績およびモデル別利用内訳レポート"`
}

// AdminLimitsInput は管理者向けリミット設定入力型
type AdminLimitsInput struct {
	AdminKey       string                  `header:"X-Admin-API-Key" doc:"管理者用マスター API キー (または Authorization: Bearer)" example:"sk-admin-master-key"`
	Authorization  string                  `header:"Authorization" doc:"管理者用マスター API キー (Bearer 形式)" example:"Bearer sk-admin-master-key"`
	InternalSecret string                  `header:"X-Internal-Secret" doc:"内部サービス専用シークレット"`
	Body           usecase.SetLimitRequest `doc:"テナントクォータおよび課金プラン設定ペイロード"`
}

// AdminLimitsOutput は管理者向けリミット設定出力型
type AdminLimitsOutput struct {
	Body struct {
		Status  string `json:"status" example:"ok" doc:"実行ステータス"`
		Message string `json:"message" example:"Tenant limit updated successfully" doc:"処理結果メッセージ"`
	}
}

// AdminCreateKeyInput は管理者向け API キー発行入力型
type AdminCreateKeyInput struct {
	AdminKey       string                      `header:"X-Admin-API-Key" doc:"管理者用マスター API キー (または Authorization: Bearer)" example:"sk-admin-master-key"`
	Authorization  string                      `header:"Authorization" doc:"管理者用マスター API キー (Bearer 形式)" example:"Bearer sk-admin-master-key"`
	InternalSecret string                      `header:"X-Internal-Secret" doc:"内部サービス専用シークレット"`
	Body           usecase.CreateAPIKeyRequest `doc:"API キー発行リクエストペイロード"`
}

// AdminCreateKeyOutput は管理者向け API キー発行出力型
type AdminCreateKeyOutput struct {
	Body entity.APIKeyRecord `doc:"発行された API キーレコード"`
}

// AdminListKeysInput は管理者向け API キー一覧取得入力型
type AdminListKeysInput struct {
	AdminKey       string `header:"X-Admin-API-Key" doc:"管理者用マスター API キー (または Authorization: Bearer)" example:"sk-admin-master-key"`
	Authorization  string `header:"Authorization" doc:"管理者用マスター API キー (Bearer 形式)" example:"Bearer sk-admin-master-key"`
	InternalSecret string `header:"X-Internal-Secret" doc:"内部サービス専用シークレット"`
	ServiceID      string `query:"service_id" doc:"集計対象のサービス識別子（未指定時は全件取得）" example:"demo-service"`
}

// AdminListKeysOutput は管理者向け API キー一覧取得出力型
type AdminListKeysOutput struct {
	Body struct {
		ServiceID string                 `json:"service_id"`
		Keys      []*entity.APIKeyRecord `json:"keys"`
	}
}

// AdminRevokeKeyInput は管理者向け API キー失効入力型
type AdminRevokeKeyInput struct {
	AdminKey       string `header:"X-Admin-API-Key" doc:"管理者用マスター API キー (または Authorization: Bearer)" example:"sk-admin-master-key"`
	Authorization  string `header:"Authorization" doc:"管理者用マスター API キー (Bearer 形式)" example:"Bearer sk-admin-master-key"`
	InternalSecret string `header:"X-Internal-Secret" doc:"内部サービス専用シークレット"`
	APIKey         string `query:"api_key" doc:"失効対象の API キー" required:"true" example:"gw-live-xxxxxx"`
}

// AdminRevokeKeyOutput は管理者向け API キー失効出力型
type AdminRevokeKeyOutput struct {
	Body struct {
		Status  string `json:"status" example:"ok" doc:"実行ステータス"`
		Message string `json:"message" example:"API key revoked successfully" doc:"処理結果メッセージ"`
	}
}

// AdminRunJobInput はバッチジョブ手動実行入力型
type AdminRunJobInput struct {
	AdminKey       string `header:"X-Admin-API-Key" doc:"管理者用マスター API キー (または Authorization: Bearer)" example:"sk-admin-master-key"`
	Authorization  string `header:"Authorization" doc:"管理者用マスター API キー (Bearer 形式)" example:"Bearer sk-admin-master-key"`
	InternalSecret string `header:"X-Internal-Secret" doc:"内部サービス専用シークレット"`
	Body           struct {
		JobName string `json:"job_name" required:"true" doc:"実行するジョブ名 (monthly_settlement または quota_alert)" example:"monthly_settlement"`
	}
}

// AdminRunJobOutput はバッチジョブ手動実行出力型
type AdminRunJobOutput struct {
	Body struct {
		Status  string `json:"status" example:"ok" doc:"実行ステータス"`
		Message string `json:"message" example:"Job executed successfully" doc:"処理結果メッセージ"`
	}
}

// AdminListNotificationsInput は管理者向けアプリ内通知一覧取得入力型
type AdminListNotificationsInput struct {
	AdminKey       string `header:"X-Admin-API-Key" doc:"管理者用マスター API キー (または Authorization: Bearer)" example:"sk-admin-master-key"`
	Authorization  string `header:"Authorization" doc:"管理者用マスター API キー (Bearer 形式)" example:"Bearer sk-admin-master-key"`
	InternalSecret string `header:"X-Internal-Secret" doc:"内部サービス専用シークレット"`
	Limit          int    `query:"limit" doc:"取得上限件数 (最大100件、デフォルト20件)" example:"20"`
}

// AdminListNotificationsOutput は管理者向けアプリ内通知一覧取得出力型
type AdminListNotificationsOutput struct {
	Body struct {
		Notifications []*entity.Notification `json:"notifications" doc:"アプリ内通知リスト (新しい順)"`
	}
}


const internalSecretKey = "itcp_internal_service_secret_key_888"

// verifyAdmin は管理者キーおよび内部共有シークレットの適合性を検証する
func verifyAdmin(adminHandler *AdminHandler, adminKey, authHeader, internalSecret string) bool {
	if internalSecret == internalSecretKey {
		return true
	}
	if adminHandler == nil {
		return true
	}
	if adminKey != "" && adminHandler.VerifyKey(adminKey) {
		return true
	}
	if authHeader != "" && adminHandler.VerifyKey(authHeader) {
		return true
	}
	return false
}

// SetupHumaAPI は Huma v2 を初期化し、OpenAPI 3.1 仕様書および Scalar ドキュメントを自動登録する
func SetupHumaAPI(
	mux *http.ServeMux,
	handler *Handler,
	authMiddleware *AuthMiddleware,
	adminHandler *AdminHandler,
	rateLimitMiddleware ...*RateLimitMiddleware,
) huma.API {
	config := huma.DefaultConfig("LLM API Gateway", "2.0.0")
	config.DocsRenderer = huma.DocsRendererScalar
	config.DocsPath = "/api/v1/llm/docs"
	config.OpenAPIPath = "/api/v1/llm/openapi"
	config.Info.Description = "Azure OpenAI、Anthropic Claude、Google Gemini に対応したマルチテナント向け高パフォーマンス LLM API ゲートウェイ。DynamoDB によるトークン計測・コスト算出、完全従量課金 / 上限設定プラン、仮想モデルエイリアス (fast, smart, flash)、日本データレジデンシーに対応。"

	// Scalar の設定: デフォルトクライアントを Go (native) に設定
	config.DocsRendererConfig = map[string]any{
		"defaultHttpClient": map[string]string{
			"targetKey": "go",
			"clientKey": "native",
		},
		"theme": "purple",
	}

	// セキュリティスキームの登録（日本語）
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"TenantAuth": {
			Type:        "http",
			Scheme:      "bearer",
			Description: "API キー認証。フォーマット: 3階層キー `service:tenant:user` またはテナント単一キー",
		},
		"AdminAuth": {
			Type:        "apiKey",
			In:          "header",
			Name:        "X-Admin-API-Key",
			Description: "管理者用マスター API キー認証",
		},
	}

	api := humago.New(mux, config)

	type humaCtxKeyType string
	const humaCtxKey humaCtxKeyType = "huma_ctx"

	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx = huma.WithValue(ctx, humaCtxKey, ctx)
		next(ctx)
	})

	// 1. GET /api/llm/health
	huma.Register(api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/api/llm/health",
		Summary:     "ヘルスチェック",
		Description: "ゲートウェイの稼働状態を確認するエンドポイント。ALB やコンテナの死活監視に使用。",
		Tags:        []string{"システム"},
	}, func(ctx context.Context, input *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	// 2. POST /api/v1/llm/chat/completions (OpenAI compatible chat completion with SSE streaming)
	huma.Register(api, huma.Operation{
		OperationID: "create-chat-completion",
		Method:      http.MethodPost,
		Path:        "/api/v1/llm/chat/completions",
		Summary:     "チャット補完リクエストの作成",
		Description: "OpenAI 互換のチャット補完エンドポイント。Azure OpenAI、Claude、Gemini へのリバースプロキシ中継、トークン集計、コスト算出、上限判定を実行。Server-Sent Events (SSE) によるストリーミングに対応。",
		Tags:        []string{"サービス向け API"},
		Security: []map[string][]string{
			{"TenantAuth": {}},
		},
	}, func(ctx context.Context, input *ChatCompletionInput) (*ChatCompletionOutput, error) {
		if authMiddleware != nil && handler != nil {
			if hCtx, ok := ctx.Value(humaCtxKey).(huma.Context); ok {
				req, rw := humago.Unwrap(hCtx)
				// Huma がアンマーシャル用に r.Body を消費しているため、req.Body を復元
				if bodyBytes, err := json.Marshal(input.Body); err == nil {
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
				chain := handler.ChatCompletions
				if len(rateLimitMiddleware) > 0 && rateLimitMiddleware[0] != nil {
					chain = rateLimitMiddleware[0].Wrap(chain)
				}
				authMiddleware.Wrap(chain)(rw, req)
			}
		}
		return nil, nil
	})

	// 3. GET /api/v1/llm/realtime (WebSocket Passthrough)
	huma.Register(api, huma.Operation{
		OperationID: "realtime-websocket",
		Method:      http.MethodGet,
		Path:        "/api/v1/llm/realtime",
		Summary:     "Realtime WebSocket パススルー",
		Description: "Azure OpenAI Realtime API への WebSocket 接続をパススルーし、低遅延なマルチモーダル音声・テキストストリーミングを実現。",
		Tags:        []string{"サービス向け API"},
		Security: []map[string][]string{
			{"TenantAuth": {}},
		},
	}, func(ctx context.Context, input *RealtimeInput) (*RealtimeOutput, error) {
		if authMiddleware != nil && handler != nil {
			if hCtx, ok := ctx.Value(humaCtxKey).(huma.Context); ok {
				req, rw := humago.Unwrap(hCtx)
				authMiddleware.Wrap(handler.Realtime)(rw, req)
			}
		}
		return nil, nil
	})

	// 4. GET /api/v1/llm/internal/usage
	huma.Register(api, huma.Operation{
		OperationID: "get-internal-usage",
		Method:      http.MethodGet,
		Path:        "/api/v1/llm/internal/usage",
		Summary:     "サービス別月次利用実績の取得",
		Description: "指定されたサービスおよび月のトークン累計消費量、推定コスト、モデル別利用内訳を取得。",
		Tags:        []string{"内部サービス専用 API (Internal)"},
		Security: []map[string][]string{
			{"AdminAuth": {}},
		},
	}, func(ctx context.Context, input *AdminUsageInput) (*AdminUsageOutput, error) {
		if !verifyAdmin(adminHandler, input.AdminKey, input.Authorization, input.InternalSecret) {
			return nil, huma.Error401Unauthorized("Invalid or missing admin API key")
		}
		report, err := adminHandler.UseCase().GetMonthlyUsage(ctx, input.ServiceID, input.Month)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return &AdminUsageOutput{Body: *report}, nil
	})

	// 5. POST /api/v1/llm/internal/limits
	huma.Register(api, huma.Operation{
		OperationID: "set-internal-limits",
		Method:      http.MethodPost,
		Path:        "/api/v1/llm/internal/limits",
		Summary:     "テナントクォータ・課金プランの設定",
		Description: "テナントの月次トークン上限、コスト上限、および課金タイプ (payg / capped) を登録・更新。",
		Tags:        []string{"内部サービス専用 API (Internal)"},
		Security: []map[string][]string{
			{"AdminAuth": {}},
		},
	}, func(ctx context.Context, input *AdminLimitsInput) (*AdminLimitsOutput, error) {
		if !verifyAdmin(adminHandler, input.AdminKey, input.Authorization, input.InternalSecret) {
			return nil, huma.Error401Unauthorized("Invalid or missing admin API key")
		}
		if err := adminHandler.UseCase().SetTenantLimit(ctx, &input.Body); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		out := &AdminLimitsOutput{}
		out.Body.Status = "ok"
		out.Body.Message = "Tenant limit updated successfully"
		return out, nil
	})

	// 6. POST /api/v1/llm/internal/keys
	huma.Register(api, huma.Operation{
		OperationID:   "create-internal-key",
		Method:        http.MethodPost,
		Path:          "/api/v1/llm/internal/keys",
		Summary:       "サービス向け API キーの発行",
		Description:   "指定サービスに紐づくセキュアな API キー (gw-live-xxxxxx) を新規生成し、DynamoDB に永続化する。",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"内部サービス専用 API (Internal)"},
		Security: []map[string][]string{
			{"AdminAuth": {}},
		},
	}, func(ctx context.Context, input *AdminCreateKeyInput) (*AdminCreateKeyOutput, error) {
		if !verifyAdmin(adminHandler, input.AdminKey, input.Authorization, input.InternalSecret) {
			return nil, huma.Error401Unauthorized("Invalid or missing admin API key")
		}
		rec, err := adminHandler.UseCase().CreateAPIKey(ctx, &input.Body)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return &AdminCreateKeyOutput{Body: *rec}, nil
	})

	// 7. GET /api/v1/llm/internal/keys
	huma.Register(api, huma.Operation{
		OperationID: "list-internal-keys",
		Method:      http.MethodGet,
		Path:        "/api/v1/llm/internal/keys",
		Summary:     "サービス別 API キー一覧の取得",
		Description: "指定サービスに発行された API キーの一覧と有効状態を取得する。",
		Tags:        []string{"内部サービス専用 API (Internal)"},
		Security: []map[string][]string{
			{"AdminAuth": {}},
		},
	}, func(ctx context.Context, input *AdminListKeysInput) (*AdminListKeysOutput, error) {
		if !verifyAdmin(adminHandler, input.AdminKey, input.Authorization, input.InternalSecret) {
			return nil, huma.Error401Unauthorized("Invalid or missing admin API key")
		}
		keys, err := adminHandler.UseCase().ListAPIKeys(ctx, input.ServiceID)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		out := &AdminListKeysOutput{}
		out.Body.ServiceID = input.ServiceID
		out.Body.Keys = keys
		return out, nil
	})

	// 8. DELETE /api/v1/llm/internal/keys
	huma.Register(api, huma.Operation{
		OperationID: "revoke-internal-key",
		Method:      http.MethodDelete,
		Path:        "/api/v1/llm/internal/keys",
		Summary:     "API キーの失効・無効化",
		Description: "指定された API キーを即時無効化 (is_active = false) し、Gateway へのアクセスを遮断する。",
		Tags:        []string{"内部サービス専用 API (Internal)"},
		Security: []map[string][]string{
			{"AdminAuth": {}},
		},
	}, func(ctx context.Context, input *AdminRevokeKeyInput) (*AdminRevokeKeyOutput, error) {
		if !verifyAdmin(adminHandler, input.AdminKey, input.Authorization, input.InternalSecret) {
			return nil, huma.Error401Unauthorized("Invalid or missing admin API key")
		}
		if err := adminHandler.UseCase().RevokeAPIKey(ctx, input.APIKey); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		out := &AdminRevokeKeyOutput{}
		out.Body.Status = "ok"
		out.Body.Message = "API key revoked successfully"
		return out, nil
	})

	// 9. POST /api/v1/llm/internal/jobs/run
	huma.Register(api, huma.Operation{
		OperationID: "run-internal-job",
		Method:      http.MethodPost,
		Path:        "/api/v1/llm/internal/jobs/run",
		Summary:     "バッチジョブの手動トリガー実行",
		Description: "指定された定期バッチジョブ (monthly_settlement または quota_alert) を即時実行する。",
		Tags:        []string{"内部サービス専用 API (Internal)"},
		Security: []map[string][]string{
			{"AdminAuth": {}},
		},
	}, func(ctx context.Context, input *AdminRunJobInput) (*AdminRunJobOutput, error) {
		if !verifyAdmin(adminHandler, input.AdminKey, input.Authorization, input.InternalSecret) {
			return nil, huma.Error401Unauthorized("Invalid or missing admin API key")
		}
		if adminHandler == nil || adminHandler.BatchUseCase() == nil {
			return nil, huma.Error500InternalServerError("Batch usecase not configured")
		}

		var err error
		switch input.Body.JobName {
		case "monthly_settlement", "monthly_report":
			err = adminHandler.BatchUseCase().RunMonthlyReport(ctx)
		case "quota_alert", "quota_alerts":
			err = adminHandler.BatchUseCase().RunQuotaAlerts(ctx)
		default:
			return nil, huma.Error400BadRequest("Invalid job_name. Must be 'monthly_report' or 'quota_alerts'")
		}

		if err != nil {
			return nil, huma.Error500InternalServerError("Job execution failed: " + err.Error())
		}

		out := &AdminRunJobOutput{}
		out.Body.Status = "ok"
		out.Body.Message = "Job " + input.Body.JobName + " executed successfully"
		return out, nil
	})

	// 10. GET /api/v1/llm/internal/notifications
	huma.Register(api, huma.Operation{
		OperationID: "list-internal-notifications",
		Method:      http.MethodGet,
		Path:        "/api/v1/llm/internal/notifications",
		Summary:     "アプリ内通知一覧の取得",
		Description: "予算アラートや月次利用実績レポートなど、ゲートウェイ内部に蓄積された通知・お知らせ一覧を降順（最新順）で取得する。",
		Tags:        []string{"内部サービス専用 API (Internal)"},
		Security: []map[string][]string{
			{"AdminAuth": {}},
		},
	}, func(ctx context.Context, input *AdminListNotificationsInput) (*AdminListNotificationsOutput, error) {
		if !verifyAdmin(adminHandler, input.AdminKey, input.Authorization, input.InternalSecret) {
			return nil, huma.Error401Unauthorized("Invalid or missing admin API key")
		}
		if adminHandler == nil || adminHandler.UseCase() == nil {
			return nil, huma.Error500InternalServerError("Admin usecase not configured")
		}

		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		notifications, err := adminHandler.UseCase().ListNotifications(ctx, limit)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to list notifications: " + err.Error())
		}

		out := &AdminListNotificationsOutput{}
		out.Body.Notifications = notifications
		return out, nil
	})


	// 全エンドポイントの OpenAPI 定義に共通ステータスコードを注入
	errorSchema := &huma.Schema{
		Ref: "#/components/schemas/ErrorModel",
	}
	errorContent := map[string]*huma.MediaType{
		"application/problem+json": {Schema: errorSchema},
	}
	commonErrors := map[string]*huma.Response{
		"401": {Description: "API キーが無効または未指定", Content: errorContent},
		"403": {Description: "アクセス権限不足 (管理者権限またはテナント権限不足)", Content: errorContent},
		"404": {Description: "指定されたリソースが存在しない、または非公開 API への外部アクセス遮断", Content: errorContent},
		"422": {Description: "リクエストパラメータまたはボディのバリデーションエラー", Content: errorContent},
		"429": {Description: "月次トークンクォータ超過またはレートリミット上限到達", Content: errorContent},
		"500": {Description: "サーバー内部エラー", Content: errorContent},
		"502": {Description: "上流 LLM プロバイダー (Azure OpenAI / Claude / Gemini) への接続エラー", Content: errorContent},
		"504": {Description: "上流 LLM プロバイダーからの応答タイムアウト", Content: errorContent},
	}

	if openapi := api.OpenAPI(); openapi != nil && openapi.Paths != nil {
		for _, item := range openapi.Paths {
			ops := []*huma.Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch}
			for _, op := range ops {
				if op == nil {
					continue
				}
				if op.Responses == nil {
					op.Responses = make(map[string]*huma.Response)
				}
				for code, resp := range commonErrors {
					if _, exists := op.Responses[code]; !exists {
						op.Responses[code] = resp
					}
				}
			}
		}
	}

	return api
}
