# Kura (AWS × Go)

マルチテナント環境における大規模言語モデル（Microsoft Foundry, Google Gemini 等）へのアクセスを一元管理・中継する、超軽量・高パフォーマンスな API ゲートウェイ。
Standard Go Project Layout をベースにしたレイヤードアーキテクチャ（クリーンアーキテクチャ簡略版）を採用。

## 💡 なぜ Kura を作ったのか？（背景と目的）

マルチプロダクトや複数チームが関わる開発において、各アプリケーションが各種 LLM（Microsoft Foundry, Google Gemini 等）を直接呼び出す運用には、以下のような課題が発生します。

1. **コスト・トークン消費のブラックボックス化**:
   どのチームや機能がどれだけコストを消費したかを横断的に追跡・配賦（Showback / Chargeback）することが困難。
2. **過剰請求・リソース枯渇の事故リスク**:
   開発中のループバグや急激なトラフィック急増時に、予算上限でリクエストを自動停止するフェイルセーフが存在しない。
3. **ベンダーロックインと実装の重複**:
   プロバイダー独自の SDK や API 仕様（認証ヘッダー、ストリーミング、新機能パラメータ）にアプリが依存し、モデル移行や新モデル採用のコストが増大。
4. **セキュリティ・ガバナンス統制の欠如**:
   プロバイダーのマスター API キーを開発者やコンテナに直接配布することによる漏洩リスク、高価モデルの無秩序な利用、日本国内データレジデンシー等の企業ポリシーの徹底が困難。
5. **既存の包括的ソリューションとの設計思想の違い（LiteLLM との比較）**:
   LiteLLM に代表される実績豊富な汎用プロキシは、100 以上のプロバイダー対応や高度なフェイルオーバーを備えた非常に優れたオールインワン・ソリューションです。
   一方で、私たちのユースケース（Microsoft Foundry や Gemini 等の特定プロバイダーへの低遅延な中継、社内向けクォータ・キー管理、国内データレジデンシー）に対しては機能が過剰（オーバースペック）であり、設定・運用の認知負荷やコンテナフットプリント（起動速度・メモリ消費）の観点で、より絞り込まれたミニマルなアーキテクチャが求められていました。

### 🎯 本プロジェクトが提供する価値（コア思想）

- **必要最小限に絞り込んだフォーカス設計**:
  あらゆるモデルを網羅するのではなく、本番運用に真に必要なコア機能（OpenAI 互換中継、コストガード、構造化ログ）に特化。API キー管理・認証は前段の API ゲートウェイ（tollgate）に委譲し、コードベース全体を見通し良く保ち、誰でも内部ロジックを完全に把握・拡張できる高い保守性を実現。
- **超軽量・極低レイテンシー (Go × Alpine)**:
  CGO なしの Go 実装。数MBの Alpine コンテナで瞬時に起動し、最小の CPU/メモリ使用量とほぼゼロに近いプロキシオーバーヘッドを実現。
- **OpenAI 互換規格によるインターフェース統一**:
  クライアントは使い慣れた単一エンドポイント (`/v1/chat/completions`) を叩くだけ。仮想モデルエイリアス (`fast`, `smart`, `flash`) により、アプリ側のコード変更なしで裏側のモデルを柔軟に切り替え。
- **確実なコストガード & マルチテナント制御**:
  Amazon DynamoDB（Single Table Design）によるミリ秒単位のリアルタイム集計。予算上限到達時の即時自動遮断 (`429 Too Many Requests`)、動的レートリミット（RPM制御）。
- **完全自律型運用（外部バッチ不要）**:
  EventBridge や Lambda を必要とせず、プロセス内 Goroutine と DynamoDB 分散ロックにより月次締めレポート集計・アラート通知を完結。

---

## 主な機能
- **OpenAI 互換エンドポイント (`/v1/chat/completions`)**: 単一のインターフェースで全ベンダーにアクセス。
- **tollgate 連携 & マルチテナント解決**: API キー管理を tollgate に集約し、`X-Service-ID` 等のヘッダーにより透過的にテナントコンテキスト・予算を解決。
- **仮想モデルエイリアス (Routing & Fallback)**: `fast` / `default` (`gpt-5.4-mini`), `smart` / `code` (`claude-3-5-sonnet`), `flash` (`gemini-1.5-flash`) などの共通エイリアス解決。
- **動的レートリミット（RPM制御）**: テナント/サービス単位のオンデマンドな分間リクエスト制御（ノイジーネイバー防止）。超過時は即座に 429 Too Many Requests を返却。
- **透過メタデータ・タグ収集**: `X-Environment`, `X-Feature`, `X-Tags` ヘッダーから事前マスタなしで透過的に収集・利用ログへ付与。
- **SSE ストリーミング制御**: バッファリング完全無効化、最終フレームでの正確なトークン計測。
- **未知パラメータのパススルー (FR-06)**: 各ベンダーの新機能パラメータ（`thinking` 等）をそのまま透過。
- **非同期利用量ログ**: メイン処理をブロックせず、標準出力へ JSON 構造化ログを出力（CloudWatch Logs / Firehose 互換）。
- **リアルタイム通信**: `/v1/realtime` の WebSocket パススルー。
- **統合 LLM モックサーバー**: ローカル開発・CI検証用の Microsoft Foundry & Google AI Studio スタブサーバーを内包。

