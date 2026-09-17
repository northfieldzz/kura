# 認証 & バーチャルキー仕様書

## 1. 概要
LLM Gateway では、各クライアントやサービスが各プロバイダー（Microsoft Foundry, Gemini 等）の生 API キーを直接保持することを禁止し、Gateway が発行・管理する「バーチャルキー」または「3階層識別子」による安全なアクセス制御を提供する。

---

## 2. 認証方式

Gateway へのリクエストは `Authorization: Bearer <TOKEN>` ヘッダーで行う。以下の 2 つの認証方式に対応している。

### 2.1 バーチャルキー認証（推奨）
Gateway の管理 API（`/api/v1/llm/internal/keys`）で発行された一意の API キー（フォーマット: `gw-live-<32文字hex>`）。

- **許可モデル制限 (`allowed_models`)**:
  - キーごとに利用可能なモデルのホワイトリストを設定可能。
  - ワイルドカード対応（例: `["gpt-5.4-mini", "gemini-*", "fast"]`）。
  - リスト外のモデルを指定された場合、即座に **HTTP 403 Forbidden** (`model_not_allowed`) でリクエストを遮断する。
- **有効期限 (`expires_at`)**:
  - RFC 3339 形式の日時（例: `2026-12-31T23:59:59Z`）を設定可能。期限切れのキーは即座に **HTTP 401 Unauthorized** で遮断。
- **即時失効機能**:
  - キー漏洩時やプロジェクト終了時、管理者 API 経由で即時に無効化 (`is_active = false`) が可能。

### 2.2 3階層識別子認証（後方互換・社内基盤連携）
社内サービス間通信などで、事前に API キーを発行せず透過的に利用・集計するためのコロン区切りフォーマット。

```
<ServiceID>:<TenantID>:<UserID>
```
- 例: `finance-app:dept-risk-01:user-4521`
  - `ServiceID`: 呼び出し元アプリケーション・サービス識別子
  - `TenantID`: 課金・クォータ単位となるテナント識別子（DynamoDB の PK）
  - `UserID`: エンドユーザー識別子（監査ログ用）
- 後方互換性: コロンを含まない単一キー（例: `tenant-alpha`）が渡された場合、キー全体を `TenantID` とみなし、`ServiceID`・`UserID` は空文字として処理する。

### 2.3 管理者マスターキー認証
管理用 API（`/api/v1/llm/internal/*`）へのアクセスには、マスターキー認証が必要。
- ヘッダー: `X-Admin-API-Key: <ADMIN_API_KEY>` または `Authorization: Bearer <ADMIN_API_KEY>`
- 環境変数 `ADMIN_API_KEY`（デフォルト: `sk-admin-master-key`）と照合し、不一致時は **HTTP 401 Unauthorized** を返却。

---

## 3. 動的レートリミット (RPM 制御)

ノイジーネイバー（特定キーやテナントによるリクエスト殺到による他サービスへの影響）を防止するため、Gateway 内部で高パフォーマンスなインメモリ・スライディングウィンドウ式レートリミッターを備える。

- **制御単位**: テナント識別子（または API キー）単位
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
