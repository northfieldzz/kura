# 課金モデル & クォータ制御仕様書

## 1. 概要
Kura は、各テナント・サービスが消費したトークン数（Prompt / Completion / Cached / Reasoning）をリアルタイムで追跡し、外部単価表ファイル（`pricing.json`）に基づいて概算利用コスト（USD）を自動計算する。
月次予算上限を超過した際の自動遮断（クォータガード）を提供し、予期せぬ高額請求を確実に防ぐ。

---

## 2. 課金プラン種別 (`BillingType`)

サービス・テナント単位で以下の 2 種類の課金プランを設定可能。

| プラン (`billing_type`) | 上限設定 (`cost_limit`) | 挙動 |
|---|---|---|
| **`pay_as_you_go`**<br>(完全従量課金) | 任意 (0 または上限値) | コスト・トークン集計はリアルタイムで行われるが、上限超過によるリクエスト遮断は発生しない。 |
| **`capped`**<br>(予算上限設定プラン) | 正の実数 (例: `50.0` USD) | 当月の累計利用コストが `cost_limit`（または月次トークン上限 `token_quota`）に達している場合、次回以降のリクエストを **HTTP 429 Quota Exceeded** で即時遮断する。 |

※ 新規テナントの初回利用時は、環境変数 `DEFAULT_TOKEN_QUOTA`（デフォルト: 1,000,000 トークン）を上限とする `capped` プランとして自動初期化される。

---

## 3. コスト計算 & 料金表 (`pricing.json`)

リクエスト完了時に捕捉したトークン数（ストリーミング時は最終チャンクの公式 `usage`）に基づき、外部 JSON 単価表（`pricing.json`）からコストを自動算出する。

### 3.1 単価項目 (USD / 1M tokens)
- `input_cost`: 通常のプロンプト入力単価
- `output_cost`: 補完出力単価
- `cached_input_cost`: プロンプトキャッシュヒット時の割引単価（未指定時は `input_cost` へ自動フォールバック）
- `reasoning_cost`: 推論（Thinking）トークン単価（未指定時は `output_cost` へ自動フォールバック）

### 3.2 計算式
$$\text{Cost (USD)} = \frac{\text{PromptTokens} \times \text{InputPrice} + \text{CompletionTokens} \times \text{OutputPrice} + \text{CachedTokens} \times \text{CachedPrice} + \text{ReasoningTokens} \times \text{ReasoningPrice}}{1,000,000}$$

### 3.3 未定義モデルポリシー (`UNKNOWN_MODEL_POLICY`)
- `warn`（デフォルト）: 警告ログを出力し、フォールバック単価（Input: $1.00 / Output: $3.00 per 1M）を適用して集計を継続。
- `reject`: `400 Bad Request` で即座にリクエストを拒絶。

---

## 4. レスポンスヘッダー仕様

チャット補完リクエストの HTTP レスポンスヘッダーに、テナントのリアルタイムな利用状況・クォータ残量が付与される。

| ヘッダー名 | 説明 | 値の例 |
|---|---|---|
| `X-Billing-Type` | テナントの課金プラン | `pay_as_you_go` または `capped` |
| `X-Quota-Limit-Tokens` | 月次上限トークン数 | `1000000` または `unlimited` |
| `X-Quota-Remaining-Tokens` | 当月の残り利用可能トークン数 | `854200` または `unlimited` |
| `X-Monthly-Usage-Tokens` | 当月の累計消費トークン数 | `145800` |
| `X-Monthly-Usage-Cost` | 当月の累計概算コスト (USD) | `$0.44` |
| `X-Request-ID` | リクエスト一意識別子 (UUID) | `4a3b7c2d-98e1-4567-a890-123456789abc` |
