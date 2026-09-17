package service

import (
	"context"
	"time"
)

// RateLimiter はオンデマンドなレート制限（RPM / TPM 等）を検証するインターフェース
type RateLimiter interface {
	// Allow は指定キーのアクセスを許可するか判定し、残り枠と次回リセットまでの時間を返す
	// allowed: リクエストを許可するかどうか
	// remaining: 当該ウィンドウ内の残りリクエスト可能数
	// retryAfter: レート超過時の待機推奨時間 (0なら超過なし)
	// limit: 設定された上限値
	Allow(ctx context.Context, key string) (allowed bool, remaining int, retryAfter time.Duration, limit int, err error)
}
