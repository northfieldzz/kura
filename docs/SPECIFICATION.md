# LLM API Gateway システム仕様書 (SPECIFICATION)

本ドキュメントは、LLM API Gateway の全体仕様書のインデックスです。  
詳細な技術仕様は、責務ごとに以下の各ドキュメントへ細分化されています。
各エンドポイントの詳細なリクエスト/レスポンススキーマは、自動生成される **[Scalar API ドキュメント](http://localhost:8088/api/v1/llm/docs)** を参照してください。

---

## 📚 詳細仕様ドキュメント一覧

1. **[アーキテクチャ & システム設計](architecture.md)**
   - 全体アーキテクチャ構成図
   - レイヤー設計 (Standard Go Project Layout / Clean Architecture)
   - 仮想モデルエイリアス解決 (`fast`, `smart`, `flash`)
   - 日本データレジデンシー仕様 (`X-Data-Residency: japan`)
   - 未知パラメータの透過パススルー (FR-06: `thinking` 等)

2. **[認証 & バーチャルキー仕様](auth-and-virtual-keys.md)**
   - バーチャル API キー管理（モデル制限 `allowed_models`、有効期限、即時失効）
   - 3階層識別子認証 (`<ServiceID>:<TenantID>:<UserID>`)
   - 動的レートリミット（RPM 制御 / 429 Too Many Requests）
   - 透過メタデータ・タグ収集 (`X-Environment`, `X-Feature`, `X-Tags`)

3. **[課金モデル & クォータ制御](billing-and-quota.md)**
   - 課金プラン種別 (`pay_as_you_go` / `capped`)
   - コスト計算式とモデル別標準単価テーブル
   - クォータ判定・予算ガード
   - レスポンスヘッダー仕様 (`X-Billing-Type`, `X-Quota-*`, `X-Monthly-Usage-*`)

4. **[DynamoDB スキーマ & 永続化仕様](dynamodb-schema.md)**
   - Single Table Design 設計方針
   - テーブル定義 & グローバルセカンダリインデックス (`GSI_ServiceUsage`)
   - エンティティ設計 & レコードパターン
   - アトミック集計加算 (Atomic Counter) & インメモリフォールバック
   - JST 基準の月次締め切り境界仕様

5. **[スケジューラー & オブザーバビリティ](scheduler-and-observability.md)**
   - 内蔵バッチスケジューラー (`CronScheduler`)
   - DynamoDB 条件付き書き込みによる分散ロック制御
   - 月次レポートバッチ & 予算残量低下アラート
   - 非同期構造化ロガー (`UsageLogEvent`) & 分散トレース伝播

6. **[API リファレンス概要](api-reference.md)**
   - Scalar API ドキュメント UI (`/api/v1/llm/docs`)
   - OpenAPI 3.1 仕様書 (`/api/v1/llm/openapi.json`)
   - カテゴリ別エンドポイントサマリ一覧
