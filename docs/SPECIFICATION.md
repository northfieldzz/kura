# LLM API Gateway システム包括仕様書

## 1. システム概要
本システムは、マルチテナント環境における大規模言語モデル（LLM）へのアクセスを一元管理・中継する高パフォーマンスな API ゲートウェイである。
Azure OpenAI、Anthropic Claude、Google Gemini のマルチプロバイダーに対応し、テナントごとのトークン消費量・コストのリアルタイム集計、プラン別クォータ制御（従量課金 / 上限設定）、日本データレジデンシー対応、オブザーバビリティ、および管理用 API を提供する。

```
                       +-----------------------------+
                       |      Client Application     |
                       +-----------------------------+
                                      |
                                      | HTTP / WebSocket
                                      v
       +-------------------------------------------------------------+
       |                      LLM API Gateway                        |
       |  - 3-tier Auth (Service / Tenant / User)                    |
       |  - Model Alias Resolution (fast, smart, flash)              |
       |  - Billing & Quota Enforcement (PayG / Capped)              |
       |  - Response Headers (X-Billing-Type, X-Quota-Remaining, etc)|
       |  - Observability & Distributed Tracing                      |
       |  - Japan Data Residency Router                              |
       +-------------------------------------------------------------+
              |                      |                      |
              v                      v                      v
       +-------------+        +-------------+        +-------------+
       | Azure OpenAI|        |  Anthropic  |        |    Google   |
       |   Foundry   |        |   Claude    |        |    Gemini   |
       +-------------+        +-------------+        +-------------+
              ^
              | Token Metering & Cost Tracking
              v
       +------------------------------------+
       |          Amazon DynamoDB           |
       |     Table: LLMGatewayUsage         |
       +------------------------------------+
```

---

## 2. アーキテクチャ & レイヤー設計
Standard Go Project Layout および Clean Architecture の原則に準拠した構成をとる。

| ディレクトリ | 役割 |
|---|---|
| `cmd/server/` | エントリーポイント（設定ロード、DI、ルーティング、Graceful Shutdown） |
| `internal/domain/entity/` | ドメインエンティティ（テナントコンテキスト、利用状況、課金タイプ、単価マスタ） |
| `internal/domain/repository/` | リポジトリインターフェース（`QuotaRepository`） |
| `internal/infrastructure/` | 外部連携（DynamoDB、Azure/OpenAI/Claude/Gemini アダプター、リバースプロキシ、WebSocket、ロガー） |
| `internal/usecase/` | ビジネスロジック（認証・認可、クォータ判定、チャット中継、管理機能） |
| `internal/delivery/http/` | HTTP プレゼンテーション層（ハンドラー、ミドルウェア、エラーレスポンス、Scalar ドキュメント） |
| `internal/delivery/http/docs/`| OpenAPI 3.1 仕様書 (`openapi.json`) |
| `docs/` | システム仕様書、Phase ドキュメント |

---

## 3. マルチテナント & 認証仕様

### 3.1 認証キー構造
Gateway へのリクエストは `Authorization: Bearer <API_KEY>` ヘッダーで行う。
API キーは以下の 3 階層コロン区切り形式を標準とする。

```
<ServiceID>:<TenantID>:<UserID>
```

- 例: `finance-app:dept-risk-01:user-4521`
  - `ServiceID`: 呼び出し元アプリケーション・サービス識別子
  - `TenantID`: 課金・クォータ単位となるテナント識別子（DynamoDB の PK）
  - `UserID`: エンドユーザー識別子（ログ・監査用）
- 後方互換性: コロンを含まない単一キー（例: `tenant-alpha`）が渡された場合、キー全体を `TenantID` とみなし、`ServiceID`・`UserID` は空文字として処理する。

### 3.2 管理用認証
管理用 API（`/v1/admin/*`）は `X-Admin-API-Key` ヘッダーによるマスターキー認証を行う。
環境変数 `ADMIN_API_KEY`（デフォルト: `sk-admin-master-key`）と一致しない場合は HTTP 401 を即座に返却する。

