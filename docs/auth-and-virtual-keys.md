# 認証 & テナント解決仕様書

## 1. 概要
Kura では、エンドユーザーの認証および API キー管理を前段の認証ゲートウェイ（Tollgate, Kong, 自前プロキシ等）に委譲している。
Kura 自身は「リクエストが信頼するゲートウェイから送信されたこと」を**ゲートウェイ共有シークレット**で確認し、各プロバイダー（Microsoft Foundry, Gemini, Bedrock 等）への安全なルーティング、予算・クォータガード、トークンメータリングに専念する。
分間リクエスト制限（RPM）などのレート制限は前段ゲートウェイの責務として分離している。

---

## 2. ゲートウェイ共有シークレットによる信頼確認

同一ネットワーク内の別ワークロードによるヘッダー偽装を防ぐため、共有シークレットによる相互信頼確認を提供する。

### 2.1 仕様
- **ヘッダー名**: 既定は `X-Gateway-Secret`（環境変数 `GATEWAY_SECRET_HEADER` で変更可能）。
- **検証方式**: `crypto/subtle.ConstantTimeCompare` による定数時間比較（タイミング攻撃防止）。
- **起動時検証**: `GATEWAY_SHARED_SECRET` が未設定または 32 文字未満の場合は起動を拒絶する（Fail-Fast）。
- **開発用バイパス**: 開発・検証時に限り、`INSECURE_NO_GATEWAY_AUTH=true` を明示設定することでシークレット検証をバイパス可能（起動時に警告ログを出力）。
- **無停止ローテーション**: `GATEWAY_SHARED_SECRET_PREVIOUS` を設定することで、新旧両方のシークレットを並行して受け付ける期間を設けられる。

---

## 3. テナントコンテキスト解決

ゲートウェイによる認証後、以下の HTTP ヘッダーからテナントコンテキスト（`TenantContext`）が解決される。

| ヘッダー | 必須性 | 説明 | 例 |
|---|---|---|---|
| `X-Service-ID` | **必須** | 呼び出し元サービス識別子。欠落時は `400 Bad Request` (`missing_service_id`) で即時拒絶。 | `payment-service` |
| `X-Tenant-ID` | **必須** | テナント・組織識別子（クォータ・データ分離単位）。欠落時は `400 Bad Request` (`missing_tenant_id`) で即時拒絶。 | `tenant-corp-a` |
| `X-Key-ID` | 任意 | 使用された API キーの UUID（監査ログ・キー別追跡用）。 | `550e8400-e29b-41d4-a716-446655440000` |
| `X-Key-Prefix` | 任意 | API キーの先頭プレフィックス（ログ・調査用）。 | `tlge-live-8f9c` |
| `X-User-ID` | 任意 | エンドユーザー識別子（監査ログ用）。 | `user-12345` |

### 3.1 動作モード（環境変数）

| 環境変数 | 型 / デフォルト | 説明 |
|---|---|---|
| `ENFORCE_TOLLGATE_AUTH` | bool (`false`) | `true` の場合、API キー識別ヘッダー（`X-Key-ID`）が欠落しているリクエストを `401 Unauthorized` (`missing_tollgate_headers`) で拒絶する。 |

---

## 4. 管理者マスターキー認証

管理用 API（`/v1/admin/*`）へのアクセスには、マスターキー認証が必要。
- ヘッダー: `Authorization: Bearer <ADMIN_API_KEY>`
- 環境変数 `ADMIN_API_KEY` と `crypto/subtle.ConstantTimeCompare` で定数時間照合し、不一致時や未設定時は **HTTP 401 Unauthorized** を返却（未設定時は全ての管理アクセスが遮断される）。

---

## 5. 透過メタデータ・タグ収集

クライアントはリクエストヘッダーに任意のタグを付与することで、事前のマスタ登録なしに利用ログや構造化ログへメタデータを紐付けることができる。

| ヘッダー | 説明 | 例 |
|---|---|---|
| `X-Environment` | 実行環境識別子 | `production`, `staging`, `dev` |
| `X-Feature` | ユースケース・機能名 | `rag-search`, `summarize`, `chatbot` |
| `X-Tags` | カンマ区切りのキー・バリュー | `team=infra,experiment=v2,priority=high` |
| `X-Request-ID` | リクエスト追跡用 UUID | `4a3b7c2d-98e1-4567-a890-123456789abc` (未指定時は自動採番) |
| `traceparent` | W3C 分散トレーシングヘッダー | アップストリームへ透過伝播 |

---

## 6. 予算・クォータガード仕様

Kura では、過度な LLM API コストの発生を防止するため、月次コスト（USD）ベースのクォータガードを提供する。

### 6.1 階層構造
1. **サービス全体上限**:
   - `POST /v1/admin/limits`（`tenant_id` 省略時）で設定。
   - サービス配下の全テナントの月次消費合計が `cost_limit` を超過した場合、以降のリクエストを **HTTP 429 Too Many Requests** (`quota_exceeded`) で遮断。
2. **テナント個別上限**:
   - `POST /v1/admin/limits`（`tenant_id` 指定時）で設定。
   - サービス全体の上限に達していなくても、特定テナントの当月利用額がテナント個別の `cost_limit` を超過した場合、そのテナントのリクエストのみ **HTTP 429 Too Many Requests** (`quota_exceeded`) で遮断。
3. **優先順位**:
   - サービス全体上限とテナント個別上限の双方が設定されている場合、**いずれか一方でも上限に達した時点でリクエストがブロック**される。
