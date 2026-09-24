package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics は Prometheus メトリクス定義を保持する
type Metrics struct {
	Registry                    *prometheus.Registry
	RequestsTotal               *prometheus.CounterVec
	TokensTotal                 *prometheus.CounterVec
	RequestDuration             *prometheus.HistogramVec
	TimeToFirstToken            *prometheus.HistogramVec
	EstimatedCostTotal          *prometheus.CounterVec
	ActiveRequests              prometheus.Gauge
	CacheHitsTotal              *prometheus.CounterVec
	CacheMissesTotal            *prometheus.CounterVec
	CacheNegativeRejectionsTotal *prometheus.CounterVec
	CacheFlushCountTotal        prometheus.Counter
	CacheBufferedCostUSD        prometheus.Gauge
}

// NewMetrics は Prometheus メトリクスおよび標準コレクターを初期化・登録する
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	// Go ランタイムおよびプロセスメトリクス (メモリ、GC、Goroutine 数等) の自動登録
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		Registry: registry,

		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kura",
				Name:      "requests_total",
				Help:      "Total number of HTTP / WebSocket requests handled by Kura",
			},
			[]string{"model", "status", "stream", "service_id"},
		),

		TokensTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kura",
				Name:      "tokens_total",
				Help:      "Total number of tokens consumed",
			},
			[]string{"model", "type", "service_id"},
		),

		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "kura",
				Name:      "request_duration_seconds",
				Help:      "Total latency of Kura requests in seconds",
				Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
			},
			[]string{"model", "stream", "status"},
		),

		TimeToFirstToken: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "kura",
				Name:      "time_to_first_token_seconds",
				Help:      "Time to first token (TTFT) for streaming responses in seconds",
				Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
			},
			[]string{"model"},
		),

		EstimatedCostTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kura",
				Name:      "estimated_cost_usd_total",
				Help:      "Cumulative estimated cost in USD based on token usage",
			},
			[]string{"model", "service_id"},
		),

		ActiveRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "kura",
				Name:      "active_requests",
				Help:      "Number of currently active requests in flight",
			},
		),

		CacheHitsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kura",
				Name:      "cache_hits_total",
				Help:      "Total number of cache hits in Kura cache layer",
			},
			[]string{"type"},
		),

		CacheMissesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kura",
				Name:      "cache_misses_total",
				Help:      "Total number of cache misses in Kura cache layer",
			},
			[]string{"type"},
		),

		CacheNegativeRejectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kura",
				Name:      "cache_negative_rejections_total",
				Help:      "Total number of requests rejected by local negative cache",
			},
			[]string{"service_id"},
		),

		CacheFlushCountTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "kura",
				Name:      "cache_flush_count_total",
				Help:      "Total number of cache batch write flushes",
			},
		),

		CacheBufferedCostUSD: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "kura",
				Name:      "cache_buffered_cost_usd",
				Help:      "Total unflushed cost in USD in batch buffer",
			},
		),
	}

	registry.MustRegister(
		m.RequestsTotal,
		m.TokensTotal,
		m.RequestDuration,
		m.TimeToFirstToken,
		m.EstimatedCostTotal,
		m.ActiveRequests,
		m.CacheHitsTotal,
		m.CacheMissesTotal,
		m.CacheNegativeRejectionsTotal,
		m.CacheFlushCountTotal,
		m.CacheBufferedCostUSD,
	)

	return m
}

// Handler は Prometheus スクレイピング用 HTTP ハンドラーを返す
func (m *Metrics) Handler() http.Handler {
	if m == nil || m.Registry == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
	}
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// IncActive はアクティブ実行中リクエスト数をインクリメントする
func (m *Metrics) IncActive() {
	if m == nil {
		return
	}
	m.ActiveRequests.Inc()
}

// DecActive はアクティブ実行中リクエスト数をデクリメントする
func (m *Metrics) DecActive() {
	if m == nil {
		return
	}
	m.ActiveRequests.Dec()
}

// RecordRequest はリクエスト結果および所要時間を記録する
func (m *Metrics) RecordRequest(model string, stream bool, statusCode int, duration time.Duration, serviceID string) {
	if m == nil {
		return
	}
	if model == "" {
		model = "unknown"
	}
	if serviceID == "" {
		serviceID = "default"
	}

	streamStr := "false"
	if stream {
		streamStr = "true"
	}
	statusStr := fmt.Sprintf("%d", statusCode)

	m.RequestsTotal.WithLabelValues(model, statusStr, streamStr, serviceID).Inc()
	m.RequestDuration.WithLabelValues(model, streamStr, statusStr).Observe(duration.Seconds())
}

// RecordTTFT は初速トークン生成時間 (TTFT) を記録する
func (m *Metrics) RecordTTFT(model string, ttft time.Duration) {
	if m == nil || ttft <= 0 {
		return
	}
	if model == "" {
		model = "unknown"
	}
	m.TimeToFirstToken.WithLabelValues(model).Observe(ttft.Seconds())
}

// RecordTokens はトークン消費量および概算コストを記録する
func (m *Metrics) RecordTokens(model string, prompt, completion, total int64, cost float64, serviceID string) {
	if m == nil {
		return
	}
	if model == "" {
		model = "unknown"
	}
	if serviceID == "" {
		serviceID = "default"
	}

	if prompt > 0 {
		m.TokensTotal.WithLabelValues(model, "prompt", serviceID).Add(float64(prompt))
	}
	if completion > 0 {
		m.TokensTotal.WithLabelValues(model, "completion", serviceID).Add(float64(completion))
	}
	if total > 0 {
		m.TokensTotal.WithLabelValues(model, "total", serviceID).Add(float64(total))
	}
	if cost > 0 {
		m.EstimatedCostTotal.WithLabelValues(model, serviceID).Add(cost)
	}
}

// RecordCacheHit はキャッシュヒット数を記録する
func (m *Metrics) RecordCacheHit(cacheType string) {
	if m == nil || m.CacheHitsTotal == nil {
		return
	}
	m.CacheHitsTotal.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss はキャッシュミス数を記録する
func (m *Metrics) RecordCacheMiss(cacheType string) {
	if m == nil || m.CacheMissesTotal == nil {
		return
	}
	m.CacheMissesTotal.WithLabelValues(cacheType).Inc()
}

// RecordNegativeRejection はネガティブキャッシュによる拒否を記録する
func (m *Metrics) RecordNegativeRejection(serviceID string) {
	if m == nil || m.CacheNegativeRejectionsTotal == nil {
		return
	}
	if serviceID == "" {
		serviceID = "default"
	}
	m.CacheNegativeRejectionsTotal.WithLabelValues(serviceID).Inc()
}

// RecordCacheFlush はバッチフラッシュ実行を記録する
func (m *Metrics) RecordCacheFlush() {
	if m == nil || m.CacheFlushCountTotal == nil {
		return
	}
	m.CacheFlushCountTotal.Inc()
}

// SetBufferedCost はバッファ中の未反映コストを設定する
func (m *Metrics) SetBufferedCost(usd float64) {
	if m == nil || m.CacheBufferedCostUSD == nil {
		return
	}
	m.CacheBufferedCostUSD.Set(usd)
}