---

## 4. 課金モデル & クォータ制御

### 4.1 課金プラン種別 (`BillingType`)

| プラン (`billing_type`) | 上限設定 (`token_quota`) | 挙動 |
|---|---|---|
| `pay_as_you_go` (完全従量課金) | 任意 (0 または任意値) | 上限到達によるブロックは発生しない。月次利用トークンおよび利用金額（USD）がリアルタイム加算・追跡される。 |
| `capped` (上限設定プラン) | 正の整数 (例: `1000000`) | 当月の累計利用トークン数が `token_quota` に達している場合、リクエストを **HTTP 429 Quota Exceeded** で遮断する。 |

新規テナントの初回アクセス時は、環境変数 `DEFAULT_TOKEN_QUOTA`（デフォルト: 1,000,000 トークン）を上限とする `capped` プランとして自動初期化される。

### 4.2 コスト計算 (単価マスタ)
モデルごとの入力・出力トークン数に基づき、USD 単位のコストを自動算出する。

| モデル種別 / プレフィックス | 入力単価 (USD / 1M tokens) | 出力単価 (USD / 1M tokens) |
|---|---|---|
| `gpt-4o`, `gpt-5` | $2.50 | $10.00 |
| `gpt-4o-mini`, `gpt-5-mini`, `gpt-5.4-mini` | $0.15 | $0.60 |
| `claude-3-5-sonnet` | $3.00 | $15.00 |
| `claude-3-5-haiku` | $0.80 | $4.00 |
| `gemini-1.5-pro`, `gemini-2.0-pro` | $1.25 | $5.00 |
| `gemini-1.5-flash`, `gemini-2.0-flash` | $0.075 | $0.30 |
| その他（デフォルト単価） | $1.00 | $3.00 |

### 4.3 レスポンスヘッダー仕様
すべてのチャット補完リクエストの応答ヘッダーに、テナントの現在の課金・利用メトリクスが付与される。

| ヘッダー名 | 説明 | 値の例 |
|---|---|---|
| `X-Billing-Type` | テナントの課金プラン | `pay_as_you_go` または `capped` |
| `X-Quota-Limit-Tokens` | 月次上限トークン数 | `1000000` または `unlimited` |
| `X-Quota-Remaining-Tokens` | 当月の残り利用可能トークン数 | `854200` または `unlimited` |
| `X-Monthly-Usage-Tokens` | 当月の累計消費トークン数 | `145800` |
| `X-Monthly-Usage-Cost` | 当月の累計概算コスト (USD) | `0.437400` (小数点以下6桁) |
| `X-Request-ID` | リクエスト追跡用一意識別子 | `4a3b7c2d-98e1-4567-a890-123456789abc` |

---

## 5. 仮想モデルエイリアス仕様
クライアントが特定のベンダー名・モデルバージョンを直接ハードコードせずに利用できるよう、仮想モデルエイリアス解決を提供する。

| 仮想エイリアス | 解決先モデル名 | ターゲットプロバイダー | 主な用途 |
|---|---|---|---|
| `fast` | `gpt-5.4-mini` | Azure OpenAI | 高速応答、低レイテンシ、安価なタスク |
| `smart` | `claude-3-5-sonnet` | Anthropic Claude | 高度な推論、コーディング、複雑な指示追従 |
| `flash` | `gemini-1.5-flash` | Google Gemini | 大規模コンテキスト、マルチモーダル、超高速処理 |

※ クライアントが `gpt-4o` や `claude-3-5-haiku` などの実モデル名を直接指定した場合は、エイリアス変換を行わずそのまま各プロバイダーにルーティングされる。

---

