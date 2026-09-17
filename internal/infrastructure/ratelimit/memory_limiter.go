package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/northfieldzz/kura/internal/domain/service"
)

type memoryRateLimiter struct {
	mu         sync.Mutex
	rpmLimit   int
	windows    map[string][]time.Time
	lastCleanup time.Time
}

// NewMemoryRateLimiter はインメモリのスライディングウィンドウ・レートリミッターを生成する
func NewMemoryRateLimiter(rpmLimit int) service.RateLimiter {
	return &memoryRateLimiter{
		rpmLimit:    rpmLimit,
		windows:     make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}
}

func (l *memoryRateLimiter) Allow(ctx context.Context, key string) (bool, int, time.Duration, int, error) {
	if l.rpmLimit <= 0 {
		return true, 999999, 0, 0, nil // 上限未設定 (無制限)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	// 定期的なメモリクリーンアップ (5分毎に期限切れエントリをパージ)
	if now.Sub(l.lastCleanup) > 5*time.Minute {
		for k, timestamps := range l.windows {
			valid := false
			for _, ts := range timestamps {
				if ts.After(oneMinuteAgo) {
					valid = true
					break
				}
			}
			if !valid {
				delete(l.windows, k)
			}
		}
		l.lastCleanup = now
	}

	timestamps := l.windows[key]
	// 1分以上経過した過去のタイムスタンプを除外
	validIdx := 0
	for i, ts := range timestamps {
		if ts.After(oneMinuteAgo) {
			validIdx = i
			break
		}
		if i == len(timestamps)-1 {
			validIdx = len(timestamps)
		}
	}
	timestamps = timestamps[validIdx:]

	count := len(timestamps)
	if count >= l.rpmLimit {
		// 最も古い有効タイムスタンプから1分後までの残り待機時間
		oldest := timestamps[0]
		retryAfter := oldest.Add(1 * time.Minute).Sub(now)
		if retryAfter < 0 {
			retryAfter = 1 * time.Second
		}
		l.windows[key] = timestamps
		return false, 0, retryAfter, l.rpmLimit, nil
	}

	// リクエスト許可: 現在時刻を追加
	timestamps = append(timestamps, now)
	l.windows[key] = timestamps
	remaining := l.rpmLimit - len(timestamps)

	return true, remaining, 0, l.rpmLimit, nil
}
