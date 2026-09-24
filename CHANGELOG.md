# Changelog

All notable changes to this project will be documented in this file.

## [v1.0.0] - 2026-09-24

### ⚠️ Breaking Changes (破壊的変更)
- **ゲートウェイ共有シークレットによる信頼確認の必須化**: テナント識別ヘッダー（`X-Tenant-ID` 等）の偽装を防ぐため、サービスエンドポイントへのリクエストに対して `X-Gateway-Secret`（環境変数 `GATEWAY_SHARED_SECRET`、最低 32 文字）の検証を標準で必須化した。未設定時は起動を拒否する（開発時のみ `INSECURE_NO_GATEWAY_AUTH=true` でバイパス可能）。
- **RPM (分間リクエスト制限) の削除**: プロセス内メモリ依存を排除しマルチコンテナスケールを可能にするため、RPM リミッターおよび環境変数 `RATE_LIMIT_RPM`、Prometheus のレートリミットメトリクスを Kura から完全に排除した。RPM 制御は前段の認証ゲートウェイ（Tollgate 等）の責務となる。
- **ストアアーキテクチャの役割分離**: 単一の `QuotaRepository` をホットパス用の `CostStore`（残枠確認・アトミック加算）と永続用の `UsageStore`（利用実績記録・月次集計・分散ロック）に分離した。
- **環境変数の刷新**: `GATEWAY_SHARED_SECRET`, `GATEWAY_SHARED_SECRET_PREVIOUS`, `GATEWAY_SECRET_HEADER`, `INSECURE_NO_GATEWAY_AUTH`, `COST_STORE`, `USAGE_STORE`, `VALKEY_URL`, `POSTGRES_DSN`, `PRICING_FILE`, `UNKNOWN_MODEL_POLICY`, `BEDROCK_API_KEY`, `BEDROCK_REGION`, `BEDROCK_ENDPOINT` を追加した。
- **ストア既定値の変更**: ストレージ未指定時のデフォルトバックエンドを `memory` から `sqlite` に変更した。外部サービスなしで起動した場合でも、再起動後に使用量・設定が永続化される。
- **`memory` 設定値の廃止**: `COST_STORE=memory` および `USAGE_STORE=memory` の本番設定を廃止した。環境変数で `memory` が指定された場合は、エラーで起動を拒絶し `sqlite` の利用を促すメッセージを出力する（インメモリ実装はテストヘルパー専用に移行）。

### 🚀 New Features (新機能)
- **ゲートウェイ共有シークレット検証 & ゼロダウンタイムローテーション**:
  - `crypto/subtle.ConstantTimeCompare` による定数時間検証。
  - `GATEWAY_SHARED_SECRET_PREVIOUS` による新旧シークレットの並行受け付け（無停止ローテーション）。
  - ヘッダー名カスタマイズ (`GATEWAY_SECRET_HEADER`)。
  - シークレット漏洩防止（ログ・メトリクス・上流 LLM プロバイダー転送から完全除外）。
  - Prometheus メトリクス `kura_gateway_auth_failures_total` の追加。
- **マルチバックエンド対応**:
  - `CostStore`: DynamoDB, Valkey / Redis, PostgreSQL (非推奨), SQLite
  - `UsageStore`: DynamoDB, PostgreSQL, SQLite
  - 不正なストレージ組み合わせの起動時検証 (Fail-Fast) および単一インスタンス専用ストア（SQLite）利用時の警告出力を追加。
- **集計補正 (Reconciliation) エンジン**:
  - Valkey / Redis を `CostStore` として使用する際、フェイルオーバー等でカウンタが失われても `UsageStore` の正の実績データから残高を再構築・同期する補正機能を実装。
  - 定期バックグラウンド補正および手動実行 API (`POST /v1/admin/jobs/reconcile`) を提供。
- **JSON 単価表エンジン (`pricing.json`)**:
  - モデル別単価（入力・出力・キャッシュ入力・推論トークン）を JSON ファイルで外出し管理。
  - 適用日・バージョン追跡 (`version`) を利用実績ログに記録。
  - 未定義モデル呼び出し時のポリシー (`UNKNOWN_MODEL_POLICY=warn|reject`) 設定に対応。
- **Amazon Bedrock (bedrock-mantle) アダプター**:
  - Bedrock の OpenAI 互換エンドポイント (`bedrock-mantle.{region}.api.aws`) への直接ルーティングを実装。
  - Bedrock API キーによる認証、SSE ストリーミング最終フレームからのトークン使用量取得、コスト計算に対応。
- **Docker Compose プロファイル拡張**:
  - `compose.yaml` に `valkey` および `postgres` プロファイルを追加。
- **SQLite バックエンド（単一ノード標準ストレージ）**:
  - `CostStore` と `UsageStore` の両方を単一の SQLite ファイルで提供。
  - CGO 不要の純 Go 実装（`modernc.org/sqlite`）を採用し、`CGO_ENABLED=0` の静的バイナリおよび distroless/Alpine コンテナイメージを維持。
  - WAL モード、`busy_timeout=5000`、単一ライター直列化による安全な同時実行制御。
  - 環境変数 `SQLITE_PATH`（既定: `./data/kura.db`）でファイルパスを指定可能。