## 6. 日本データレジデンシー仕様
クライアントが `X-Data-Residency: japan` ヘッダーを付与した場合、Microsoft Foundry へのリクエストは自動的に東日本リージョン等の国内限定エンドポイント（環境変数 `MICROSOFT_FOUNDRY_ENDPOINT_JAPAN`）へルーティングされる。
未指定時はグローバルエンドポイント（`MICROSOFT_FOUNDRY_ENDPOINT`）が使用される。

---

## 7. DynamoDB スキーマ仕様 (Single Table Design)

### 7.1 テーブル定義
- **テーブル名**: `LLMGatewayUsage`（環境変数 `DYNAMODB_TABLE_NAME`）
- **パーティションキー (PK)**: `pk` (String)
- **ソートキー (SK)**: `sk` (String)
- **グローバルセカンダリインデックス (GSI)**:
  - **インデックス名**: `GSI_ServiceUsage`
  - **GSI PK**: `service_id` (String)
  - **GSI SK**: `month` (String, `YYYY-MM`)
  - **Projection**: `ALL`
  - **目的**: サービス全体の月次利用実績取得時におけるテーブル全件スキャン (`Scan`) の完全撤廃と高速 `Query` 化。

※ 既存の `LLMGatewayUsage` テーブルをそのまま共用し、テーブルを分割することなく同一テーブル内でクォータ集計、サービス別 API キー管理、および高速インデックス集計を完結する Single Table Design を採用。

### 7.2 エンティティ設計 & レコードパターン

| 用途 | PK | SK | 主な属性 | 説明 |
|---|---|---|---|---|
| **サービス設定 (Master)** | `SERVICE#<service_id>` | `METADATA` | `service_id`, `billing_type`, `cost_limit`, `updated_at` | **【永続】** サービス全体の月次上限設定（Single Source of Truth）。月次予算ガードの基準値。 |
| **テナント月次利用実績** | `SVC#<service_id>#TENANT#<tenant_id>` | `MONTH#<YYYY-MM>` | `total_tokens`, `total_cost`, `models`, `updated_at`, `ttl` | **【月別】** その月の実際のトークン消費量と概算コスト（内訳タグ集計）。 |
| **API キー直接引当** | `KEY#<api_key>` | `METADATA` | `api_key`, `service_id`, `name`, `billing_type`, `cost_limit`, `is_active`, `created_at` | リクエスト認証時の O(1) 高速検証用 |
| **サービス別キー一覧** | `SERVICE#<service_id>` | `KEY#<api_key>` | `api_key`, `service_id`, `name`, `billing_type`, `cost_limit`, `is_active`, `created_at` | サービス配下の API キー一覧クエリ用 |

### 7.3 月次締め切り仕様 (日本標準時 JST 基準)
- **締め切り境界**: **日本時間 (JST: UTC+9) の毎月末日 23:59:59.999999999**
- **月キーの切り替え**:
  - サーバー OS やコンテナのタイムゾーン設定（UTC 等）に依存せず、内部ロジックにおいて常に JST 基準で月文字列 (`YYYY-MM`) を算出。
  - 毎月末日の 23:59:59 までは当月レコード（例: `MONTH#2026-08`）に集計加算。
  - 翌月1日 00:00:00.000 に達した最初のリクエストから自動的に翌月レコード（例: `MONTH#2026-09`）が初期化・集計され、クォータ（上限）判定も当月枠としてクリーンにリセットされる。

### 7.4 アトミック集計更新
リクエスト完了時、リバースプロキシ層から DynamoDB の `UpdateItem` を使用してアトミックに加算される：
```
SET updated_at = :now,
    token_quota = if_not_exists(token_quota, :default_quota),
    billing_type = if_not_exists(billing_type, :default_billing)
ADD total_tokens :tokens,
    total_cost_usd :cost
```
万が一 DynamoDB との通信が失敗した場合は、フォールバックインメモリストアで加算を継続し、Gateway のサービス停止を防ぐ。

---

## 8. API エンドポイント詳細仕様

