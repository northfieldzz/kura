package proxy_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/domain/service"
	"github.com/northfieldzz/llm_gateway/internal/infrastructure/proxy"
)

// mockTestAdapter はテスト用のアダプター
type mockTestAdapter struct {
	targetURL string
}

func (m *mockTestAdapter) Provider() service.ProviderType { return service.ProviderOpenAI }
func (m *mockTestAdapter) IsEnabled() bool                { return true }
func (m *mockTestAdapter) PrepareRequest(ctx context.Context, origReq *entity.ChatCompletionRequest, httpReq *http.Request) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodPost, m.targetURL, nil)
}
func (m *mockTestAdapter) ExtractUsageFromResponse(body []byte) (*entity.UsageInfo, error) {
	return nil, nil
}
func (m *mockTestAdapter) ExtractUsageFromChunk(chunk []byte) (*entity.UsageInfo, error) {
	return nil, nil
}
func (m *mockTestAdapter) NormalizeResponse(statusCode int, body []byte) ([]byte, error) {
	return body, nil
}
func (m *mockTestAdapter) NormalizeSSEChunk(chunk []byte) ([][]byte, error) {
	return [][]byte{chunk}, nil
}

// flushWriter は http.Flusher を実装した httptest.ResponseRecorder のラッパー
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushRecorder) Flush() {
	f.flushed = true
}

func TestStreamingClientDisconnect(t *testing.T) {
	var upstreamCanceled atomic.Bool
	upstreamStarted := make(chan struct{})

	// アップストリームのモックサーバー: チャンクを無限に送信し続ける
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(upstreamStarted)

		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				upstreamCanceled.Store(true)
				return
			case <-ticker.C:
				_, err := fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
				if err != nil {
					upstreamCanceled.Store(true)
					return
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}))
	defer upstreamServer.Close()

	llmProxy := proxy.NewLLMProxy(nil, nil)
	adapter := &mockTestAdapter{targetURL: upstreamServer.URL}

	ctx, cancel := context.WithCancel(context.Background())

	clientReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	reqObj := &entity.ChatCompletionRequest{
		Model:  "gpt-4o",
		Stream: true,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		llmProxy.ServeForward(rec, clientReq, nil, reqObj, adapter)
	}()

	// アップストリームが開始するのを待つ
	select {
	case <-upstreamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream to start")
	}

	// クライアント側で切断をシミュレート
	time.Sleep(100 * time.Millisecond)
	cancel()

	// プロキシの処理が即座に完了するか確認（ハングしないこと）
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeForward did not terminate promptly upon client disconnect")
	}

	// アップストリームのコネクションも切断されたか確認
	time.Sleep(100 * time.Millisecond)
	if !upstreamCanceled.Load() {
		t.Error("expected upstream request to be canceled when client disconnected")
	}
}

func TestNonStreamingClientDisconnect(t *testing.T) {
	var upstreamCanceled atomic.Bool
	upstreamStarted := make(chan struct{})

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(upstreamStarted)
		select {
		case <-r.Context().Done():
			upstreamCanceled.Store(true)
			return
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}
	}))
	defer upstreamServer.Close()

	llmProxy := proxy.NewLLMProxy(nil, nil)
	adapter := &mockTestAdapter{targetURL: upstreamServer.URL}

	ctx, cancel := context.WithCancel(context.Background())
	clientReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	reqObj := &entity.ChatCompletionRequest{
		Model:  "gpt-4o",
		Stream: false,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		llmProxy.ServeForward(rec, clientReq, nil, reqObj, adapter)
	}()

	select {
	case <-upstreamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream to start")
	}

	// クライアント切断
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeForward did not terminate promptly upon client disconnect")
	}

	time.Sleep(100 * time.Millisecond)
	if !upstreamCanceled.Load() {
		t.Error("expected upstream request to be canceled when client disconnected in non-streaming mode")
	}
}
