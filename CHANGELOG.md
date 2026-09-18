# Release v0.0.1

Kura の初回公式リリースです。  
マルチテナント環境における大規模言語モデルへのアクセスを一元管理・中継する、超軽量・高パフォーマンスな API ゲートウェイを提供します。

---

## 🚀 主な機能 (Key Features)

### 1. OpenAI 互換推論インターフェース
- **統一エンドポイント (`/v1/chat/completions`)**: 単一のインターフェースで Microsoft Foundry や Google Gemini などのマルチプロバイダーに透過アクセス。
- **仮想モデルエイリアス (Model Alias Routing)**:
  - `fast` / `default` → `gpt-5.4-mini`
  - `smart` / `code` → `claude-3-5-sonnet`
  - `flash` → `gemini-1.5-flash`
- **リアルタイム SSE ストリーミング**:
  - `stream: true` 時のバッファリング完全無効化 (`X-Accel-Buffering: no`)。
  - プロバイダーから返却される最終チャンクの公式トークン情報 (`usage`) を透過インターセプト。
- **未知パラメータのパススルー (FR-06)**: 各ベンダーの新機能・独自パラメータ（`thinking` 等）をスキーマ変更なしで透過。
- **日本データレジデンシー対応**: `X-Data-Residency: japan` ヘッダー付与時、国内限定エンドポイントへ自動ルーティング。

### 2. バーチャル API キー & セキュリティ管理
- **社内向けバーチャルキー発行 (`/v1/internal/keys`)**:
  - キー単位での利用許可モデル制限 (`allowed_models`、`*` ワイルドカード対応)。
  - 有効期限 (`expires_at`) 設定。
  - 即時失効機能 (`DELETE` によるアクセス遮断)。
- **動的レートリミット (RPM 制御)**:
  - テナント/キー単位のスライディングウィンドウ方式による分間リクエスト上限制御 (超過時 `429 Too Many Requests`)。
- **透過メタデータ・タグ収集**:
  - `X-Environment`, `X-Feature`, `X-Tags` ヘッダーから事前マスタ定義なしで透過収集し、利用ログへ自動付与。

### 3. 利用量計測 & コスト管理 (Amazon DynamoDB)
- **Single Table Design**: 高スループットな月次トークン消費量および推定利用コスト (USD) のリアルタイム永続化。
- **柔軟な課金プラン**:
  - `pay_as_you_go` (完全従量課金)
  - `capped` (コスト上限設定、上限到達時の安全遮断)
- **定期バッチ & 通知機能**:
  - 月次利用実績レポート自動集計、クォータ警告通知の生成。

### 4. 開発者体験 (DX) & 運用基盤
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