### 8.1 一般・ドキュメントエンドポイント

#### `GET /health`
- **概要**: ヘルスチェック（ALB / コンテナ死活監視用）
- **認証**: 不要
- **レスポンス**: `{"status":"ok"}` (HTTP 200)

#### `GET /docs`
- **概要**: Scalar API Reference（Huma v2 による完全自動生成モダン API ドキュメント UI、Go サンプルコード優先表示）
- **認証**: 不要
- **レスポンス**: HTML（Scalar UI）

#### `GET /openapi` (および `/openapi.json`)
- **概要**: OpenAPI 3.1 仕様書（Go の構造体・型定義から Huma v2 がランタイム自動生成）
- **認証**: 不要
- **レスポンス**: JSON

---

### 8.2 LLM 中継エンドポイント

#### `POST /v1/chat/completions`
- **概要**: OpenAI 互換チャット補完（ストリーミング SSE 対応）
- **認証**: `Authorization: Bearer <API_KEY>` (必須。発行キー `gw-live-...` または `service:tenant:user` 形式)
- **主要ヘッダー**:
  - `X-Data-Residency`: `japan` (任意)
  - `X-Request-ID`: リクエスト追跡用 ID (任意。省略時は Gateway が自動生成)
  - `traceparent`: W3C 分散トレースコンテキスト (任意。アップストリームへ透過伝播)
- **レスポンス**:
  - `200 OK`: チャット完了レスポンスまたは SSE ストリーム
  - `400 Bad Request`: リクエスト構文不正、またはプロバイダー無効 (`provider_disabled`)
  - `401 Unauthorized`: API キー欠落または無効・失効
  - `429 Too Many Requests`: クォータ上限超過 (`quota_exceeded`)
  - `502 Bad Gateway`: アップストリームプロバイダー通信エラー

#### `GET /v1/realtime`
- **概要**: OpenAI Realtime API (WebSocket) パススルー
- **認証**: `Authorization: Bearer <API_KEY>` (必須)
- **レスポンス**: `101 Switching Protocols`

---

### 8.3 管理用 API (Admin)

#### `GET /v1/admin/usage`
- **概要**: サービスの月次利用実績、モデル別利用内訳、およびテナント別請求内訳 (Showback / Chargeback) の取得
- **認証**: `X-Admin-API-Key: <ADMIN_API_KEY>` または `Authorization: Bearer <ADMIN_API_KEY>` (必須)
- **クエリパラメータ**:
  - `service_id` (必須): 対象サービス ID
  - `month` (任意): 対象月 (`YYYY-MM`)。省略時は当月
- **レスポンス例 (HTTP 200)**:
  ```json
  {
    "service_id": "billing-demo",
    "month": "2026-09",
    "total_tokens": 165000,
    "total_cost_usd": 3.3,
    "cost_limit": 10.0,
    "billing_type": "capped",
    "models": {
      "gpt-4o": { "tokens": 120000, "cost_usd": 2.4 },
      "gpt-4o-mini": { "tokens": 45000, "cost_usd": 0.9 }
    },
    "tenants": {
      "client-alpha": { "tenant_id": "client-alpha", "total_tokens": 120000, "total_cost_usd": 2.4 },
      "client-beta": { "tenant_id": "client-beta", "total_tokens": 45000, "total_cost_usd": 0.9 }
    }
  }
  ```

#### `POST /v1/admin/limits`
- **概要**: サービスの月次コスト上限および課金タイプの更新
- **認証**: `X-Admin-API-Key: <ADMIN_API_KEY>` または `Authorization: Bearer <ADMIN_API_KEY>` (必須)
- **リクエストボディ**:
  ```json
  {
    "service_id": "demo-service",
    "cost_limit": 100.0,
    "billing_type": "capped"
  }
  ```

