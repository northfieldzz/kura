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
	Registry           *prometheus.Registry
	RequestsTotal      *prometheus.CounterVec
	TokensTotal        *prometheus.CounterVec
	RequestDuration    *prometheus.HistogramVec
	TimeToFirstToken   *prometheus.HistogramVec
	EstimatedCostTotal *prometheus.CounterVec
	RateLimitedTotal   *prometheus.CounterVec
	ActiveRequests     prometheus.Gauge
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
				Namespace: "llm_gateway",
				Name:      "requests_total",
				Help:      "Total number of HTTP / WebSocket requests handled by LLM Gateway",
			},
			[]string{"model", "status", "stream", "service_id"},
		),

		TokensTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "llm_gateway",
				Name:      "tokens_total",
				Help:      "Total number of tokens consumed",
			},
			[]string{"model", "type", "service_id"},
		),

		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "llm_gateway",
				Name:      "request_duration_seconds",
				Help:      "Total latency of LLM Gateway requests in seconds",
				Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
			},
			[]string{"model", "stream", "status"},
		),

		TimeToFirstToken: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "llm_gateway",
				Name:      "time_to_first_token_seconds",
				Help:      "Time to first token (TTFT) for streaming responses in seconds",
				Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
			},
			[]string{"model"},
		),

		EstimatedCostTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "llm_gateway",
				Name:      "estimated_cost_usd_total",
				Help:      "Cumulative estimated cost in USD based on token usage",
			},
			[]string{"model", "service_id"},
		),

		RateLimitedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "llm_gateway",
				Name:      "rate_limited_total",
				Help:      "Total number of requests rejected by dynamic rate limiter (RPM)",
			},
			[]string{"service_id"},
		),

		ActiveRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "llm_gateway",
				Name:      "active_requests",
				Help:      "Number of currently active requests in flight",
			},
		),
	}

	registry.MustRegister(
		m.RequestsTotal,
		m.TokensTotal,
		m.RequestDuration,
		m.TimeToFirstToken,
		m.EstimatedCostTotal,
		m.RateLimitedTotal,
		m.ActiveRequests,
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

// RecordRateLimited はレートリミット拒絶 (429) を記録する
func (m *Metrics) RecordRateLimited(serviceID string) {
	if m == nil {
		return
	}
	if serviceID == "" {
		serviceID = "default"
	}
	m.RateLimitedTotal.WithLabelValues(serviceID).Inc()
}
