# 認証 & テナント解決仕様書

## 1. 概要
Kura では、クライアント認証および API キー管理を前段の API ゲートウェイ（**tollgate**）に委譲している。
Kura 自身は各プロバイダー（Microsoft Foundry, Gemini 等）への安全なルーティング、予算・クォータガード、動的レートリミット、トークンメータリングに専念する。

---

## 2. 認証 & テナント解決方式

クライアントからのリクエスト認証は tollgate で検証された後、以下のヘッダーを介してテナントコンテキスト（`TenantContext`）が解決される。

### 2.1 tollgate 連携ヘッダー（推奨）
tollgate が認証後に付与するヘッダーからサービス・テナント情報を解決する。

| ヘッダー | 説明 | 例 |
|---|---|---|
| `X-Service-ID` | 呼び出し元サービス識別子（予算クォータ単位） | `payment-service` |
| `X-Tenant-ID` | テナント・組織識別子（省略時は ServiceID と同値） | `tenant-corp-a` |
| `X-User-ID` | エンドユーザー識別子（監査ログ用） | `user-12345` |

### 2.2 3階層識別子 / Bearer 互換（後方互換・社内基盤連携）
tollgate 前段なしの直接通信や後方互換用として、`Authorization: Bearer <TOKEN>` による解決もサポートする。

```
<ServiceID>:<TenantID>:<UserID>
```
- 例: `finance-app:dept-risk-01:user-4521`
  - `ServiceID`: 呼び出し元アプリケーション・サービス識別子
  - `TenantID`: 課金・クォータ単位となるテナント識別子（DynamoDB の PK）
  - `UserID`: エンドユーザー識別子（監査ログ用）
- 単一キー（例: `tenant-alpha`）が渡された場合、キー全体を `ServiceID` および `TenantID` とみなして処理する。
- どちらのヘッダーも指定されない場合、デフォルトテナント（`anonymous`）として解決される。

### 2.3 管理者マスターキー認証
管理用 API（`/v1/internal/*`）へのアクセスには、マスターキー認証が必要。
- ヘッダー: `X-Admin-API-Key: <ADMIN_API_KEY>` または `Authorization: Bearer <ADMIN_API_KEY>`
- 環境変数 `ADMIN_API_KEY`（デフォルト: `sk-admin-master-key`）と照合し、不一致時は **HTTP 401 Unauthorized** を返却。

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
   - `POST /v1/internal/limits`（`tenant_id` 省略時）で設定。
   - サービス配下の全テナントの月次消費合計が `cost_limit` を超過した場合、即座に **HTTP 429 Too Many Requests** (`quota_exceeded`) を返却。
2. **テナント個別上限**:
   - `POST /v1/internal/limits`（`tenant_id` 指定時）で設定。
   - サービス全体の上限に達していなくても、特定テナントの当月利用額がテナント個別の `cost_limit` を超過した場合、そのテナントのリクエストのみ **HTTP 429 Too Many Requests** (`quota_exceeded`) で遮断。
3. **優先順位**:
   - サービス全体上限とテナント個別上限の双方が設定されている場合、**いずれか一方でも上限に達した時点でリクエストがブロック**される。