#### `POST /v1/admin/keys`
- **概要**: サービス向け API キーの発行
- **認証**: `X-Admin-API-Key: <ADMIN_API_KEY>` または `Authorization: Bearer <ADMIN_API_KEY>` (必須)
- **リクエストボディ**:
  ```json
  {
    "service_id": "payment-service",
    "name": "Payment Service Prod Key",
    "billing_type": "pay_as_you_go",
    "cost_limit": 0
  }
  ```
- **レスポンス例 (HTTP 200)**:
  ```json
  {
    "api_key": "gw-live-32298d7249cfeea71174260088f0c3c4",
    "service_id": "payment-service",
    "name": "Payment Service Prod Key",
    "billing_type": "pay_as_you_go",
    "cost_limit": 0,
    "is_active": true,
    "created_at": "2026-09-11T19:27:54.427480241Z",
    "updated_at": "2026-09-11T19:27:54.427480241Z"
  }
  ```

#### `GET /v1/admin/keys`
- **概要**: サービス別 API キー一覧の取得
- **認証**: `X-Admin-API-Key: <ADMIN_API_KEY>` または `Authorization: Bearer <ADMIN_API_KEY>` (必須)
- **クエリパラメータ**:
  - `service_id` (必須): 対象サービス ID
- **レスポンス例 (HTTP 200)**:
  ```json
  {
    "service_id": "payment-service",
    "keys": [
      {
        "api_key": "gw-live-32298d7249cfeea71174260088f0c3c4",
        "service_id": "payment-service",
        "name": "Payment Service Prod Key",
        "billing_type": "pay_as_you_go",
        "is_active": true,
        "created_at": "2026-09-11T19:27:54.427480241Z"
      }
    ]
  }
  ```

#### `DELETE /v1/admin/keys`
- **概要**: API キーの即時失効・無効化
- **認証**: `X-Admin-API-Key: <ADMIN_API_KEY>` または `Authorization: Bearer <ADMIN_API_KEY>` (必須)
- **クエリパラメータ**:
  - `api_key` (必須): 失効対象の API キー (`gw-live-...`)
- **レスポンス例 (HTTP 200)**:
  ```json
  {
    "status": "ok",
    "message": "API key revoked successfully"
  }
  ```

---

## 9. 定期バッチ処理 & 分散ロック仕様 (In-Process Scheduler)

外部の AWS EventBridge や Lambda を使わず、Go サーバープロセス内（ECS コンテナ内）のバックグラウンド Goroutine でバッチ処理を自律実行する。
ECS の複数タスク構成（Auto Scaling）時でも、**DynamoDB 条件付き書き込み（`attribute_not_exists`）による分散ロック** により、同一バッチの二重実行やすり抜けを完全に防止する。

### 9.1 定期ジョブ一覧

| ジョブ名 | 実行スケジュール (JST) | ロックキー形式 | TTL | 処理内容 |
|---|---|---|---|---|
| **月次締めレポート通知** | 毎月1日 00:05 (`5 0 1 * *`) | `monthly_report#<YYYY-MM>` | 30日 | 前月の全サービス・全テナントのトークン消費量・概算コストを集計し、Slack / ログへ通知。 |
| **クォータ残量低下アラート** | 毎時 00分 (`0 * * * *`) | `quota_alert#<YYYY-MM-DD-HH>` | 1時間 | `capped` プランで月次クォータ消費率が 80% / 90% を超過したテナントを検知し、Slack / ログへ警告通知。 |

### 9.2 ロギング仕様 (CloudWatch Logs 統合)
- コンテナ標準出力 (`stdout`) へ非同期チャネル経由で 1 行 1 JSON 形式の構造化ログ（`UsageLogEvent`）を出力。
- ECS / Fargate の `awslogs` ログドライバーにより、追加ミドルウェアなしで自動的に CloudWatch Logs へ安全に集約される。

---

## 10. 環境変数一覧

