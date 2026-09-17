# アーキテクチャ & システム設計仕様書

## 1. システム概要 & アーキテクチャ構成

本システムは、マルチテナント環境における大規模言語モデル（LLM）へのアクセスを一元管理・中継する超軽量・高パフォーマンスな API ゲートウェイである。
OpenAI 互換のインターフェースを提供し、Microsoft Foundry (旧 Azure AI Foundry / OpenAI) や Google Gemini 等のマルチプロバイダーへ低遅延でルーティングする。
本番インフラでは前段に **AWS ALB (Application Load Balancer / ELB)** が配置され、ローカル開発環境（Docker Compose）では **Nginx** を用いてこの ELB のルーティングやバッファリング制御をシミュレートする。

```
                       +-----------------------------+
                       |      Client Application     |
                       +-----------------------------+
                                      |
                                      | HTTP / WebSocket
                                      v
                       +-----------------------------+
                       |        AWS ALB (ELB)        |
                       |  (Local: Nginx Simulation)  |
                       +-----------------------------+
                                      |
                                      v
       +-------------------------------------------------------------+
       |                         Kura                                |
       |  - テナント解決 (tollgate 連携) & モデル認可 (403 Forbidden)|
       |  - 仮想モデルエイリアス解決 (fast, smart, flash)            |
       |  - 動的レートリミット (RPM 制御: 429 Too Many Requests)     |
       |  - 予算・クォータガード (PayG / Capped)                     |
       |  - 日本データレジデンシールーター (X-Data-Residency: japan) |
       |  - 未知パラメータ透過パススルー (FR-06: thinking 等)       |
       |  - SSE ストリーミング制御 (X-Accel-Buffering: no)           |
       |  - 非同期構造化ログ (UsageLogEvent) & 分散トレース伝播      |
       +-------------------------------------------------------------+
              |                                      |
              v                                      v
       +--------------------+                 +--------------------+
       |  Microsoft Foundry |                 |   Google Gemini    |
       | (GPT / Claude 等)  |                 |  (AI Studio 互換)  |
       +--------------------+                 +--------------------+
              ^                                      ^
              |               Token Metering         |
              +----------------------+---------------+
                                     v
                       +---------------------------+
                       |      Amazon DynamoDB      |
                       |   Table: KuraUsage        |
                       +---------------------------+
```

---

## 2. レイヤー設計 (Standard Go Project Layout)

クリーンアーキテクチャの原則に準拠し、依存関係が内側（ドメイン層）に向かうレイヤード構造を採用している。

```
kura/
├── cmd/
│   ├── server/                     # Gateway 本体エントリーポイント
│   └── mock_server/                # Microsoft Foundry & AI Studio 統合モックサーバー
├── internal/
│   ├── domain/                     # 【ドメイン層】外部依存を持たない純粋な業務ルール・モデル
│   │   ├── entity/                 # Chat, Tenant, Pricing, Usage, Error
│   │   ├── repository/             # QuotaRepository インターフェース
│   │   └── service/                # Adapter, UsageLogger, RateLimiter インターフェース
│   ├── usecase/                    # 【ユースケース層】ドメインを組み合わせた業務シナリオ
│   │   ├── auth_usecase.go         # テナント認証/解決・タグ抽出・クォータ上限判定
│   │   ├── chat_usecase.go         # 仮想モデル名解決・モデルアクセス認可
│   │   ├── admin_usecase.go        # 管理用 API・クォータ設定/利用量取得
│   │   └── batch_usecase.go        # 月次締めレポート・残量低下アラート
│   ├── infrastructure/             # 【インフラ層】外部技術・フレームワークの具象実装
│   │   ├── adapter/                # Microsoft Foundry (OpenAI 互換) アダプター
│   │   ├── config/                 # 環境変数ローダー
│   │   ├── dynamodb/               # DynamoDB クォータ永続化 (インメモリフォールバック対応)
│   │   ├── logger/                 # 非同期チャネル構造化コンソールロガー
│   │   ├── notifier/               # アプリ内通知・Slack 通知アダプター
│   │   ├── proxy/                  # リバースプロキシ・SSE・Usageインターセプト
│   │   ├── ratelimit/              # インメモリ・スライディングウィンドウ・レートリミッター
│   │   ├── scheduler/              # JST 内蔵 Cron スケジューラー
│   │   └── websocket/              # Realtime API WebSocket パススルー
│   └── delivery/                   # 【プレゼンテーション層】入出力アダプター
│       └── http/
│           ├── api.go              # Huma v2 OpenAPI 3.1 & Scalar ドキュメント自動生成
│           ├── handler.go          # HTTP ハンドラ (/health, /v1/chat/completions, /v1/realtime)
│           ├── admin_handler.go    # 管理用 API ハンドラ (/api/v1/llm/internal/*)
│           ├── middleware.go       # 認証・レート制限・コンテキスト付与ミドルウェア
│           ├── cors.go             # CORS 設定ミドルウェア
│           └── response.go         # レスポンスヘルパー
└── docs/                           # 詳細仕様ドキュメント群
```

---

## 3. ルーティング & プロバイダー解決