---

## ディレクトリ構造

```
kura/
├── cmd/
│   ├── server/                     # Gateway 本体エントリーポイント
│   └── mock_server/                # Microsoft Foundry & AI Studio 統合モックサーバー
├── internal/
│   ├── domain/                     # ドメイン層（エンティティ・インターフェース）
│   │   ├── entity/                 # Chat, Tenant, Pricing, Error, Usage 等
│   │   ├── repository/             # QuotaRepository インターフェース
│   │   └── service/                # Adapter, UsageLogger, RateLimiter インターフェース
│   ├── usecase/                    # ユースケース層（業務ロジック）
│   │   ├── auth_usecase.go         # テナント解決・タグ抽出・クォータ上限判定 (429)
│   │   ├── chat_usecase.go         # 仮想モデル名解決・モデルアクセス認可 (403)
│   │   └── admin_usecase.go        # 管理用 API・クォータ設定

│   ├── infrastructure/             # インフラ層（外部連携・具象実装）
│   │   ├── adapter/                # OpenAI / Microsoft Foundry アダプター
│   │   ├── config/                 # 環境変数ローダー
│   │   ├── dynamodb/               # DynamoDB クォータ永続化 (インメモリフォールバック対応)
│   │   ├── logger/                 # 非同期構造化コンソールロガー
│   │   ├── proxy/                  # リバースプロキシ・SSE・Usageインターセプト
│   │   ├── ratelimit/              # インメモリ・スライディングウィンドウ・レートリミッター
│   │   └── websocket/              # WebSocket パススルー
│   └── delivery/                   # プレゼンテーション層
│       └── http/
│           ├── api.go              # Huma v2 OpenAPI 3.1 & Scalar ドキュメント自動生成
│           ├── handler.go          # HTTP ハンドラ (/health, /v1/chat/completions, /v1/realtime)
│           ├── admin_handler.go    # 管理用 API ハンドラ (/v1/admin/*)
│           ├── middleware.go       # 認証・レート制限・コンテキスト付与ミドルウェア
│           └── response.go         # レスポンスヘルパー
├── docs/                           # システム仕様書群 (細分化ドキュメント)
├── Dockerfile                      # Gateway 本体マルチステージビルド (Alpine)
├── Dockerfile.mock                 # モックサーバー用マルチステージビルド
├── compose.yaml                    # ローカル検証用 (Gateway + Nginx[ELBシミュレータ] + Mock + DynamoDB)
├── nginx.conf                      # ローカル用 AWS ELB (ALB) シミュレータ設定 (WebSocket/SSE対応)
└── go.mod
```

## 📖 仕様書・詳細ドキュメント

