# API リファレンス & ドキュメント仕様

## 1. 概要 & 対話型ドキュメント

Kura では、API 定義の二重管理・ドキュメントの陳腐化を防ぐため、Go の構造体定義から **Huma v2** が OpenAPI 3.1 仕様書および Scalar ドキュメント UI をランタイム自動生成する方式を採用している。

詳細なリクエスト/レスポンススキーマ、ステータスコード、および各言語のコードサンプル（Go Native サンプル等）は、以下の対話型 UI で確認可能。

| ドキュメント | アクセス先 URL | 説明 |
|---|---|---|
| ドキュメント | アクセス先 URL | 説明 |
|---|---|---|
| **Scalar API ドキュメント** | `http://localhost:8088/docs` | ブラウザから直接 API テストが可能なモダン ドキュメント UI |
| **OpenAPI 3.1 仕様書 (JSON)** | `http://localhost:8088/openapi.json` | クライアント SDK 自動生成やスキーマ検証用 JSON |

> **Note**: 
> - **Scalar ドキュメント UI**: 環境変数 `DOCS_PATH`（デフォルト: `/docs`）で配信パスの変更や無効化が可能。`DOCS_PATH=""`（空文字）または `off` / `none` に設定すると無効化される。
> - **OpenAPI 3.1 仕様書**: 環境変数 `OPENAPI_PATH`（デフォルト: `/openapi`）でスキーマエンドポイントのパス変更や無効化が可能。`OPENAPI_PATH=""`（空文字）または `off` / `none` に設定すると OpenAPI 仕様書エンドポイントが無効化される（OpenAPI が無効化された場合は Scalar UI も連動して無効化される）。

---

## 2. エンドポイント一覧 (サマリ)

### 2.1 システム & ヘルスチェック
| パス | メソッド | 認証 | 概要 |
|---|:---:|:---:|---|
| `/healthz` | `GET` | 不要 | **総合ヘルスチェック**: ゲートウェイの稼働状態確認 |
| `/livez` | `GET` | 不要 | **Liveness プローブ**: プロセス死活監視 (外部依存なし、高速 200 返却) |
| `/readyz` | `GET` | 不要 | **Readiness プローブ**: トラフィック受入監視 (DynamoDB 疎通・Graceful Shutdown 検知) |
| `/metrics` | `GET` | 不要 | **Prometheus メトリクス**: リクエスト数、レイテンシー、TTFT、トークン消費量、推定コスト、429拒絶数、稼働プロセス統計 |

### 2.2 推論・中継 API (OpenAI 互換)
| パス | メソッド | 認証 | 概要 |
|---|:---:|:---:|---|
| `/v1/chat/completions` | `POST` | Bearer キー | OpenAI 互換チャット補完 (非ストリーミング & SSE ストリーミング) |
| `/v1/realtime` | `GET` | Bearer キー | OpenAI Realtime API (WebSocket) パススルー |

### 2.3 サービス・テナント向け自己照会 API
サービス（クライアント）が自身の API キーや識別子を用いて、当月の累計消費量、予算上限、残り予算枠、許可モデル、有効期限をリアルタイムに自己照会するエンドポイント。

| パス | メソッド | 認証 | 概要 |
|---|:---:|:---:|---|
| `/v1/usage` | `GET` | X-Service-ID または Bearer | サービス別月次使用量・リアルタイム残枠確認 |

**レスポンス例 (`200 OK`)**:
```json
{
  "service_id": "payment-service",
  "tenant_id": "tenant-corp-a",
  "month": "2026-09",
  "billing_type": "capped",
  "service_cost_limit_usd": 100.0,
  "service_total_cost_usd": 25.5,
  "service_remaining_cost_usd": 74.5,
  "tenant_cost_limit_usd": 50.0,
  "tenant_remaining_cost_usd": 24.5,
  "total_tokens": 150000,
  "prompt_tokens": 100000,
  "completion_tokens": 50000,
  "allowed_models": ["gpt-4o", "claude-3-5-sonnet-20241022"],
  "is_quota_exceeded": false
}
```
> [!NOTE]
> `billing_type` が `pay_as_you_go` の場合、予算上限なしのため `service_remaining_cost_usd` は `-1` が返却される。テナント個別上限が未設定の場合は `tenant_cost_limit_usd` は `0`、`tenant_remaining_cost_usd` は `-1` となる。

### 2.4 管理用 API (Admin)
マスター API キー（`X-Admin-API-Key` または `Authorization: Bearer <ADMIN_API_KEY>`）による認証が必要。

| パス | メソッド | 概要 |
|---|:---:|---|
| `/v1/admin/limits` | `POST` | サービス全体またはテナント個別の月次コスト上限 (`cost_limit`) & プラン設定 |
| `/v1/admin/usage` | `GET` | サービス別月次トークン消費量・概算コスト・モデル別内訳レポート取得 |
| `/v1/admin/jobs/run` | `POST` | 定期バッチジョブ (`monthly_report`, `quota_alerts`) の手動即時実行 |
| `/v1/admin/notifications` | `GET` | Gateway 内部に蓄積された通知・アラート一覧取得 |

#### `/v1/admin/limits` リクエスト例:
```json
{
  "service_id": "payment-service",
  "tenant_id": "tenant-corp-a",
  "cost_limit": 50.0,
  "billing_type": "capped"
}
```
※ `tenant_id` を省略した場合はサービス全体の上限が設定され、指定した場合は該当テナント個別の上限が設定される。