### 3.1 仮想モデルエイリアス (Model Alias Routing)
クライアント側が特定のベンダー名・モデルバージョンを直接ハードコードせずに利用できるよう、仮想エイリアスによる解決を提供する。

| 仮想エイリアス | 解決先モデル名 | ターゲットプロバイダー | 主な用途 |
|---|---|---|---|
| `fast`, `default` | `gpt-5.4-mini` | Microsoft Foundry | 高速応答、低レイテンシ、低コストタスク |
| `smart`, `code` | `claude-3-5-sonnet`| Microsoft Foundry | 高度な推論、コーディング、複雑な指示追従 |
| `flash` | `gemini-1.5-flash` | Google Gemini | 大規模コンテキスト、マルチモーダル、超高速処理 |

※ クライアントが実モデル名（例: `gpt-4o`, `claude-3-5-haiku` 等）を直接指定した場合は、エイリアス変換を行わずそのまま各プロバイダーにルーティングされる。

### 3.2 日本データレジデンシー仕様 (Japan Data Residency)
クライアントが `X-Data-Residency: japan` ヘッダーを付与した場合、Microsoft Foundry へのリクエストは自動的に東日本リージョン等の国内限定エンドポイント（環境変数 `MICROSOFT_FOUNDRY_ENDPOINT_JAPAN`）へルーティングされる。
未指定時はグローバルエンドポイント（`MICROSOFT_FOUNDRY_ENDPOINT`）が使用される。

### 3.3 未知パラメータの透過 (FR-06)
各ベンダーの新機能パラメータ（例: OpenAI の `thinking: { type: "enabled", budget_tokens: 1024 }` や Anthropic の独自フィールド等）を受信した場合、Gateway は構造体定義外のキーを `ExtraFields` として保持し、プロバイダー向けリクエスト JSON へ無変換で復元・透過する。
これにより、プロバイダーの新機能追加時に Gateway のコード改修を待たずに即時利用できる。

---

## 4. パフォーマンス & メモリアロケーション検証 (ベンチマーク)

Kura では、大量のリクエストおよび長時間の SSE ストリーミング中継において Go ランタイムの GC（ガベージコレクション）負荷を最小限に抑えるため、ホットパスにおける**ゼロアロケーション（Zero Allocation）**および省メモリ設計を徹底している。

### 4.1 ベンチマーク実測結果 (`go test -bench=. -benchmem`)

AMD Ryzen 9 / Linux コンテナ環境における実測結果：

| レイヤー / 対象 | ベンチマーク項目 | 実行時間 (ns/op) | メモリ消費 (B/op) | アロケーション (allocs/op) | 最適化内容 & 特徴 |
|---|---|:---:|:---:|:---:|---|
| **Domain (Entity)** | `CalculateCost` | 14.8 ns | 0 B | 0 | 料金マップ検索・コスト計算の完全ゼロアロケーション |
| | `ResolveModelAlias` | 7.5 ns | 0 B | 0 | モデル名エイリアス解決のゼロアロケーション |
| | `ValidateAllowedModel` | 135.0 ns | 0 B | 0 | ワイルドカード・プレフィックス認可判定のゼロアロケーション |
| **Adapter (SSE Parse)** | `ExtractUsageFromChunk_WithoutUsage` | **52.8 ns** | **0 B** | **0** | **99%のSSEチャンクを `bytes.Contains` でゼロコピー早期判定（最適化前: 979ns/497B/4allocs から20倍高速化 & アロケーション完全排除）** |
| | `ExtractUsageFromChunk_WithUsage` | 976.4 ns | 120 B | 2 | 最終チャンクの Usage 抽出（最適化前 489B/4allocs からメモリ消費 75% 削減） |
| | `ExtractUsageFromChunk_Done` | 6.4 ns | 0 B | 0 | `[DONE]` 終端判定のゼロアロケーション |
| **Metrics (Prometheus)** | `RecordRequest` | 179.4 ns | 3 B | 1 | リクエスト毎のカウンタ・レイテンシヒストグラム記録 |
| | `RecordTokens` | 281.2 ns | 0 B | 0 | トークン消費・推定コスト集計のゼロアロケーション |
| | `RecordTTFT` | 39.8 ns | 0 B | 0 | 初速トークン生成時間 (TTFT) 記録のゼロアロケーション |
| **Proxy (Streaming)** | `StreamingProxy_ServeForward` (50 chunks) | 235.3 µs | 27.9 KB | 220 | 50チャンク中継時の全体所要時間。1チャンクあたり **わずか 4.7 µs / 4 allocs** の超低オーバーヘッド |
| **Usecase (Auth & Tags)** | `ParseTagsHeader_Empty` | **2.2 ns** | **0 B** | **0** | タグ未指定時のファストパス（最適化前 42ns/64B/2allocs からゼロアロケーション化） |
| | `ParseTagsHeader` (複数タグ) | 338.3 ns | 528 B | 7 | スライス長に応じた map 事前キャパシティ確保 |
| | `AuthenticateRequest` | 750.7 ns | 832 B | 12 | Bearer抽出、DynamoDB/メモリ照合、Context 生成を含む |

### 4.2 実行方法
```bash
# 全ホットパスのベンチマーク実行
go test -bench=. -benchmem -run=^# ./internal/...
```