| 環境変数名 | 必須 | デフォルト値 | 説明 |
|---|---|---|---|
| `PORT` | 任意 | `8080` | Gateway 待受ポート |
| `AWS_REGION` | 任意 | `ap-northeast-1` | AWS リージョン |
| `DYNAMODB_ENDPOINT` | 任意 | 空文字 (ローカル時は `http://dynamodb:8000`) | DynamoDB エンドポイント URL |
| `DYNAMODB_TABLE_NAME` | 任意 | `LLMGatewayUsage` | 利用集計用 DynamoDB テーブル名 |
| `ADMIN_API_KEY` | 任意 | `sk-admin-master-key` | 管理用 API 認証マスターキー |
| `DEFAULT_TOKEN_QUOTA` | 任意 | `1000000` | 新規テナント初期月次トークン上限 |
| `ENABLE_INTERNAL_CRON` | 任意 | `true` | 内蔵バッチスケジューラーの有効化フラグ |
| `SLACK_WEBHOOK_URL` | 任意 | 空文字 | 月次レポート・クォータ警告用 Slack Webhook URL (未設定時はログ出力) |
| `MICROSOFT_FOUNDRY_ENDPOINT` | 任意 | 空文字 | Microsoft Foundry グローバルエンドポイント (旧 `AZURE_OPENAI_ENDPOINT`) |
| `MICROSOFT_FOUNDRY_ENDPOINT_JAPAN` | 任意 | 空文字 | Microsoft Foundry 日本国内限定エンドポイント (旧 `AZURE_OPENAI_ENDPOINT_JAPAN`) |
| `MICROSOFT_FOUNDRY_API_KEY` | 任意 | 空文字 | Microsoft Foundry API キー (旧 `AZURE_OPENAI_API_KEY`) |
| `MICROSOFT_FOUNDRY_API_VERSION` | 任意 | `2024-02-15-preview` | Microsoft Foundry API バージョン (旧 `AZURE_OPENAI_API_VERSION`) |
| `MICROSOFT_FOUNDRY_DEFAULT_DEPLOYMENT`| 任意 | 空文字 | Microsoft Foundry デフォルトデプロイ名 |
| `GEMINI_API_KEY` | 任意 | 空文字 | Google Gemini API キー |

---

## 11. ローカル開発 & 検証環境

### 10.1 サービスエンドポイント一覧

| サービス | URL | 説明 |
|---|---|---|
| **LLM Gateway** | `http://localhost:8081` | API ゲートウェイ本体 |
| **Scalar API Reference** | `http://localhost:8081/docs` | モダン API ドキュメント & テスター UI |
| **OpenAPI 3.1 Spec** | `http://localhost:8081/openapi.json` | OpenAPI 定義 JSON |
| **DynamoDB Admin GUI** | `http://localhost:8001` | DynamoDB Local の GUI ビューアー |
| **DynamoDB Local** | `http://localhost:8002` | ローカル DynamoDB (コンテナ内は 8000) |

### 10.2 起動手順
```bash
cd apps/llm_gateway
nerdctl compose up -d --build
```

### 10.3 代表的な curl コマンド検証

#### 1. ヘルスチェック
```bash
curl http://localhost:8081/health
```

#### 2. チャット補完 (仮想モデル `fast` 指定、レスポンスヘッダー確認)
```bash
curl -i -X POST http://localhost:8081/v1/chat/completions \
  -H "Authorization: Bearer my-app:tenant-a:user-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "fast",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

#### 3. テナント利用状況取得 (Admin API)
```bash
curl -H "X-Admin-API-Key: sk-admin-master-key" \
  "http://localhost:8081/v1/admin/usage?tenant_id=tenant-a"
```

#### 4. テナントプラン変更 (完全従量課金に変更)
```bash
curl -X POST http://localhost:8081/v1/admin/limits \
  -H "X-Admin-API-Key: sk-admin-master-key" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "tenant-a",
    "token_quota": 0,
    "billing_type": "pay_as_you_go"
  }'
```
