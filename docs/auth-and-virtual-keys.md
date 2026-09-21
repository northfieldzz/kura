# 認証 & テナント解決仕様書

## 1. 概要
Kura では、クライアント認証および API キー管理を前段の API ゲートウェイ（**tollgate**）に委譲している。
Kura 自身は各プロバイダー（Microsoft Foundry, Gemini 等）への安全なルーティング、予算・クォータガード、動的レートリミット、トークンメータリングに専念する。

---

## 2. 認証 & テナント解決方式

クライアントからのリクエスト認証は tollgate で検証された後、以下のヘッダーを介してテナントコンテキスト（`TenantContext`）が解決される。

### 2.1 tollgate 連携ヘッダー（推奨）
tollgate が認証後に付与するヘッダーからサービス・テナント情報を解決する。トレーサビリティ担保のため、暗黙のデフォルト値へのフォールバックは行わない。

| ヘッダー | 必須性 (Tollgate) | 説明 | 例 |
|---|---|---|---|
| `X-Service-ID` | **必須** | 呼び出し元サービス識別子。APIキーに紐づくサービス名。欠落時は `400 Bad Request` (`missing_service_id`) で即時拒絶。 | `payment-service` |
| `X-Tenant-ID` | **必須** | テナント・組織識別子（データ分離・クォータ単位）。欠落時は `400 Bad Request` (`missing_tenant_id`) で即時拒絶。 | `tenant-corp-a` |
| `X-Key-ID` | **必須** | 使用された API キーの UUID（監査ログ・キー別追跡用）。 | `550e8400-e29b-41d4-a716-446655440000` |
| `X-Key-Prefix` | 任意 | API キーの先頭プレフィックス（ログ・調査用）。 | `tlge-live-8f9c` |
| `X-User-ID` | 任意 | エンドユーザー識別子（監査ログ用）。 | `user-12345` |

### 2.2 動作モード（環境変数）

| 環境変数 | 型 / デフォルト | 説明 |
|---|---|---|
| `ENFORCE_TOLLGATE_AUTH` | bool (`false`) | `true` の場合、Tollgate の API キー識別ヘッダー（`X-Key-ID`）が欠落しているリクエストを `401 Unauthorized` (`missing_tollgate_headers`) で拒絶する。 |

- **`is_proxied` 判定**:
  `X-Key-ID` が付与されている場合、Tollgate プロキシ経由のリクエストと判定され、コンテキスト (`TenantContext`) および利用ログ (`UsageLogEvent`) の `is_proxied` が `true` となる。

### 2.3 管理者マスターキー認証
管理用 API（`/v1/admin/*`）へのアクセスには、マスターキー認証が必要。
- ヘッダー: `Authorization: Bearer <ADMIN_API_KEY>`
- 環境変数 `ADMIN_API_KEY`（デフォルト: 空文字列）と照合し、不一致時や未設定時は **HTTP 401 Unauthorized** を返却（未設定時は全てのアクセスが拒絶される）。

---

## 3. 動的レートリミット (RPM 制御)

ノイジーネイバー（特定キーやテナントによるリクエスト殺到による他サービスへの影響）を防止するため、Gateway 内部で高パフォーマンスなインメモリ・スライディングウィンドウ式レートリミッターを備える。

- **制御単位**: サービス・テナント識別子単位 (`ServiceID` または `TenantID`)
- **デフォルト上限**: 環境変数 `RATE_LIMIT_RPM`（デフォルト: 600 req/min、0 で無制限）
- **上限超過時の挙動**: プロバイダーへリクエストを転送せず、即座に **HTTP 429 Too Many Requests** (`rate_limit_exceeded`) を返却。
- **レスポンスヘッダー**:
  - `X-RateLimit-Limit-RPM`: 設定された分間上限数
  - `X-RateLimit-Remaining-RPM`: 当分内の残り可能リクエスト数

---

## 4. 透過メタデータ・タグ収集

クライアントはリクエストヘッダーに任意のタグを付与することで、事前のマスタ登録なしに利用ログや CloudWatch Logs へメタデータを紐付けることができる。

| ヘッダー | 説明 | 例 |
|---|---|---|
| `X-Environment` | 実行環境識別子 | `production`, `staging`, `dev` |
| `X-Feature` | ユースケース・機能名 | `rag-search`, `summarize`, `chatbot` |
| `X-Tags` | カンマ区切りのキー・バリュー | `team=infra,experiment=v2,priority=high` |
| `X-Request-ID` | リクエスト追跡用 UUID | `4a3b7c2d-98e1-4567-a890-123456789abc` (未指定時は自動採番) |
| `traceparent` | W3C 分散トレーシングヘッダー | アップストリームへ透過伝播 |

---

## 5. 予算・クォータガード仕様

Kura では、過度な LLM API コストの発生を防止するため、月次コスト（USD）ベースのクォータガードを提供する。

### 5.1 階層構造
1. **サービス全体上限**:
   - `POST /v1/admin/limits`（`tenant_id` 省略時）で設定。
   - サービス配下の全テナントの月次消費合計が `cost_limit` を超過した場合、即座に **HTTP 429 Too Many Requests** (`quota_exceeded`) を返却。
2. **テナント個別上限**:
   - `POST /v1/admin/limits`（`tenant_id` 指定時）で設定。
   - サービス全体の上限に達していなくても、特定テナントの当月利用額がテナント個別の `cost_limit` を超過した場合、そのテナントのリクエストのみ **HTTP 429 Too Many Requests** (`quota_exceeded`) で遮断。
3. **優先順位**:
   - サービス全体上限とテナント個別上限の双方が設定されている場合、**いずれか一方でも上限に達した時点でリクエストがブロック**される。
