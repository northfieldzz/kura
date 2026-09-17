# API リファレンス & ドキュメント仕様

## 1. 概要 & 対話型ドキュメント

LLM Gateway では、API 定義の二重管理・ドキュメントの陳腐化を防ぐため、Go の構造体定義から **Huma v2** が OpenAPI 3.1 仕様書および Scalar ドキュメント UI をランタイム自動生成する方式を採用している。

詳細なリクエスト/レスポンススキーマ、ステータスコード、および各言語のコードサンプル（Go Native サンプル等）は、以下の対話型 UI で確認可能。

| ドキュメント | アクセス先 URL | 説明 |
|---|---|---|
| **Scalar API ドキュメント** | `http://localhost:8088/api/v1/llm/docs` | ブラウザから直接 API テストが可能なモダン ドキュメント UI |
| **OpenAPI 3.1 仕様書 (JSON)** | `http://localhost:8088/api/v1/llm/openapi.json` | クライアント SDK 自動生成やスキーマ検証用 JSON |

---

## 2. エンドポイント一覧 (サマリ)

### 2.1 システム & ヘルスチェック
| パス | メソッド | 認証 | 概要 |
|---|:---:|:---:|---|
| `/health` | `GET` | 不要 | ゲートウェイ稼働状態確認 (ALB / 死活監視用) |
| `/api/llm/health` | `GET` | 不要 | Huma v2 形式ヘルスチェック |

### 2.2 推論・中継 API (OpenAI 互換)
| パス | メソッド | 認証 | 概要 |
|---|:---:|:---:|---|
| `/v1/chat/completions` | `POST` | Bearer キー | OpenAI 互換チャット補完 (非ストリーミング & SSE ストリーミング) |
| `/api/v1/llm/chat/completions` | `POST` | Bearer キー | 同上 (Scalar / OpenAPI 公開用ルート) |
| `/v1/realtime` | `GET` | Bearer キー | OpenAI Realtime API (WebSocket) パススルー |
| `/api/v1/llm/realtime` | `GET` | Bearer キー | 同上 (Scalar / OpenAPI 公開用ルート) |

### 2.3 管理用 API (Internal)
マスター API キー（`X-Admin-API-Key` または `Authorization: Bearer <ADMIN_API_KEY>`）による認証が必要。

| パス | メソッド | 概要 |
|---|:---:|---|
| `/api/v1/llm/internal/keys` | `POST` | バーチャル API キーの新規発行 (許可モデル・有効期限付き) |
| `/api/v1/llm/internal/keys` | `GET` | サービス識別子による発行済み API キー一覧取得 |
| `/api/v1/llm/internal/keys` | `DELETE` | 指定 API キーの即時無効化・失効 |
| `/api/v1/llm/internal/limits` | `POST` | サービスの月次コスト上限 (`cost_limit`) & プラン (`capped` / `pay_as_you_go`) 設定 |
| `/api/v1/llm/internal/usage` | `GET` | サービス別月次トークン消費量・概算コスト・モデル別内訳レポート取得 |
| `/api/v1/llm/internal/jobs/run` | `POST` | 定期バッチジョブ (`monthly_report`, `quota_alerts`) の手動即時実行 |
| `/api/v1/llm/internal/notifications` | `GET` | Gateway 内部に蓄積された通知・アラート一覧取得 |