詳細な設計仕様は [docs/](docs/SPECIFICATION.md) 配下に細分化して管理しています。各エンドポイントの入出力スキーマやコードサンプルは [Scalar API ドキュメント](http://localhost:8088/docs) を参照してください。

- **[全体システム仕様書 (目次)](docs/SPECIFICATION.md)**
- **[アーキテクチャ & レイヤー設計](docs/architecture.md)**: 全体構成、Standard Go Layout、データレジデンシー、未知パラメータ透過
- **[認証 & バーチャルキー仕様](docs/auth-and-virtual-keys.md)**: モデル制限、有効期限、動的レートリミット (RPM 制御)
- **[課金モデル & クォータ制御](docs/billing-and-quota.md)**: PayG / Capped プラン、単価マスタ、リアルタイム集計、レスポンスヘッダー
- **[DynamoDB スキーマ仕様](docs/dynamodb-schema.md)**: Single Table Design、アトミック集計加算、JST 締め切り仕様
- **[スケジューラー & オブザーバビリティ](docs/scheduler-and-observability.md)**: 内蔵 Cron、分散ロック、非同期構造化ログ、Slack通知
- **[API リファレンス概要](docs/api-reference.md)**: Scalar / OpenAPI 案内とエンドポイントサマリ一覧

---

## ローカル起動手順

### 1. 単体起動 (Go 環境)
```bash
cd kura
go run ./cmd/server
```
※ DynamoDB が未起動の場合、自動的にインメモリストアにフォールバックして単体起動します。

### 2. Docker / nerdctl での起動 (推奨)
```bash
# 全体 (Gateway + Nginx[ローカルELB] + Mock + DynamoDB) 起動
nerdctl compose up -d --build
```
- **Gateway (Nginx / ローカル ELB 経由)**: `http://localhost:8088`
- **Scalar API ドキュメント (Go サンプル付き)**: `http://localhost:8088/docs`
- **OpenAPI 3.1 仕様書**: `http://localhost:8088/openapi.json`
- **統合 LLM モックサーバー**: `http://localhost:8090`

---

## 環境変数一覧

| 環境変数 | 説明 | デフォルト値 |
| :--- | :--- | :--- |
| `PORT` | サーバー待受ポート | `8080` |
| `AWS_REGION` | AWS リージョン | `ap-northeast-1` |
| `ADMIN_API_KEY` | 管理者用マスターキー | 空 (未設定時は全拒否) |
| `DEFAULT_TOKEN_QUOTA` | 初期テナントの月間トークン上限 | `1000000` |
| `RATE_LIMIT_RPM` | 1分あたりの最大リクエスト数 (0で無制限) | `600` |
| `DOCS_PATH` | Scalar API ドキュメント UI パス (空文字で無効化) | `/docs` |
| `OPENAPI_PATH` | OpenAPI 3.1 仕様書エンドポイント パス (空文字で無効化) | `/openapi` |
| `DYNAMODB_ENDPOINT` | DynamoDB エンドポイント (ローカル: `http://dynamodb:8000`) | 空 (AWS デフォルト) |
| `DYNAMODB_TABLE_NAME` | DynamoDB 利用量テーブル名 | `KuraUsage` |
| `MICROSOFT_FOUNDRY_ENDPOINT` | Microsoft Foundry ベース URL (旧 `AZURE_OPENAI_ENDPOINT`) | 空 |
| `MICROSOFT_FOUNDRY_API_KEY` | Microsoft Foundry API キー (旧 `AZURE_OPENAI_API_KEY`) | 空 |
| `MICROSOFT_FOUNDRY_API_VERSION`| Microsoft Foundry API バージョン (旧 `AZURE_OPENAI_API_VERSION`) | `2024-02-15-preview` |
| `MICROSOFT_FOUNDRY_ENDPOINT_JAPAN` | 日本国内限定エンドポイント (X-Data-Residency: japan 指定時) | 空 |
| `GEMINI_API_KEY` | Google Gemini API キー | 空 |
| `GEMINI_BASE_URL` | Google Gemini ベース URL | `https://generativelanguage.googleapis.com` |

---

## API リクエスト例

### 1. ヘルスチェック
```bash
curl -i http://localhost:8080/health
```

### 2. チャット補完 (メタデータ・タグ付与)
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-internal-team-alpha" \
  -H "X-Environment: staging" \
  -H "X-Feature: rag-search" \
  -H "X-Tags: team=infra,experiment=v1" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.4-mini",
    "messages": [{"role": "user", "content": "こんにちは"}],
    "stream": true
  }'
```

### 3. テナントクォータ・課金プランの設定
```bash
curl -X POST http://localhost:8080/v1/admin/limits \
  -H "X-Admin-API-Key: sk-admin-master-key" \
  -H "Content-Type: application/json" \
  -d '{
    "service_id": "payment-service",
    "tenant_id": "tenant-corp-a",
    "cost_limit": 50.0,
    "billing_type": "capped"
  }'
```

---

## CI / CD (GitHub Actions)

`.github/workflows/docker-publish.yml` により、以下のトリガーで **GitHub Container Registry (`ghcr.io`)** への自動ビルド・Push が行われます。

- **トリガー**:
  - セマンティックバージョニングタグ（例: `v1.0.0`）の Push、または手動実行 (`workflow_dispatch`)
- **パブリッシュ先**: `ghcr.io/<owner>/kura:<tag>`
- **マルチアーキテクチャ対応**: `linux/amd64`, `linux/arm64`
- **認証**: リポジトリ標準の `GITHUB_TOKEN`（`packages: write` 権限）を使用するため、個別の外部 Secret 登録は不要。

---

## ライセンス

本プロジェクトは [MIT License](LICENSE) のもとで公開されています。