- **キャッシュ層（デコレーター）**:
  - 各バックエンド（DynamoDB, PostgreSQL, Valkey/Redis）の前段に配置するインメモリキャッシュデコレーターを実装（SQLite では自動バイパス）。
  - **遮断済みテナントのネガティブキャッシュ**: 予算超過判定されたテナントをローカルメモリに記録し、以降のリクエストをストアアクセスなしで即座に拒絶（TTL: 当月末時刻と最大 TTL の短い方、Admin API 操作時に即時無効化）。
  - **テナント/サービス設定の読み取りキャッシュ**: 短い TTL（既定: 60秒）で設定をローカルキャッシュ。
  - **残枠キャッシュ & 加算バッチ書き込み**: オプションで残枠読み取りキャッシュおよび加算のバッファリングを提供（既定オフ）。グレースフルシャットダウン時に必ずバッファをフラッシュ。
  - **可観測性の拡充**: Prometheus メトリクスにキャッシュヒット/ミス数、ネガティブ遮断数、フラッシュ回数、バッファ中未反映額を追加。
- **Docker Compose 最短起動の改善**:
  - 既定で SQLite バックエンドとデータボリューム（`kura-data:/data`）を使用し、外部サービスなしで `docker compose up` だけで永続化付き最短起動が可能に。

---

## [v0.0.1] - 2026-09-24

### 🚀 New Features (新機能)
- **OpenAI 互換推論インターフェース**
  - 統一エンドポイント (`/v1/chat/completions`)**: 単一のインターフェースで Microsoft Foundry や Google Gemini などのマルチプロバイダーに透過アクセス。
  - **仮想モデルエイリアス (Model Alias Routing)**:
    -   `fast` / `default` → `gpt-5.4-mini`
    -   `smart` / `code` → `claude-3-5-sonnet`
    -   `flash` → `gemini-1.5-flash`
  - **リアルタイム SSE ストリーミング**:
    - `stream: true` 時のバッファリング完全無効化 (`X-Accel-Buffering: no`)。
    - プロバイダーから返却される最終チャンクの公式トークン情報 (`usage`) を透過インターセプト。
  - **未知パラメータのパススルー (FR-06)**: 各ベンダーの新機能・独自パラメータ（`thinking` 等）をスキーマ変更なしで透過。
  - **日本データレジデンシー対応**: `X-Data-Residency: japan` ヘッダー付与時、国内限定エンドポイントへ自動ルーティング。

- **バーチャル API キー & セキュリティ管理**
  - **社内向けバーチャルキー発行 (`/v1/admin/keys`)**:
    - キー単位での利用許可モデル制限 (`allowed_models`、`*` ワイルドカード対応)。
    - 有効期限 (`expires_at`) 設定。
    - 即時失効機能 (`DELETE` によるアクセス遮断)。
  - **動的レートリミット (RPM 制御)**:
    - テナント/キー単位のスライディングウィンドウ方式による分間リクエスト上限制御 (超過時 `429 Too Many Requests`)。
  - **透過メタデータ・タグ収集**:
    - `X-Environment`, `X-Feature`, `X-Tags` ヘッダーから事前マスタ定義なしで透過収集し、利用ログへ自動付与。

- **利用量計測 & コスト管理 (Amazon DynamoDB)**
  - **Single Table Design**: 高スループットな月次トークン消費量および推定利用コスト (USD) のリアルタイム永続化。
  - **柔軟な課金プラン**:
    - `pay_as_you_go` (完全従量課金)
    - `capped` (コスト上限設定、上限到達時の安全遮断)
  - **定期バッチ & 通知機能**:
    - 月次利用実績レポート自動集計、クォータ警告通知の生成。

- **開発者体験 (DX) & 運用基盤**
  - **OpenAPI 3.1 & Scalar ドキュメント自動生成**:
    - `/docs` (Go サンプルコード付き Scalar UI)
    - `/openapi.json` (OpenAPI 3.1 スキーマ)
  - **統合 LLM モックサーバー (`cmd/mock_server`)**:
    - 外部 API 契約なしでローカル開発・CI 検証が完結するスタブサーバーを内包。
  - **コンテナ最適化 & CI/CD**:
    - Docker / nerdctl 対応（Alpine ベースの最小マルチステージビルド）。
    - GitHub Actions CI (自動テスト、タグ Push 時のマルチアーキテクチャ Docker Hub パブリッシュ)。

---

## 📦 リリースに含まれるコンポーネント
- **Kura コアエンジン**: `cmd/server/main.go`
- **統合モックサーバー**: `cmd/mock_server/main.go`
- **DynamoDB ローカル検証環境**: `dynamodb.compose.yaml`
- **Docker Compose 定義**: `compose.yaml` (専用ネットワーク `kura_network` 構成)
