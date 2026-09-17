# DynamoDB スキーマ & 永続化仕様書

## 1. 概要
LLM API Gateway は、超高並行なリクエスト環境下でもボトルネックを作らず、低レイテンシー・高スループットを維持するため、Amazon DynamoDB の **Single Table Design** を採用している。
複数テーブルの JOIN や高負荷なテーブル全件スキャン (`Scan`) を完全に排除し、すべての認証・クォータ判定・集計クエリを O(1) または効率的な `Query` で完結させる。

---

## 2. テーブル & インデックス定義

- **テーブル名**: `LLMGatewayUsage`（環境変数 `DYNAMODB_TABLE_NAME`）
- **パーティションキー (PK)**: `pk` (String)
- **ソートキー (SK)**: `sk` (String)
- **課金モード**: オンデマンド (`PAY_PER_REQUEST`)
- **グローバルセカンダリインデックス (GSI)**:
  - **インデックス名**: `GSI_ServiceUsage`
  - **GSI PK**: `service_id` (String)
  - **GSI SK**: `month` (String, フォーマット: `YYYY-MM`)
  - **Projection**: `ALL`
  - **用途**: サービス別月次利用実績レポート取得時、全件スキャンを行わずに対象サービス・対象月の全レコードを高速ピンポイント抽出する。

---

## 3. エンティティ設計 & レコードパターン

単一テーブル内で以下の 4 つの用途のレコードを共存管理する。

| 用途 | PK | SK | 主な属性 | 説明 |
|---|---|---|---|---|
| **サービス設定 (Master)** | `SERVICE#<service_id>` | `METADATA` | `service_id`, `billing_type`, `cost_limit`, `updated_at` | サービス全体の上限・課金プラン設定（Single Source of Truth）。 |
| **テナント月次利用実績** | `SVC#<service_id>#TENANT#<tenant_id>` | `MONTH#<YYYY-MM>` | `total_tokens`, `total_cost_usd`, `models`, `updated_at`, `ttl` | その月のトークン消費量と概算コスト（モデル別内訳含む）。 |
| **API キー直接引当** | `KEY#<api_key>` | `METADATA` | `api_key`, `service_id`, `name`, `billing_type`, `cost_limit`, `allowed_models`, `expires_at`, `is_active`, `created_at` | リクエスト認証時の O(1) 高速検証用レコード。 |
| **サービス別キー一覧** | `SERVICE#<service_id>` | `KEY#<api_key>` | `api_key`, `service_id`, `name`, `billing_type`, `cost_limit`, `is_active`, `created_at` | サービス配下に発行された全 API キーの高速一覧クエリ用。 |
| **アプリ内通知** | `NOTIFICATIONS` | `NOTIFICATION#<timestamp>#<id>` | `id`, `type`, `title`, `message`, `is_alert`, `created_at` | 月次レポートや予算アラートなどの通知ログ。 |

---

## 4. アトミック集計加算 (Atomic Counter)

リクエスト完了時（ストリーミング時は全チャンク送信完了時）、リバースプロキシ層から DynamoDB の `UpdateItem` を使用してアトミックに数値を加算する。これにより、複数インスタンス（ECS タスク）からの並行アクセスでもレースコンディションを起こさず、正確な合算値が維持される。

```
SET updated_at = :now,
    billing_type = if_not_exists(billing_type, :default_billing)
ADD total_tokens :tokens,
    total_cost_usd :cost
```

### インメモリフォールバック
万が一 DynamoDB のネットワーク障害やスロットリングが発生した場合でも、Gateway はダウンせず、自動的に**インメモリフォールバックストア**へ切り替えて集計を継続し、可用性（Availability）を最優先する。

---

## 5. 月次締め切り仕様 (日本標準時 JST 基準)

- **締め切り境界**: **毎月末日 23:59:59.999999999 (JST: UTC+9)**
- **月キー (`YYYY-MM`) の自動算出**:
  - サーバー OS やコンテナのタイムゾーン（UTC 等）に依存せず、内部ロジックにおいて常に JST 基準で月文字列を決定。
  - 月末 23:59:59 までは当月レコード（例: `MONTH#2026-09`）に加算。
  - 翌月1日 00:00:00 に達した最初のリクエストから自動的に翌月レコード（例: `MONTH#2026-10`）が生成され、クォータ判定も新月枠としてリセットされる。
